import pytest
import requests

from audiogen import openai_client
from audiogen.core import (
    FatalProviderError,
    PermanentItemError,
    parse_s3_key,
    validate_whisper,
    audio_key,
    split_for_tts,
    storage_url,
    timestamps_key,
    voice_label,
    whisper_language,
    whisper_prompt,
)


def test_split_for_tts_keeps_short_text_whole():
    assert split_for_tts("Uma frase. Outra frase.") == ["Uma frase. Outra frase."]


def test_split_for_tts_chunks_on_sentence_boundaries_within_limit():
    text = " ".join(f"Sentence number {i} ends here." for i in range(200))
    chunks = split_for_tts(text, limit=500)
    assert len(chunks) > 1
    assert all(len(c) <= 500 for c in chunks)
    assert " ".join(chunks) == text  # nothing lost, nothing duplicated
    assert all(c.endswith(".") for c in chunks)  # never cut mid-sentence


def test_split_for_tts_hard_splits_a_single_oversized_sentence():
    text = "palavra " * 300  # no sentence punctuation at all
    chunks = split_for_tts(text, limit=100)
    assert all(len(c) <= 100 for c in chunks)
    assert " ".join(chunks).split() == text.split()


def test_split_for_tts_empty_text():
    assert split_for_tts("   ") == []


def test_keys_and_urls_match_the_backend_contract():
    label = voice_label("openai", "onyx")
    assert label == "openai-onyx"
    assert audio_key(label, "s1") == "audio/openai-onyx/s1.mp3"
    assert timestamps_key(label, "a1") == "timestamps/openai-onyx/a1.json"
    # backend parseS3Key: "s3://" prefix, then bucket, then a non-empty key
    assert storage_url("rio-audio-guide", "audio/x.mp3") == "s3://rio-audio-guide/audio/x.mp3"


def test_whisper_language_maps_supported_codes_only():
    assert whisper_language("pt") == "pt"
    assert whisper_language("FR-fr") == "fr"
    assert whisper_language("de") is None


def test_whisper_prompt_is_capped():
    assert len(whisper_prompt("x" * 5000)) == 500


class _Resp:
    def __init__(self, status, text="", content=b""):
        self.status_code, self.text, self.content = status, text, content
        self.ok = status < 400

    def raise_for_status(self):
        raise requests.HTTPError(f"{self.status_code}")


@pytest.fixture(autouse=True)
def _no_sleep(monkeypatch):
    monkeypatch.setattr(openai_client.time, "sleep", lambda _s: None)


def test_synthesize_returns_audio_bytes(monkeypatch):
    monkeypatch.setattr(openai_client.requests, "post", lambda *a, **k: _Resp(200, content=b"mp3"))
    assert openai_client.synthesize("oi", "key", "tts-1", "onyx") == b"mp3"


def test_exhausted_credit_is_fatal_and_not_retried(monkeypatch):
    calls = []

    def post(*a, **k):
        calls.append(1)
        return _Resp(429, text='{"error":{"code":"insufficient_quota"}}')

    monkeypatch.setattr(openai_client.requests, "post", post)
    with pytest.raises(FatalProviderError):
        openai_client.synthesize("oi", "key", "tts-1", "onyx")
    assert len(calls) == 1


def test_bad_key_is_fatal(monkeypatch):
    monkeypatch.setattr(openai_client.requests, "post", lambda *a, **k: _Resp(401, text="bad key"))
    with pytest.raises(FatalProviderError):
        openai_client.synthesize("oi", "key", "tts-1", "onyx")


def test_server_errors_retry_a_bounded_number_of_times(monkeypatch):
    calls = []

    def post(*a, **k):
        calls.append(1)
        return _Resp(503, text="down")

    monkeypatch.setattr(openai_client.requests, "post", post)
    with pytest.raises(requests.HTTPError):
        openai_client.synthesize("oi", "key", "tts-1", "onyx")
    assert len(calls) == 3  # never an unbounded loop


def test_transient_failure_then_success(monkeypatch):
    responses = iter([_Resp(500, text="x"), _Resp(200, content=b"ok")])
    monkeypatch.setattr(openai_client.requests, "post", lambda *a, **k: next(responses))
    assert openai_client.synthesize("oi", "key", "tts-1", "onyx") == b"ok"


def test_other_4xx_is_a_permanent_item_error_that_keeps_openai_reason(monkeypatch):
    monkeypatch.setattr(
        openai_client.requests, "post", lambda *a, **k: _Resp(400, text='{"error":"invalid voice"}')
    )
    with pytest.raises(PermanentItemError, match="invalid voice"):
        openai_client.synthesize("oi", "key", "tts-1", "nope")


def test_parse_s3_key_inverts_storage_url():
    assert parse_s3_key(storage_url("bucket", "audio/x/y.mp3")) == "audio/x/y.mp3"
    for bad in ["https://x/y", "s3://bucket", "s3://bucket/", "audio/x.mp3"]:
        with pytest.raises(ValueError):
            parse_s3_key(bad)


def test_validate_whisper_accepts_a_real_looking_body_and_returns_duration():
    body = b'{"duration": 12.5, "segments": [{"start": 0.0, "text": "Oi."}]}'
    assert validate_whisper(body) == 12.5


@pytest.mark.parametrize(
    "body",
    [b"", b"not json", b"[]", b'{"duration": 5}', b'{"duration": 5, "segments": []}',
     b'{"segments": [{"start": 0, "text": "x"}]}', b'{"duration": 0, "segments": [{"start": 0}]}'],
)
def test_validate_whisper_rejects_unusable_bodies(body):
    with pytest.raises(ValueError):
        validate_whisper(body)


class _Cursor:
    def __init__(self, rowcount=1, row=None):
        self.rowcount, self._row = rowcount, row

    def fetchone(self):
        return self._row


class _FakeConn:
    """Records statements; the script UPDATE reports `script_rowcount` rows."""

    def __init__(self, script_rowcount):
        self.script_rowcount, self.statements, self.committed = script_rowcount, [], None

    def transaction(self):
        conn = self

        class _Tx:
            def __enter__(self):
                return conn

            def __exit__(self, exc_type, *_):
                conn.committed = exc_type is None
                return False

        return _Tx()

    def execute(self, sql, params=()):
        self.statements.append(sql)
        if "UPDATE scripts" in sql:
            return _Cursor(rowcount=self.script_rowcount)
        return _Cursor(row=None)


def test_save_ready_publishes_the_script_in_the_same_transaction():
    from audiogen.generate import save_ready

    conn = _FakeConn(script_rowcount=1)
    save_ready(conn, "s1", "openai-onyx", "s3://b/audio/x.mp3", 1000)
    assert conn.committed is True
    assert any("INSERT INTO audio_files" in s for s in conn.statements)
    assert any("UPDATE scripts" in s for s in conn.statements)


def test_save_ready_rolls_back_when_the_script_does_not_exist():
    from audiogen.generate import save_ready

    conn = _FakeConn(script_rowcount=0)
    with pytest.raises(RuntimeError, match="not found"):
        save_ready(conn, "ghost", "openai-onyx", "s3://b/audio/x.mp3", 1000)
    assert conn.committed is False  # the exception unwound the transaction
