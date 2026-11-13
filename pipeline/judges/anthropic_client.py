"""Thin Anthropic Messages API wrapper with bounded retries (same shape as audiogen.openai_client)."""
import time

import requests

from audiogen.core import FatalProviderError, PermanentItemError

_URL = "https://api.anthropic.com/v1/messages"
_VERSION = "2023-06-01"
_MAX_ATTEMPTS = 3
_BACKOFF_S = 2.0


def complete(prompt: str, api_key: str, model: str, max_tokens: int = 16000) -> str:
    """One user prompt in, the reply text out. Bad key stops the run, other 4xx rejects the item."""
    last: Exception | None = None
    for attempt in range(1, _MAX_ATTEMPTS + 1):
        try:
            resp = requests.post(
                _URL,
                headers={"x-api-key": api_key, "anthropic-version": _VERSION, "content-type": "application/json"},
                json={"model": model, "max_tokens": max_tokens, "messages": [{"role": "user", "content": prompt}]},
                timeout=120,
            )
        except requests.RequestException as exc:  # network blip: transient
            last = exc
        else:
            if resp.ok:
                payload = resp.json()
                if payload.get("stop_reason") == "max_tokens":
                    # Thinking tokens count against max_tokens; a cut-off reply must not reach a JSON parser.
                    raise PermanentItemError("anthropic reply was cut off at max_tokens")
                blocks = [b.get("text", "") for b in payload.get("content", []) if b.get("type") == "text"]
                if not any(blocks):
                    raise PermanentItemError("anthropic returned an empty reply")
                return "".join(blocks)
            if resp.status_code in (401, 403):
                raise FatalProviderError(f"{resp.status_code}: {resp.text[:200]}")
            if resp.status_code < 500 and resp.status_code != 429:
                raise PermanentItemError(f"{resp.status_code}: {resp.text[:200]}")
            last = requests.HTTPError(f"{resp.status_code}: {resp.text[:200]}")
        if attempt < _MAX_ATTEMPTS:
            time.sleep(_BACKOFF_S * attempt)
    raise last  # type: ignore[misc]
