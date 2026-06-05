import os

import boto3
import pytest
from moto import mock_aws

os.environ.setdefault("AWS_DEFAULT_REGION", "ap-northeast-1")
os.environ.setdefault("AWS_ACCESS_KEY_ID", "testing")
os.environ.setdefault("AWS_SECRET_ACCESS_KEY", "testing")
os.environ.setdefault("AWS_SECURITY_TOKEN", "testing")
os.environ.setdefault("AWS_SESSION_TOKEN", "testing")

_REGION = "ap-northeast-1"


@pytest.fixture
def aws_mock():
    with mock_aws():
        yield


@pytest.fixture
def dynamodb_tables(aws_mock):
    dynamo = boto3.resource("dynamodb", region_name=_REGION)
    chunks_table = dynamo.create_table(
        TableName="test-chunks",
        KeySchema=[{"AttributeName": "chunkId", "KeyType": "HASH"}],
        AttributeDefinitions=[{"AttributeName": "chunkId", "AttributeType": "S"}],
        BillingMode="PAY_PER_REQUEST",
    )
    keywords_table = dynamo.create_table(
        TableName="test-keywords",
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
    return dynamo, chunks_table, keywords_table


@pytest.fixture
def s3_bucket(aws_mock):
    s3 = boto3.client("s3", region_name=_REGION)
    s3.create_bucket(
        Bucket="test-bucket",
        CreateBucketConfiguration={"LocationConstraint": _REGION},
    )
    return s3
