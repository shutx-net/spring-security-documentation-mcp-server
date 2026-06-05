import importlib.util
import pathlib

import boto3 as _real_boto3
import pytest
from moto import mock_aws
from unittest.mock import MagicMock, patch

_path = pathlib.Path(__file__).parent.parent / "lambda" / "cleanup" / "index.py"
_spec = importlib.util.spec_from_file_location("cleanup_handler", _path)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

_REGION = "ap-northeast-1"
_CHUNKS_TABLE = "test-chunks-lambda"
_KEYWORDS_TABLE = "test-keywords-lambda"
_GSI_NAME = "ref-commitSha-index"
_VECTOR_INDEX = "arn:aws:s3vectors:ap-northeast-1:123456789012:bucket/test/index/idx"
_BUCKET = "test-bucket-lambda"


@pytest.fixture(autouse=True)
def lambda_env(monkeypatch):
    monkeypatch.setenv("CHUNKS_TABLE", _CHUNKS_TABLE)
    monkeypatch.setenv("KEYWORDS_TABLE", _KEYWORDS_TABLE)
    monkeypatch.setenv("CHUNKS_TABLE_GSI_NAME", _GSI_NAME)
    monkeypatch.setenv("VECTOR_INDEX", _VECTOR_INDEX)
    monkeypatch.setenv("CONTENT_BUCKET", _BUCKET)


@pytest.fixture
def aws_resources():
    with mock_aws():
        dynamo = _real_boto3.resource("dynamodb", region_name=_REGION)
        chunks_table = dynamo.create_table(
            TableName=_CHUNKS_TABLE,
            KeySchema=[{"AttributeName": "chunkId", "KeyType": "HASH"}],
            AttributeDefinitions=[
                {"AttributeName": "chunkId", "AttributeType": "S"},
                {"AttributeName": "ref", "AttributeType": "S"},
                {"AttributeName": "commitSha", "AttributeType": "S"},
            ],
            GlobalSecondaryIndexes=[{
                "IndexName": _GSI_NAME,
                "KeySchema": [
                    {"AttributeName": "ref", "KeyType": "HASH"},
                    {"AttributeName": "commitSha", "KeyType": "RANGE"},
                ],
                "Projection": {"ProjectionType": "ALL"},
            }],
            BillingMode="PAY_PER_REQUEST",
        )
        keywords_table = dynamo.create_table(
            TableName=_KEYWORDS_TABLE,
            KeySchema=[
                {"AttributeName": "keyword", "KeyType": "HASH"},
                {"AttributeName": "refAreaChunkId", "KeyType": "RANGE"},
            ],
            AttributeDefinitions=[
                {"AttributeName": "keyword", "AttributeType": "S"},
                {"AttributeName": "refAreaChunkId", "AttributeType": "S"},
            ],
            BillingMode="PAY_PER_REQUEST",
        )
        s3 = _real_boto3.client("s3", region_name=_REGION)
        s3.create_bucket(
            Bucket=_BUCKET,
            CreateBucketConfiguration={"LocationConstraint": _REGION},
        )
        yield chunks_table, keywords_table, s3


def _invoke(ref="6.5.x", commit_sha="new_sha", mock_sv=None):
    if mock_sv is None:
        mock_sv = MagicMock()

    real_client = _real_boto3.client

    def _factory(service, **kwargs):
        if service == "s3vectors":
            return mock_sv
        return real_client(service, **kwargs)

    with patch("boto3.client", side_effect=_factory):
        result = _mod.handler({"ref": ref, "commitSha": commit_sha}, None)
    return result, mock_sv


def test_no_stale_data(aws_resources):
    chunks_table, _, _ = aws_resources
    chunks_table.put_item(Item={"chunkId": "new1", "ref": "6.5.x", "commitSha": "new_sha"})

    result, mock_sv = _invoke()

    assert result == {"chunks": 0, "keywords": 0, "vectors": 0, "s3_objects": 0}
    mock_sv.delete_vectors.assert_not_called()


def test_removes_stale_chunks(aws_resources):
    chunks_table, _, _ = aws_resources
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    chunks_table.put_item(Item={"chunkId": "new1", "ref": "6.5.x", "commitSha": "new_sha"})

    result, _ = _invoke()

    assert result["chunks"] == 1
    remaining = chunks_table.scan()["Items"]
    assert len(remaining) == 1
    assert remaining[0]["chunkId"] == "new1"


def test_removes_stale_keywords(aws_resources):
    chunks_table, keywords_table, _ = aws_resources
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

    result, _ = _invoke()

    assert result["keywords"] == 1
    remaining = keywords_table.scan()["Items"]
    assert len(remaining) == 1
    assert remaining[0]["chunkId"] == "new1"


def test_calls_delete_vectors(aws_resources):
    chunks_table, _, _ = aws_resources
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})

    mock_sv = MagicMock()
    result, _ = _invoke(mock_sv=mock_sv)

    assert result["vectors"] == 1
    mock_sv.delete_vectors.assert_called_once_with(
        indexArn=_VECTOR_INDEX,
        keys=["old1"],
    )


def test_deletes_stale_s3_objects(aws_resources):
    chunks_table, _, s3 = aws_resources
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    s3.put_object(
        Bucket=_BUCKET,
        Key="chunks/spring-security/6.5.x/old_sha/chunks.jsonl.gz",
        Body=b"data",
    )

    result, _ = _invoke()

    assert result["s3_objects"] == 1
    resp = s3.list_objects_v2(Bucket=_BUCKET, Prefix="chunks/spring-security/6.5.x/old_sha/")
    assert resp.get("KeyCount", 0) == 0


def test_preserves_current_sha_s3_objects(aws_resources):
    chunks_table, _, s3 = aws_resources
    chunks_table.put_item(Item={"chunkId": "old1", "ref": "6.5.x", "commitSha": "old_sha"})
    s3.put_object(Bucket=_BUCKET, Key="chunks/spring-security/6.5.x/old_sha/f", Body=b"old")
    s3.put_object(Bucket=_BUCKET, Key="chunks/spring-security/6.5.x/new_sha/f", Body=b"new")

    _invoke()

    resp = s3.list_objects_v2(Bucket=_BUCKET, Prefix="chunks/spring-security/6.5.x/new_sha/")
    assert resp["KeyCount"] == 1


def test_multiple_stale_chunks_batched_to_delete_vectors(aws_resources):
    chunks_table, _, _ = aws_resources
    for i in range(3):
        chunks_table.put_item(Item={"chunkId": f"old{i}", "ref": "6.5.x", "commitSha": "old_sha"})

    mock_sv = MagicMock()
    result, _ = _invoke(mock_sv=mock_sv)

    assert result["chunks"] == 3
    assert result["vectors"] == 3
    called_keys = mock_sv.delete_vectors.call_args[1]["keys"]
    assert set(called_keys) == {"old0", "old1", "old2"}
