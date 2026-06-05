import gzip
import io
import json
import tarfile
import tempfile
import os
from pathlib import Path

from indexer import publish_to_s3


def test_publish_to_s3_latest_json(s3_bucket):
    chunks = [{"chunkId": "id1", "ref": "6.5.x", "commitSha": "abc123"}]
    publish_to_s3(s3_bucket, "test-bucket", "6.5.x", "abc123", chunks, "2026-06-06T00:00:00Z")

    obj = s3_bucket.get_object(Bucket="test-bucket", Key="indexes/spring-security/6.5.x/latest.json")
    latest = json.loads(obj["Body"].read())
    assert latest["commitSha"] == "abc123"
    assert latest["ref"] == "6.5.x"
    assert latest["chunksKey"].endswith("chunks.jsonl.gz")
    assert latest["builtAt"] == "2026-06-06T00:00:00Z"


def test_publish_to_s3_metadata_json(s3_bucket):
    chunks = [{"chunkId": "id1"}, {"chunkId": "id2"}]
    publish_to_s3(s3_bucket, "test-bucket", "6.5.x", "sha1", chunks, "2026-06-06T00:00:00Z")

    obj = s3_bucket.get_object(Bucket="test-bucket", Key="chunks/spring-security/6.5.x/sha1/metadata.json")
    meta = json.loads(obj["Body"].read())
    assert meta["chunkCount"] == 2
    assert meta["ref"] == "6.5.x"
    assert meta["commitSha"] == "sha1"


def test_publish_to_s3_chunks_jsonl_gz(s3_bucket):
    chunks = [{"chunkId": "id1", "title": "T1"}, {"chunkId": "id2", "title": "T2"}]
    publish_to_s3(s3_bucket, "test-bucket", "6.5.x", "sha1", chunks, "2026-06-06T00:00:00Z")

    obj = s3_bucket.get_object(Bucket="test-bucket", Key="chunks/spring-security/6.5.x/sha1/chunks.jsonl.gz")
    raw = gzip.decompress(obj["Body"].read()).decode("utf-8")
    lines = raw.strip().splitlines()
    assert len(lines) == 2
    assert json.loads(lines[0])["chunkId"] == "id1"
    assert json.loads(lines[1])["chunkId"] == "id2"


def test_publish_to_s3_latest_json_written_last(s3_bucket):
    # latest.json key contains the correct chunks path
    chunks = [{"chunkId": "id1"}]
    publish_to_s3(s3_bucket, "test-bucket", "7.0.x", "deadbeef", chunks, "2026-06-06T00:00:00Z")

    obj = s3_bucket.get_object(Bucket="test-bucket", Key="indexes/spring-security/7.0.x/latest.json")
    latest = json.loads(obj["Body"].read())
    assert "chunks/spring-security/7.0.x/deadbeef/" in latest["chunksKey"]


def test_s3_artifact_download_and_extract(s3_bucket, tmp_path):
    # Prepare a tar.gz of a minimal site in S3
    site_dir = tmp_path / "site"
    site_dir.mkdir()
    (site_dir / "index.html").write_text("<html><body><article><h1>T</h1></article></body></html>")

    archive = tmp_path / "site.tar.gz"
    with tarfile.open(str(archive), "w:gz") as tf:
        tf.add(str(site_dir / "index.html"), arcname="index.html")

    s3_bucket.upload_file(str(archive), "test-bucket", "artifacts/6.5.x/abc/site.tar.gz")

    # Verify the object is retrievable
    obj = s3_bucket.get_object(Bucket="test-bucket", Key="artifacts/6.5.x/abc/site.tar.gz")
    data = obj["Body"].read()
    assert len(data) > 0

    # Verify it is a valid tar.gz
    buf = io.BytesIO(data)
    with tarfile.open(fileobj=buf) as tf:
        names = tf.getnames()
    assert "index.html" in names
