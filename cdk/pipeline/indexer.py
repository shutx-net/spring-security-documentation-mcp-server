"""Spring Security docs indexer — Phase 2 of the CodeBuild pipeline.

Usage (called from buildspec.yml):
    python pipeline/indexer.py \\
        --site-dir /tmp/spring-security-6.5.x/docs/build/site \\
        --ref 6.5.x \\
        --commit-sha abc123def

Environment variables (injected by CDK / CodeBuild):
    CONTENT_BUCKET      S3 bucket for chunks.jsonl.gz / metadata.json / latest.json
    VECTOR_BUCKET       S3 Vector Bucket name
    VECTOR_INDEX        S3 Vector Index name
    CHUNKS_TABLE        DynamoDB table for doc chunks
    KEYWORDS_TABLE      DynamoDB table for keyword index
    EMBEDDING_MODEL_ID  Bedrock model ID (amazon.titan-embed-text-v2:0)
    AWS_DEFAULT_REGION  Auto-set by CodeBuild
"""

import argparse
import gzip
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

import boto3
from bs4 import BeautifulSoup

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

VECTOR_BATCH_SIZE = 500   # S3 Vectors PutVectors hard limit

AREA_PREFIXES: dict[str, str] = {
    "servlet":         "servlet",
    "reactive":        "reactive",
    "oauth2":          "oauth2",
    "saml2":           "saml2",
    "method-security": "method-security",
    "testing":         "testing",
    "architecture":    "architecture",
    "authorization":   "authorization",
    "authentication":  "authentication",
}

KEYWORDS: list[str] = [
    "SecurityFilterChain", "SecurityWebFilterChain",
    "@PreAuthorize", "@PostAuthorize",
    "@WithMockUser", "@EnableMethodSecurity",
    "oauth2ResourceServer", "csrf", "JwtDecoder",
    "AuthenticationManager", "UserDetailsService",
    "HttpSecurity", "WebSecurityCustomizer",
]

# Bedrock Titan Embed v2 token limit (safety margin applied)
MAX_INPUT_CHARS = 30_000


# ---------------------------------------------------------------------------
# AWS clients
# ---------------------------------------------------------------------------

def _clients() -> dict:
    region = os.environ["AWS_DEFAULT_REGION"]
    return {
        "bedrock":    boto3.client("bedrock-runtime", region_name=region),
        "s3":         boto3.client("s3", region_name=region),
        "s3vectors":  boto3.client("s3vectors", region_name=region),
        "dynamodb":   boto3.resource("dynamodb", region_name=region),
    }


# ---------------------------------------------------------------------------
# HTML parsing + chunking
# ---------------------------------------------------------------------------

def _detect_area(html_path: str) -> str:
    for key, val in AREA_PREFIXES.items():
        if key in html_path:
            return val
    return "other"


def _canonical_url(html_path: str, site_dir: str) -> str:
    rel = Path(html_path).relative_to(site_dir)
    return f"https://docs.spring.io/spring-security/reference/{rel}"


def _chunk_id(ref: str, commit_sha: str, canonical_url: str, heading_path: list[str]) -> str:
    raw = f"{ref}:{commit_sha}:{canonical_url}:{'/'.join(heading_path)}"
    return hashlib.sha256(raw.encode()).hexdigest()


def parse_html(html_path: str, site_dir: str, ref: str, commit_sha: str, built_at: str) -> list[dict]:
    """Parse one HTML file and return chunks split at h1/h2/h3 boundaries.

    TODO: implement proper heading-level splitting.
    Current: one chunk per page (h1 boundary only).
    """
    with open(html_path, encoding="utf-8") as f:
        soup = BeautifulSoup(f, "lxml")

    area = _detect_area(html_path)
    canonical_url = _canonical_url(html_path, site_dir)

    # Remove nav/header/footer noise before extracting content
    for tag in soup.select("nav, header, footer, .nav, .toc, script, style"):
        tag.decompose()

    content_div = soup.find("article") or soup.find("main") or soup.body
    if content_div is None:
        return []

    h1 = soup.find("h1")
    title = h1.get_text(strip=True) if h1 else Path(html_path).stem
    heading_path = [title]

    content_html = str(content_div)
    content_text = content_div.get_text(separator="\n", strip=True)

    return [{
        "chunkId":     _chunk_id(ref, commit_sha, canonical_url, heading_path),
        "ref":         ref,
        "commitSha":   commit_sha,
        "builtAt":     built_at,
        "area":        area,
        "title":       title,
        "headingPath": heading_path,
        "canonicalUrl": canonical_url,
        "sourcePath":  str(Path(html_path).relative_to(site_dir)),
        "contentHtml": content_html[:MAX_INPUT_CHARS],
        "contentText": content_text[:MAX_INPUT_CHARS],
    }]


# ---------------------------------------------------------------------------
# Embedding (Bedrock Titan Text Embeddings v2)
# ---------------------------------------------------------------------------

def embed_chunks(bedrock_client, chunks: list[dict], model_id: str) -> list[list[float]]:
    embeddings: list[list[float]] = []
    for i, chunk in enumerate(chunks, 1):
        body = json.dumps({
            "inputText": chunk["contentText"],
            "dimensions": 1024,
            "normalize": True,
        })
        resp = bedrock_client.invoke_model(
            modelId=model_id,
            body=body,
        )
        result = json.loads(resp["body"].read())
        embeddings.append(result["embedding"])
        if i % 100 == 0:
            print(f"  Embedded {i}/{len(chunks)}")
    return embeddings


# ---------------------------------------------------------------------------
# S3 Vectors
# ---------------------------------------------------------------------------

def put_vectors(s3vectors_client, vector_index_arn: str,
                chunks: list[dict], embeddings: list[list[float]]) -> None:
    vectors = [
        {
            "key": chunk["chunkId"],
            "data": {"float32": emb},
            "metadata": {
                "ref":     chunk["ref"],
                "area":    chunk["area"],
                "chunkId": chunk["chunkId"],
            },
        }
        for chunk, emb in zip(chunks, embeddings)
    ]
    for i in range(0, len(vectors), VECTOR_BATCH_SIZE):
        batch = vectors[i: i + VECTOR_BATCH_SIZE]
        s3vectors_client.put_vectors(
            indexArn=vector_index_arn,
            vectors=batch,
        )
        print(f"  PutVectors: {i + len(batch)}/{len(vectors)}")


# ---------------------------------------------------------------------------
# DynamoDB
# ---------------------------------------------------------------------------

def write_chunks_dynamo(dynamo_resource, table_name: str, chunks: list[dict]) -> None:
    table = dynamo_resource.Table(table_name)
    with table.batch_writer() as bw:
        for chunk in chunks:
            bw.put_item(Item=chunk)


def _collect_stale_chunk_ids(table, ref: str, current_commit_sha: str) -> list[str]:
    """Scan the chunks table and return chunkIds for same ref but different commitSha."""
    stale: list[str] = []
    kwargs: dict = {
        "FilterExpression": "ref = :ref AND commitSha <> :sha",
        "ExpressionAttributeValues": {":ref": ref, ":sha": current_commit_sha},
        "ProjectionExpression": "chunkId",
    }
    while True:
        resp = table.scan(**kwargs)
        stale.extend(item["chunkId"] for item in resp.get("Items", []))
        last = resp.get("LastEvaluatedKey")
        if not last:
            break
        kwargs["ExclusiveStartKey"] = last
    return stale


def cleanup_stale_data(
    dynamo_resource, s3_client, s3vectors_client,
    chunks_table_name: str, keywords_table_name: str,
    vector_index_arn: str, s3_bucket: str,
    ref: str, commit_sha: str,
) -> dict:
    """Remove stale data (same ref, old commitSha) from all stores.

    Returns counts of deleted items per store.
    """
    chunks_table   = dynamo_resource.Table(chunks_table_name)
    keywords_table = dynamo_resource.Table(keywords_table_name)

    # 1. Collect stale chunkIds once — reused by all stores.
    stale_ids = _collect_stale_chunk_ids(chunks_table, ref, commit_sha)
    if not stale_ids:
        return {"chunks": 0, "keywords": 0, "vectors": 0, "s3_objects": 0}

    stale_set = set(stale_ids)

    # 2. Delete stale chunks from DynamoDB.
    with chunks_table.batch_writer() as bw:
        for cid in stale_ids:
            bw.delete_item(Key={"chunkId": cid})

    # 3. Delete stale keyword entries (scan full table; keywords has no commitSha attr).
    kw_deleted = 0
    kw_kwargs: dict = {"ProjectionExpression": "keyword, refAreaChunkId, chunkId"}
    while True:
        resp = keywords_table.scan(**kw_kwargs)
        with keywords_table.batch_writer() as bw:
            for item in resp.get("Items", []):
                if item.get("chunkId") in stale_set:
                    bw.delete_item(Key={
                        "keyword":        item["keyword"],
                        "refAreaChunkId": item["refAreaChunkId"],
                    })
                    kw_deleted += 1
        last = resp.get("LastEvaluatedKey")
        if not last:
            break
        kw_kwargs["ExclusiveStartKey"] = last

    # 4. Delete stale vectors.
    for i in range(0, len(stale_ids), VECTOR_BATCH_SIZE):
        s3vectors_client.delete_vectors(
            indexArn=vector_index_arn,
            keys=stale_ids[i: i + VECTOR_BATCH_SIZE],
        )

    # 5. Delete stale S3 chunk files (old commitSha prefixes under chunks/{ref}/).
    prefix = f"chunks/spring-security/{ref}/"
    paginator = s3_client.get_paginator("list_objects_v2")
    s3_keys: list[str] = []
    for page in paginator.paginate(Bucket=s3_bucket, Prefix=prefix, Delimiter="/"):
        for cp in page.get("CommonPrefixes", []):
            sha = cp["Prefix"].rstrip("/").split("/")[-1]
            if sha != commit_sha:
                for obj_page in paginator.paginate(Bucket=s3_bucket, Prefix=cp["Prefix"]):
                    s3_keys.extend(obj["Key"] for obj in obj_page.get("Contents", []))
    for i in range(0, len(s3_keys), 1000):
        s3_client.delete_objects(
            Bucket=s3_bucket,
            Delete={"Objects": [{"Key": k} for k in s3_keys[i: i + 1000]]},
        )

    return {
        "chunks":     len(stale_ids),
        "keywords":   kw_deleted,
        "vectors":    len(stale_ids),
        "s3_objects": len(s3_keys),
    }


def write_keywords_dynamo(dynamo_resource, table_name: str, chunks: list[dict]) -> None:
    table = dynamo_resource.Table(table_name)
    with table.batch_writer() as bw:
        for chunk in chunks:
            text = chunk.get("contentText", "")
            for kw in KEYWORDS:
                if kw.lower() in text.lower():
                    bw.put_item(Item={
                        "keyword":        kw,
                        "refAreaChunkId": f"{chunk['ref']}#{chunk['area']}#{chunk['chunkId']}",
                        "chunkId":        chunk["chunkId"],
                        "ref":            chunk["ref"],
                        "area":           chunk["area"],
                        "title":          chunk["title"],
                        "score":          1,
                    })


# ---------------------------------------------------------------------------
# S3 publish
# ---------------------------------------------------------------------------

def publish_to_s3(s3_client, bucket: str, ref: str, commit_sha: str,
                  chunks: list[dict], built_at: str) -> None:
    prefix = f"chunks/spring-security/{ref}/{commit_sha}"

    jsonl_bytes = "\n".join(json.dumps(c) for c in chunks).encode("utf-8")
    s3_client.put_object(
        Bucket=bucket,
        Key=f"{prefix}/chunks.jsonl.gz",
        Body=gzip.compress(jsonl_bytes),
        ContentEncoding="gzip",
        ContentType="application/json",
    )

    metadata = {
        "ref":        ref,
        "commitSha":  commit_sha,
        "chunkCount": len(chunks),
        "builtAt":    built_at,
    }
    s3_client.put_object(
        Bucket=bucket,
        Key=f"{prefix}/metadata.json",
        Body=json.dumps(metadata).encode("utf-8"),
        ContentType="application/json",
    )

    # latest.json is updated last — acts as an atomic "commit" of this build.
    # If anything above failed, latest.json still points to the previous good build.
    latest = {
        "ref":       ref,
        "commitSha": commit_sha,
        "builtAt":   built_at,
        "chunksKey": f"{prefix}/chunks.jsonl.gz",
    }
    s3_client.put_object(
        Bucket=bucket,
        Key=f"indexes/spring-security/{ref}/latest.json",
        Body=json.dumps(latest).encode("utf-8"),
        ContentType="application/json",
    )
    print(f"  latest.json updated → {commit_sha}")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def _parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Index Spring Security docs into AWS")
    p.add_argument("--site-dir",   required=True, help="Path to Antora-built site root")
    p.add_argument("--ref",        required=True, help="Spring Security ref, e.g. 6.5.x")
    p.add_argument("--commit-sha", required=True, help="Git commit SHA of the Spring Security repo")
    return p.parse_args()


def main() -> None:
    args = _parse_args()

    content_bucket = os.environ["CONTENT_BUCKET"]
    vector_index   = os.environ["VECTOR_INDEX"]
    chunks_table   = os.environ["CHUNKS_TABLE"]
    keywords_table = os.environ["KEYWORDS_TABLE"]
    model_id       = os.environ["EMBEDDING_MODEL_ID"]

    clients  = _clients()
    built_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    # 1. Parse HTML → chunks
    site_path  = Path(args.site_dir)
    html_files = sorted(site_path.rglob("*.html"))
    print(f"[{args.ref}] Found {len(html_files)} HTML files")

    all_chunks: list[dict] = []
    for html_path in html_files:
        all_chunks.extend(parse_html(str(html_path), args.site_dir, args.ref, args.commit_sha, built_at))
    print(f"[{args.ref}] Generated {len(all_chunks)} chunks")

    if not all_chunks:
        print(f"[{args.ref}] No chunks — aborting", file=sys.stderr)
        sys.exit(1)

    # 2. Embed
    print(f"[{args.ref}] Embedding with {model_id} ...")
    embeddings = embed_chunks(clients["bedrock"], all_chunks, model_id)

    # 3. S3 Vectors
    print(f"[{args.ref}] Writing vectors ...")
    put_vectors(clients["s3vectors"], vector_index, all_chunks, embeddings)

    # 4. DynamoDB
    print(f"[{args.ref}] Writing DynamoDB chunks ...")
    write_chunks_dynamo(clients["dynamodb"], chunks_table, all_chunks)
    print(f"[{args.ref}] Writing DynamoDB keywords ...")
    write_keywords_dynamo(clients["dynamodb"], keywords_table, all_chunks)
    print(f"[{args.ref}] Removing stale data ...")
    removed = cleanup_stale_data(
        clients["dynamodb"], clients["s3"], clients["s3vectors"],
        chunks_table, keywords_table, vector_index, content_bucket,
        args.ref, args.commit_sha,
    )
    print(f"[{args.ref}] Removed — chunks:{removed['chunks']} keywords:{removed['keywords']} vectors:{removed['vectors']} s3:{removed['s3_objects']}")

    # 5. S3 (latest.json written last)
    print(f"[{args.ref}] Uploading to S3 ...")
    publish_to_s3(clients["s3"], content_bucket, args.ref, args.commit_sha, all_chunks, built_at)

    print(f"[{args.ref}] Done — {len(all_chunks)} chunks @ {args.commit_sha}")


if __name__ == "__main__":
    main()
