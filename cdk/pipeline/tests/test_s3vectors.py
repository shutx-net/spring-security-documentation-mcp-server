from unittest.mock import MagicMock, call

from indexer import VECTOR_BATCH_SIZE, put_vectors

_INDEX_ARN = "arn:aws:s3vectors:ap-northeast-1:123456789012:bucket/test/index/idx"


def _chunks(n: int) -> list[dict]:
    return [{"chunkId": f"id{i}", "ref": "6.5.x", "area": "servlet", "docType": "reference"} for i in range(n)]


def _embeddings(n: int) -> list[list[float]]:
    return [[0.1] * 1024 for _ in range(n)]


def test_put_vectors_calls_api():
    mock = MagicMock()
    put_vectors(mock, _INDEX_ARN, _chunks(3), _embeddings(3))
    mock.put_vectors.assert_called_once()
    _, kwargs = mock.put_vectors.call_args
    assert kwargs["indexArn"] == _INDEX_ARN
    assert len(kwargs["vectors"]) == 3


def test_put_vectors_vector_structure():
    mock = MagicMock()
    put_vectors(mock, _INDEX_ARN, _chunks(1), _embeddings(1))
    _, kwargs = mock.put_vectors.call_args
    v = kwargs["vectors"][0]
    assert v["key"] == "id0"
    assert "float32" in v["data"]
    assert len(v["data"]["float32"]) == 1024
    assert v["metadata"]["ref"] == "6.5.x"
    assert v["metadata"]["area"] == "servlet"
    assert v["metadata"]["docType"] == "reference"
    assert v["metadata"]["chunkId"] == "id0"


def test_put_vectors_batches_large_input():
    n = VECTOR_BATCH_SIZE + 1
    mock = MagicMock()
    put_vectors(mock, _INDEX_ARN, _chunks(n), _embeddings(n))
    assert mock.put_vectors.call_count == 2


def test_put_vectors_batch_sizes_are_correct():
    n = VECTOR_BATCH_SIZE + 10
    mock = MagicMock()
    put_vectors(mock, _INDEX_ARN, _chunks(n), _embeddings(n))

    calls = mock.put_vectors.call_args_list
    assert len(calls[0][1]["vectors"]) == VECTOR_BATCH_SIZE
    assert len(calls[1][1]["vectors"]) == 10


def test_put_vectors_empty_input():
    mock = MagicMock()
    put_vectors(mock, _INDEX_ARN, [], [])
    mock.put_vectors.assert_not_called()
