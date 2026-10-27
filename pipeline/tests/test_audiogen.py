import pytest
import requests

from audiogen import openai_client
from audiogen.core import (
    FatalProviderError,
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
