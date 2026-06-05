import boto3

from indexer import _collect_all_chunk_ids_for_ref, write_chunks_dynamo, write_keywords_dynamo


def test_write_chunks_dynamo(dynamodb_tables):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks = [
        {"chunkId": "id1", "ref": "6.5.x", "commitSha": "abc", "title": "T1", "contentText": "text"},
        {"chunkId": "id2", "ref": "6.5.x", "commitSha": "abc", "title": "T2", "contentText": "text"},
    ]
    write_chunks_dynamo(dynamo, "test-chunks", chunks)

    resp = chunks_table.scan()
    assert resp["Count"] == 2
    ids = {item["chunkId"] for item in resp["Items"]}
    assert ids == {"id1", "id2"}


def test_write_chunks_dynamo_empty(dynamodb_tables):
    dynamo, chunks_table, _ = dynamodb_tables
    write_chunks_dynamo(dynamo, "test-chunks", [])
    assert chunks_table.scan()["Count"] == 0


def test_write_keywords_dynamo_matches(dynamodb_tables):
    dynamo, _, keywords_table = dynamodb_tables
    chunks = [{
        "chunkId": "id1", "ref": "6.5.x", "area": "servlet",
        "title": "CSRF", "contentText": "csrf protection is important",
    }]
    write_keywords_dynamo(dynamo, "test-keywords", chunks)

    resp = keywords_table.scan()
    assert resp["Count"] >= 1
    keywords = {item["keyword"] for item in resp["Items"]}
    assert "csrf" in keywords


def test_write_keywords_dynamo_case_insensitive(dynamodb_tables):
    dynamo, _, keywords_table = dynamodb_tables
    chunks = [{
        "chunkId": "id1", "ref": "6.5.x", "area": "servlet",
        "title": "T", "contentText": "CSRF protection",
    }]
    write_keywords_dynamo(dynamo, "test-keywords", chunks)
    resp = keywords_table.scan()
    keywords = {item["keyword"] for item in resp["Items"]}
    assert "csrf" in keywords


def test_write_keywords_dynamo_no_match(dynamodb_tables):
    dynamo, _, keywords_table = dynamodb_tables
    chunks = [{"chunkId": "id1", "ref": "6.5.x", "area": "other", "title": "T", "contentText": "no keywords here"}]
    write_keywords_dynamo(dynamo, "test-keywords", chunks)
    assert keywords_table.scan()["Count"] == 0


def test_write_keywords_dynamo_record_structure(dynamodb_tables):
    dynamo, _, keywords_table = dynamodb_tables
    chunks = [{
        "chunkId": "id1", "ref": "6.5.x", "area": "servlet",
        "title": "Auth", "contentText": "JwtDecoder config",
    }]
    write_keywords_dynamo(dynamo, "test-keywords", chunks)

    items = keywords_table.scan()["Items"]
    item = next(i for i in items if i["keyword"] == "JwtDecoder")
    assert item["chunkId"] == "id1"
    assert item["ref"] == "6.5.x"
    assert item["area"] == "servlet"
    assert item["refAreaChunkId"] == "6.5.x#servlet#id1"


def test_collect_all_chunk_ids_for_ref_returns_all(dynamodb_tables):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "id1", "ref": "6.5.x", "commitSha": "old_sha"})
    chunks_table.put_item(Item={"chunkId": "id2", "ref": "6.5.x", "commitSha": "new_sha"})
    chunks_table.put_item(Item={"chunkId": "other", "ref": "7.0.x", "commitSha": "old_sha"})

    table = dynamo.Table("test-chunks")
    ids = _collect_all_chunk_ids_for_ref(table, "6.5.x")
    assert set(ids) == {"id1", "id2"}


def test_collect_all_chunk_ids_for_ref_empty(dynamodb_tables):
    dynamo, chunks_table, _ = dynamodb_tables
    table = dynamo.Table("test-chunks")
    assert _collect_all_chunk_ids_for_ref(table, "6.5.x") == []


def test_collect_all_chunk_ids_for_ref_ignores_other_ref(dynamodb_tables):
    dynamo, chunks_table, _ = dynamodb_tables
    chunks_table.put_item(Item={"chunkId": "other1", "ref": "7.0.x", "commitSha": "sha"})

    table = dynamo.Table("test-chunks")
    assert _collect_all_chunk_ids_for_ref(table, "6.5.x") == []
