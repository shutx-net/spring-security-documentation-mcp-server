"""cleanup Lambda.

Input:  { "ref": "6.5.x", "commitSha": "abc123..." }
Output: { "chunks": N, "keywords": N, "vectors": N, "s3_objects": N }

Deletes data for the same ref but a different (old) commitSha from:
  - DynamoDB chunks table   (via GSI query, then batch delete)
  - DynamoDB keywords table (via full scan filtered by stale chunkIds)
  - S3 Vectors index        (batch delete by chunkId key)
  - S3 content bucket       (old commitSha prefixes under chunks/{ref}/)

Environment variables (injected by CDK):
  CHUNKS_TABLE          DynamoDB chunks table name
  KEYWORDS_TABLE        DynamoDB keywords table name
  CHUNKS_TABLE_GSI_NAME GSI name on chunks table (ref + commitSha keys)
  VECTOR_INDEX          S3 Vectors index ARN
  CONTENT_BUCKET        S3 bucket name for chunk artifacts
"""

import os

import boto3
from boto3.dynamodb.conditions import Attr, Key

VECTOR_BATCH_SIZE = 500


def handler(event, context):
    ref = event["ref"]
    commit_sha = event["commitSha"]

    chunks_table_name = os.environ["CHUNKS_TABLE"]
    keywords_table_name = os.environ["KEYWORDS_TABLE"]
    gsi_name = os.environ["CHUNKS_TABLE_GSI_NAME"]
    vector_index_arn = os.environ["VECTOR_INDEX"]
    s3_bucket = os.environ["CONTENT_BUCKET"]

    dynamo = boto3.resource("dynamodb")
    s3_client = boto3.client("s3")
    s3vectors_client = boto3.client("s3vectors")

    chunks_table = dynamo.Table(chunks_table_name)
    keywords_table = dynamo.Table(keywords_table_name)

    # 1. Find stale chunkIds via GSI (avoids full table scan)
    stale_ids = _stale_chunk_ids(chunks_table, gsi_name, ref, commit_sha)
    if not stale_ids:
        return {"chunks": 0, "keywords": 0, "vectors": 0, "s3_objects": 0}

    stale_set = set(stale_ids)

    # 2. Delete stale chunks from DynamoDB
    with chunks_table.batch_writer() as bw:
        for cid in stale_ids:
            bw.delete_item(Key={"chunkId": cid})

    # 3. Delete stale keyword entries (keywords table has no commitSha attr)
    kw_deleted = _delete_stale_keywords(keywords_table, stale_set)

    # 4. Delete stale vectors
    for i in range(0, len(stale_ids), VECTOR_BATCH_SIZE):
        s3vectors_client.delete_vectors(
            indexArn=vector_index_arn,
            keys=stale_ids[i: i + VECTOR_BATCH_SIZE],
        )

    # 5. Delete stale S3 objects (old commitSha prefixes under chunks/{ref}/)
    s3_deleted = _delete_stale_s3(s3_client, s3_bucket, ref, commit_sha)

    return {
        "chunks": len(stale_ids),
        "keywords": kw_deleted,
        "vectors": len(stale_ids),
        "s3_objects": s3_deleted,
    }


def _stale_chunk_ids(table, gsi_name: str, ref: str, current_commit_sha: str) -> list[str]:
    stale: list[str] = []
    kwargs: dict = {
        "IndexName": gsi_name,
        "KeyConditionExpression": Key("ref").eq(ref),
        "FilterExpression": Attr("commitSha").ne(current_commit_sha),
        "ProjectionExpression": "chunkId",
    }
    while True:
        resp = table.query(**kwargs)
        stale.extend(item["chunkId"] for item in resp.get("Items", []))
        last = resp.get("LastEvaluatedKey")
        if not last:
            break
        kwargs["ExclusiveStartKey"] = last
    return stale


def _delete_stale_keywords(table, stale_set: set) -> int:
    deleted = 0
    kwargs: dict = {
        "ProjectionExpression": "#kw, refAreaChunkId, chunkId",
        "ExpressionAttributeNames": {"#kw": "keyword"},
    }
    while True:
        resp = table.scan(**kwargs)
        with table.batch_writer() as bw:
            for item in resp.get("Items", []):
                if item.get("chunkId") in stale_set:
                    bw.delete_item(Key={
                        "keyword": item["keyword"],
                        "refAreaChunkId": item["refAreaChunkId"],
                    })
                    deleted += 1
        last = resp.get("LastEvaluatedKey")
        if not last:
            break
        kwargs["ExclusiveStartKey"] = last
    return deleted


def _delete_stale_s3(s3, bucket: str, ref: str, commit_sha: str) -> int:
    prefix = f"chunks/spring-security/{ref}/"
    paginator = s3.get_paginator("list_objects_v2")
    keys: list[str] = []
    for page in paginator.paginate(Bucket=bucket, Prefix=prefix, Delimiter="/"):
        for cp in page.get("CommonPrefixes", []):
            sha = cp["Prefix"].rstrip("/").split("/")[-1]
            if sha != commit_sha:
                for obj_page in paginator.paginate(Bucket=bucket, Prefix=cp["Prefix"]):
                    keys.extend(obj["Key"] for obj in obj_page.get("Contents", []))
    for i in range(0, len(keys), 1000):
        s3.delete_objects(
            Bucket=bucket,
            Delete={"Objects": [{"Key": k} for k in keys[i: i + 1000]]},
        )
    return len(keys)
