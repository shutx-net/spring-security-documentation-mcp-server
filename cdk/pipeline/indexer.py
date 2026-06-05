"""Spring Security docs indexer.

Usage:
    # local site directory (CodeBuild legacy)
    python pipeline/indexer.py \\
        --site-dir /tmp/spring-security-6.5.x/docs/build/site \\
        --ref 6.5.x \\
        --commit-sha abc123def

    # S3 artifact (ECS Fargate)
    python pipeline/indexer.py \\
        --artifact-s3-key artifacts/6.5.x/abc123def/site.tar.gz \\
        --ref 6.5.x \\
        --commit-sha abc123def

    The tar.gz must be created with: tar -czf site.tar.gz -C /path/to/site .

Environment variables (injected by CDK):
    CONTENT_BUCKET      S3 bucket for chunks.jsonl.gz / metadata.json / latest.json
    VECTOR_BUCKET       S3 Vector Bucket name
    VECTOR_INDEX        S3 Vector Index name
    CHUNKS_TABLE        DynamoDB table for doc chunks
    KEYWORDS_TABLE      DynamoDB table for keyword index
    EMBEDDING_MODEL_ID  Bedrock model ID (amazon.titan-embed-text-v2:0)
    AWS_DEFAULT_REGION  Auto-set by CodeBuild / ECS
"""

import argparse
import gzip
import hashlib
import json
import os
import shutil
import sys
import tarfile
import tempfile
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
    # Check path components from deepest (most specific) to shallowest so that
    # servlet/oauth2/login.html → "oauth2" rather than "servlet".
    parts = Path(html_path).parts
    for part in reversed(parts):
        key = part.removesuffix(".html").lower()
        if key in AREA_PREFIXES:
            return AREA_PREFIXES[key]
    return "other"


def _canonical_url(html_path: str, site_dir: str) -> str:
    rel = Path(html_path).relative_to(site_dir)
    return f"https://docs.spring.io/spring-security/reference/{rel}"


def _chunk_id(ref: str, commit_sha: str, canonical_url: str, heading_path: list[str]) -> str:
    raw = f"{ref}:{commit_sha}:{canonical_url}:{'/'.join(heading_path)}"
    return hashlib.sha256(raw.encode()).hexdigest()


def _iter_content_nodes(element):
    """Yield (kind, node) pairs in document order.

    kind is 'h1', 'h2', 'h3', or 'content'.
    Descends into non-heading wrapper elements (e.g. Antora's div.sect1/sect2)
    so that headings nested inside those wrappers are surfaced at the correct
    level rather than being swallowed as opaque content blocks.
    """
    for child in element.children:
        if not hasattr(child, "name") or child.name is None:
            continue
        if child.name in ("h1", "h2", "h3"):
            yield child.name, child
        elif child.find(["h1", "h2", "h3"]):
            yield from _iter_content_nodes(child)
        else:
            yield "content", child


def parse_html(html_path: str, site_dir: str, ref: str, commit_sha: str, built_at: str) -> list[dict]:
    """Parse one HTML file and return chunks split at h1/h2/h3 boundaries."""
    with open(html_path, encoding="utf-8") as f:
        soup = BeautifulSoup(f, "lxml")

    area = _detect_area(html_path)
    canonical_url = _canonical_url(html_path, site_dir)
    source_path = str(Path(html_path).relative_to(site_dir))

    # Remove nav/header/footer noise before extracting content
    for tag in soup.select("nav, header, footer, .nav, .toc, script, style"):
        tag.decompose()

    content_div = soup.find("article") or soup.find("main") or soup.body
    if content_div is None:
        return []

    current_headings: list[str] = []
    html_parts: list[str] = []
    text_parts: list[str] = []
    chunks: list[dict] = []

    def flush() -> None:
        if html_parts and text_parts and current_headings:
            chunks.append({
                "chunkId":     _chunk_id(ref, commit_sha, canonical_url, current_headings),
                "ref":         ref,
                "commitSha":   commit_sha,
                "builtAt":     built_at,
                "area":        area,
                "title":       current_headings[-1],
                "headingPath": list(current_headings),
                "canonicalUrl": canonical_url,
                "sourcePath":  source_path,
                "contentHtml": "\n".join(html_parts)[:MAX_INPUT_CHARS],
                "contentText": "\n".join(text_parts)[:MAX_INPUT_CHARS],
            })
        html_parts.clear()
        text_parts.clear()

    for kind, node in _iter_content_nodes(content_div):
        if kind == "h1":
            flush()
            current_headings = [node.get_text(strip=True)]
        elif kind == "h2":
            flush()
            h = node.get_text(strip=True)
            current_headings = [current_headings[0], h] if current_headings else [h]
        elif kind == "h3":
            flush()
            h = node.get_text(strip=True)
            if len(current_headings) >= 2:
                current_headings = [current_headings[0], current_headings[1], h]
            elif len(current_headings) == 1:
                current_headings = [current_headings[0], h]
            else:
                current_headings = [h]
        else:  # content
            if current_headings:
                html_parts.append(str(node))
                text = node.get_text(strip=True)
                if text:
                    text_parts.append(text)

    flush()

    # If no chunks were produced (page has no headings or all headings lacked
    # body content), fall back to a single page-level chunk so content is not
    # silently dropped from the index.
    if not chunks:
        h1 = soup.find("h1")
        fallback_title = h1.get_text(strip=True) if h1 else Path(html_path).stem
        content_text = content_div.get_text(separator="\n", strip=True)
        if content_text.strip():
            chunks.append({
                "chunkId":     _chunk_id(ref, commit_sha, canonical_url, [fallback_title]),
                "ref":         ref,
                "commitSha":   commit_sha,
                "builtAt":     built_at,
                "area":        area,
                "title":       fallback_title,
                "headingPath": [fallback_title],
                "canonicalUrl": canonical_url,
                "sourcePath":  source_path,
                "contentHtml": str(content_div)[:MAX_INPUT_CHARS],
                "contentText": content_text[:MAX_INPUT_CHARS],
            })

    return chunks


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


def _collect_all_chunk_ids_for_ref(table, ref: str) -> list[str]:
    """Scan the chunks table and return all chunkIds for the given ref."""
    ids: list[str] = []
    kwargs: dict = {
        "FilterExpression": "#ref = :ref",
        "ExpressionAttributeNames": {"#ref": "ref"},
        "ExpressionAttributeValues": {":ref": ref},
        "ProjectionExpression": "chunkId",
    }
    while True:
        resp = table.scan(**kwargs)
        ids.extend(item["chunkId"] for item in resp.get("Items", []))
        last = resp.get("LastEvaluatedKey")
        if not last:
            break
        kwargs["ExclusiveStartKey"] = last
    return ids


def cleanup_stale_data(
    dynamo_resource, s3_client, s3vectors_client,
    chunks_table_name: str, keywords_table_name: str,
    vector_index_arn: str, s3_bucket: str,
    ref: str, commit_sha: str,
    new_chunk_ids: set[str],
) -> dict:
    """Remove stale data for the given ref from all stores.

    Stale chunks are those whose chunkId is not in new_chunk_ids — this
    catches both old-commitSha chunks (new Spring Security release) and
    old-format chunks (indexer logic changed, same commitSha).

    commit_sha is still used to identify stale S3 build artifacts.

    Returns counts of deleted items per store.
    """
    chunks_table   = dynamo_resource.Table(chunks_table_name)
    keywords_table = dynamo_resource.Table(keywords_table_name)

    # 1. Collect stale chunkIds: all existing IDs for ref minus the new ones.
    all_ids  = _collect_all_chunk_ids_for_ref(chunks_table, ref)
    stale_ids = [cid for cid in all_ids if cid not in new_chunk_ids]
    if not stale_ids:
        return {"chunks": 0, "keywords": 0, "vectors": 0, "s3_objects": 0}

    stale_set = set(stale_ids)

    # 2. Delete stale chunks from DynamoDB.
    with chunks_table.batch_writer() as bw:
        for cid in stale_ids:
            bw.delete_item(Key={"chunkId": cid})

    # 3. Delete stale keyword entries (scan full table; keywords has no commitSha attr).
    kw_deleted = 0
    kw_kwargs: dict = {
        "ProjectionExpression": "#kw, refAreaChunkId, chunkId",
        "ExpressionAttributeNames": {"#kw": "keyword"},
    }
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
    src = p.add_mutually_exclusive_group(required=True)
    src.add_argument("--site-dir",        help="Path to Antora-built site root (local)")
    src.add_argument("--artifact-s3-key", help="S3 key for site.tar.gz in CONTENT_BUCKET")
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

    # Resolve site directory
    tmp_dir = None
    if args.artifact_s3_key:
        tmp_dir = tempfile.mkdtemp()
        archive_path = os.path.join(tmp_dir, "site.tar.gz")
        print(f"[{args.ref}] Downloading s3://{content_bucket}/{args.artifact_s3_key} ...")
        clients["s3"].download_file(content_bucket, args.artifact_s3_key, archive_path)
        site_dir = os.path.join(tmp_dir, "site")
        os.makedirs(site_dir)
        with tarfile.open(archive_path) as tf:
            tf.extractall(site_dir, filter="data")
        print(f"[{args.ref}] Extracted to {site_dir}")
    else:
        site_dir = args.site_dir

    try:
        _run(args, site_dir, content_bucket, vector_index, chunks_table, keywords_table, model_id, clients, built_at)
    finally:
        if tmp_dir:
            shutil.rmtree(tmp_dir, ignore_errors=True)


def _run(
    args: argparse.Namespace,
    site_dir: str,
    content_bucket: str,
    vector_index: str,
    chunks_table: str,
    keywords_table: str,
    model_id: str,
    clients: dict,
    built_at: str,
) -> None:
    # 1. Parse HTML → chunks
    site_path  = Path(site_dir)
    all_html   = sorted(site_path.rglob("*.html"))
    html_files = [f for f in all_html if "api" not in f.relative_to(site_dir).parts]
    print(f"[{args.ref}] Found {len(html_files)} HTML files (skipped {len(all_html) - len(html_files)} api/ files)")

    all_chunks: list[dict] = []
    for html_path in html_files:
        all_chunks.extend(parse_html(str(html_path), site_dir, args.ref, args.commit_sha, built_at))

    # Deduplicate by chunkId: same URL + same headingPath → same hash.
    # Duplicate headings on one page produce identical IDs; keep the first.
    seen_ids: set[str] = set()
    deduped: list[dict] = []
    for chunk in all_chunks:
        if chunk["chunkId"] not in seen_ids:
            seen_ids.add(chunk["chunkId"])
            deduped.append(chunk)
    if len(deduped) < len(all_chunks):
        print(f"[{args.ref}] Deduplicated {len(all_chunks) - len(deduped)} duplicate chunkIds")
    all_chunks = deduped
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
    new_chunk_ids = {c["chunkId"] for c in all_chunks}
    removed = cleanup_stale_data(
        clients["dynamodb"], clients["s3"], clients["s3vectors"],
        chunks_table, keywords_table, vector_index, content_bucket,
        args.ref, args.commit_sha, new_chunk_ids,
    )
    print(f"[{args.ref}] Removed — chunks:{removed['chunks']} keywords:{removed['keywords']} vectors:{removed['vectors']} s3:{removed['s3_objects']}")

    # 5. S3 (latest.json written last)
    print(f"[{args.ref}] Uploading to S3 ...")
    publish_to_s3(clients["s3"], content_bucket, args.ref, args.commit_sha, all_chunks, built_at)

    print(f"[{args.ref}] Done — {len(all_chunks)} chunks @ {args.commit_sha}")


if __name__ == "__main__":
    main()
