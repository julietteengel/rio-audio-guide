import json

import pytest

from judges.judge_transcripts import _write_fixed


class _ClientError(Exception):
    pass


class _FakeS3:
    """Just enough of boto3's S3 client: objects in a dict, head/copy/put."""

    class exceptions:
        ClientError = _ClientError

    def __init__(self, objects):
        self.objects = dict(objects)
        self.puts = []

    def head_object(self, Bucket, Key):
        if Key not in self.objects:
            raise _ClientError("404")

    def copy_object(self, Bucket, Key, CopySource):
        self.objects[Key] = self.objects[CopySource["Key"]]

    def put_object(self, Bucket, Key, Body, ContentType):
        self.puts.append(Key)
        self.objects[Key] = Body


GOOD = {"duration": 5.0, "segments": [{"id": 0, "start": 0.0, "end": 5.0, "text": " Praça XV"}]}


def test_backs_up_the_original_then_writes_the_fix():
    s3 = _FakeS3({"k.json": b"ORIGINAL"})
    _write_fixed(s3, "b", "k.json", GOOD)
    assert s3.objects["k.json.orig"] == b"ORIGINAL"
    assert json.loads(s3.objects["k.json"])["segments"][0]["text"] == " Praça XV"


def test_never_overwrites_an_existing_backup():
    s3 = _FakeS3({"k.json": b"ALREADY FIXED ONCE", "k.json.orig": b"THE REAL ORIGINAL"})
    _write_fixed(s3, "b", "k.json", GOOD)
    assert s3.objects["k.json.orig"] == b"THE REAL ORIGINAL"


def test_refuses_to_write_a_file_the_app_could_not_parse():
    s3 = _FakeS3({"k.json": b"ORIGINAL"})
    with pytest.raises(ValueError):
        _write_fixed(s3, "b", "k.json", {"duration": 5.0, "segments": []})
    assert s3.puts == [] and s3.objects["k.json"] == b"ORIGINAL"
