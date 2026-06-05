from unittest.mock import MagicMock

from indexer import cleanup_stale_data

_BUCKET = "test-bucket"
_CHUNKS_TABLE = "test-chunks"
_KEYWORDS_TABLE = "test-keywords"
_INDEX_ARN = "arn:aws:s3vectors:ap-northeast-1:123456789012:bucket/test/index/idx"


def _run_cleanup(dynamo, s3, s3vectors=None, ref="6.5.x", commit_sha="new_sha",
                 new_chunk_ids=None):
    return cleanup_stale_data(
        dynamo_resource=dynamo,
        s3_client=s3,
        s3vectors_client=s3vectors or MagicMock(),
        chunks_table_name=_CHUNKS_TABLE,
        keywords_table_name=_KEYWORDS_TABLE,
        vector_index_arn=_INDEX_ARN,
        s3_bucket=_BUCKET,
        ref=ref,
        commit_sha=commit_sha,
        new_chunk_ids=new_chunk_ids or {"new1"},
    )


def test_cleanup_no_stale_data(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "new1", "ref": "6.5.x", "commitSha": "new_sha"})

    result = _run_cleanup(dynamo, s3_bucket, new_chunk_ids={"new1"})
    assert result == {"chunks": 0, "keywords": 0, "vectors": 0, "s3_objects": 0}


def test_cleanup_removes_stale_chunks(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    chunks_table.put_item(Item={"chunkId": "new1", "ref": "6.5.x", "commitSha": "new_sha"})

    result = _run_cleanup(dynamo, s3_bucket, new_chunk_ids={"new1"})

    assert result["chunks"] == 1
    remaining = chunks_table.scan()["Items"]
    assert len(remaining) == 1
    assert remaining[0]["chunkId"] == "new1"


def test_cleanup_removes_stale_chunks_same_commit_sha(dynamodb_tables, s3_bucket):
    # Indexer logic changed but Spring Security source didn't: old chunks share
    # the same commitSha as new chunks but have different chunkIds (different
    # heading_path due to the old 1-page-per-chunk format).
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old-format-id", "ref": "6.5.x", "commitSha": "sha"})
    chunks_table.put_item(Item={"chunkId": "new-section-id", "ref": "6.5.x", "commitSha": "sha"})

    result = _run_cleanup(dynamo, s3_bucket, commit_sha="sha",
                          new_chunk_ids={"new-section-id"})

    assert result["chunks"] == 1
    remaining = chunks_table.scan()["Items"]
    assert len(remaining) == 1
    assert remaining[0]["chunkId"] == "new-section-id"


def test_cleanup_removes_stale_keywords(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, keywords_table = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    keywords_table.put_item(Item={
        "keyword": "csrf",
        "refAreaChunkId": "6.5.x#servlet#old1",
        "chunkId": "old1",
    })
    keywords_table.put_item(Item={
        "keyword": "csrf",
        "refAreaChunkId": "6.5.x#servlet#new1",
        "chunkId": "new1",
    })

    result = _run_cleanup(dynamo, s3_bucket, new_chunk_ids={"new1"})

    assert result["keywords"] == 1
    remaining = keywords_table.scan()["Items"]
    assert len(remaining) == 1
    assert remaining[0]["chunkId"] == "new1"


def test_cleanup_calls_delete_vectors(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})

    mock_s3vectors = MagicMock()
    result = _run_cleanup(dynamo, s3_bucket, s3vectors=mock_s3vectors, new_chunk_ids={"new1"})

    assert result["vectors"] == 1
    mock_s3vectors.delete_vectors.assert_called_once_with(
        indexArn=_INDEX_ARN,
        keys=["old1"],
    )


def test_cleanup_deletes_stale_s3_objects(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    s3_bucket.put_object(
        Bucket=_BUCKET,
        Key="chunks/spring-security/6.5.x/old_sha/chunks.jsonl.gz",
        Body=b"data",
    )

    result = _run_cleanup(dynamo, s3_bucket, new_chunk_ids={"new1"})

    assert result["s3_objects"] == 1
    resp = s3_bucket.list_objects_v2(Bucket=_BUCKET, Prefix="chunks/spring-security/6.5.x/old_sha/")
    assert resp.get("KeyCount", 0) == 0


def test_cleanup_preserves_current_sha_s3_objects(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    s3_bucket.put_object(
        Bucket=_BUCKET,
        Key="chunks/spring-security/6.5.x/old_sha/chunks.jsonl.gz",
        Body=b"old data",
    )
    s3_bucket.put_object(
        Bucket=_BUCKET,
        Key="chunks/spring-security/6.5.x/new_sha/chunks.jsonl.gz",
        Body=b"new data",
    )

    _run_cleanup(dynamo, s3_bucket, new_chunk_ids={"new1"})

    resp = s3_bucket.list_objects_v2(Bucket=_BUCKET, Prefix="chunks/spring-security/6.5.x/new_sha/")
    assert resp.get("KeyCount", 0) == 1


def test_cleanup_does_not_delete_vectors_when_no_stale(dynamodb_tables, s3_bucket):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "new1", "ref": "6.5.x", "commitSha": "new_sha"})

    mock_s3vectors = MagicMock()
    _run_cleanup(dynamo, s3_bucket, s3vectors=mock_s3vectors, new_chunk_ids={"new1"})

    mock_s3vectors.delete_vectors.assert_not_called()
