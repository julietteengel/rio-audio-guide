import json

import pytest
import requests

from audiogen.core import FatalProviderError, PermanentItemError
from judges import anthropic_client
from judges.transcript import judge


class _Resp:
    def __init__(self, status, payload=None, text=""):
        self.status_code = status
        self.ok = status < 400
        self._payload = payload
        self.text = text or json.dumps(payload)

    def json(self):
        return self._payload


@pytest.fixture(autouse=True)
def no_sleep(monkeypatch):
    monkeypatch.setattr(anthropic_client.time, "sleep", lambda s: None)


def _ok(text):
    return _Resp(200, {"content": [{"type": "text", "text": text}]})


class TestComplete:
    def test_returns_the_text_of_the_reply(self, monkeypatch):
        monkeypatch.setattr(requests, "post", lambda *a, **k: _ok("hello"))
        assert anthropic_client.complete("hi", "key", "m") == "hello"

    def test_sends_key_version_model_and_prompt(self, monkeypatch):
        seen = {}

        def fake(url, headers, json, timeout):
            seen.update(url=url, headers=headers, body=json)
            return _ok("x")

        monkeypatch.setattr(requests, "post", fake)
        anthropic_client.complete("the prompt", "sk-test", "claude-test", max_tokens=99)
        assert seen["url"].endswith("/v1/messages")
        assert seen["headers"]["x-api-key"] == "sk-test"
        assert seen["headers"]["anthropic-version"]
        assert seen["body"]["model"] == "claude-test" and seen["body"]["max_tokens"] == 99
        assert seen["body"]["messages"] == [{"role": "user", "content": "the prompt"}]

    @pytest.mark.parametrize("status", [401, 403])
    def test_bad_key_is_fatal_and_not_retried(self, monkeypatch, status):
        calls = []
        monkeypatch.setattr(requests, "post", lambda *a, **k: calls.append(1) or _Resp(status, text="nope"))
        with pytest.raises(FatalProviderError):
            anthropic_client.complete("p", "k", "m")
        assert len(calls) == 1

    def test_other_4xx_is_permanent_and_not_retried(self, monkeypatch):
        calls = []
        monkeypatch.setattr(requests, "post", lambda *a, **k: calls.append(1) or _Resp(400, text="bad"))
        with pytest.raises(PermanentItemError):
            anthropic_client.complete("p", "k", "m")
        assert len(calls) == 1

    def test_retries_a_transient_error_then_succeeds(self, monkeypatch):
        replies = iter([_Resp(529, text="overloaded"), _ok("fine")])
        monkeypatch.setattr(requests, "post", lambda *a, **k: next(replies))
        assert anthropic_client.complete("p", "k", "m") == "fine"

    def test_gives_up_after_bounded_attempts(self, monkeypatch):
        calls = []
        monkeypatch.setattr(requests, "post", lambda *a, **k: calls.append(1) or _Resp(500, text="boom"))
        with pytest.raises(requests.HTTPError):
            anthropic_client.complete("p", "k", "m")
        assert len(calls) == 3

    def test_a_reply_cut_off_at_max_tokens_is_rejected_not_returned(self, monkeypatch):
        cut = _Resp(200, {"stop_reason": "max_tokens", "content": [{"type": "text", "text": '[{"index": 0'}]})
        monkeypatch.setattr(requests, "post", lambda *a, **k: cut)
        with pytest.raises(PermanentItemError):
            anthropic_client.complete("p", "k", "m")

    def test_an_empty_reply_is_rejected(self, monkeypatch):
        monkeypatch.setattr(requests, "post", lambda *a, **k: _Resp(200, {"content": []}))
        with pytest.raises(PermanentItemError):
            anthropic_client.complete("p", "k", "m")


class TestJudge:
    def test_pairs_each_mismatch_with_its_verdict(self):
        script = "Aqui fica a Praça XV de Novembro no centro."
        heard = "Aqui fica a Traça 15 de Novembro no centro."
        reply = json.dumps([{"index": 0, "verdict": "error", "correction": "Praça XV", "reason": "misheard"}])
        [(m, v)] = judge(script, heard, "pt", lambda prompt: reply)
        assert m.script_span == "Praça XV" and v.verdict == "error" and v.correction == "Praça XV"

    def test_no_mismatch_means_no_llm_call(self):
        def boom(prompt):
            raise AssertionError("must not call the model")

        assert judge("igual igual", "igual igual", "pt", boom) == []

    def test_large_batches_are_split_and_indexed_per_call(self):
        # five separate mismatches, kept apart by equal anchor words
        script = " ".join(f"s{i} anchor{i}" for i in range(5))
        heard = " ".join(f"t{i} anchor{i}" for i in range(5))
        calls = []

        def fake(prompt):
            n = prompt.count("differing script words")
            calls.append(n)
            return json.dumps([{"index": i, "verdict": "benign", "correction": None, "reason": ""} for i in range(n)])

        out = judge(script, heard, "pt", fake, batch=2)
        assert len(out) == 5 and calls == [2, 2, 1]

    def test_a_malformed_reply_raises(self):
        with pytest.raises(ValueError):
            judge("a b c", "a x c", "pt", lambda prompt: "not json")
