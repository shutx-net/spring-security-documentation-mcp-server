import json
from io import BytesIO

import boto3
import pytest
from botocore.exceptions import ClientError
from botocore.stub import Stubber

from indexer import embed_chunks

_MODEL_ID = "amazon.titan-embed-text-v2:0"
_REGION = "ap-northeast-1"


def _bedrock_client():
    return boto3.client("bedrock-runtime", region_name=_REGION)


def _fake_response(embedding: list[float]) -> dict:
    return {
        "body": BytesIO(json.dumps({"embedding": embedding}).encode()),
        "contentType": "application/json",
    }


def test_embed_chunks_returns_embeddings():
    client = _bedrock_client()
    chunks = [{"contentText": "Spring Security CSRF protection"}]
    fake_embedding = [0.1] * 1024

    with Stubber(client) as stubber:
        stubber.add_response(
            method="invoke_model",
            service_response=_fake_response(fake_embedding),
            expected_params={
                "modelId": _MODEL_ID,
                "body": json.dumps({
                    "inputText": "Spring Security CSRF protection",
                    "dimensions": 1024,
                    "normalize": True,
                }),
            },
        )
        result = embed_chunks(client, chunks, _MODEL_ID)

    assert len(result) == 1
    assert len(result[0]) == 1024
    assert result[0] == fake_embedding


def test_embed_chunks_multiple_calls():
    client = _bedrock_client()
    texts = ["text one", "text two"]
    chunks = [{"contentText": t} for t in texts]
    fake_embedding = [0.5] * 1024

    with Stubber(client) as stubber:
        for t in texts:
            stubber.add_response(
                method="invoke_model",
                service_response=_fake_response(fake_embedding),
                expected_params={
                    "modelId": _MODEL_ID,
                    "body": json.dumps({"inputText": t, "dimensions": 1024, "normalize": True}),
                },
            )
        result = embed_chunks(client, chunks, _MODEL_ID)

    assert len(result) == 2
    assert result[0] == fake_embedding
    assert result[1] == fake_embedding


def test_embed_chunks_raises_on_throttle():
    client = _bedrock_client()
    chunks = [{"contentText": "test"}]

    with Stubber(client) as stubber:
        stubber.add_client_error(
            method="invoke_model",
            service_error_code="ThrottlingException",
            http_status_code=429,
        )
        with pytest.raises(ClientError) as exc_info:
            embed_chunks(client, chunks, _MODEL_ID)

    assert exc_info.value.response["Error"]["Code"] == "ThrottlingException"


def test_embed_chunks_raises_on_model_not_found():
    client = _bedrock_client()
    chunks = [{"contentText": "test"}]

    with Stubber(client) as stubber:
        stubber.add_client_error(
            method="invoke_model",
            service_error_code="ResourceNotFoundException",
            http_status_code=404,
        )
        with pytest.raises(ClientError) as exc_info:
            embed_chunks(client, chunks, _MODEL_ID)

    assert exc_info.value.response["Error"]["Code"] == "ResourceNotFoundException"
