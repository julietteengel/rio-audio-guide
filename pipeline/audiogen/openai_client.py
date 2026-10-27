"""Thin OpenAI REST wrappers (TTS + Whisper) with bounded retries."""
import time

import requests

from .core import FatalProviderError

_BASE = "https://api.openai.com/v1"
_MAX_ATTEMPTS = 3
_BACKOFF_S = 2.0


def _post(path: str, api_key: str, **kwargs) -> requests.Response:
    last: Exception | None = None
    for attempt in range(1, _MAX_ATTEMPTS + 1):
        try:
            resp = requests.post(
                f"{_BASE}{path}",
                headers={"Authorization": f"Bearer {api_key}"},
                timeout=180,
                **kwargs,
            )
        except requests.RequestException as exc:  # network blip: transient
            last = exc
        else:
            if resp.ok:
                return resp
            if resp.status_code in (401, 403) or (
                resp.status_code == 429 and "insufficient_quota" in resp.text
            ):
                raise FatalProviderError(f"{resp.status_code}: {resp.text[:200]}")
            if resp.status_code < 500 and resp.status_code != 429:
                resp.raise_for_status()  # other 4xx: permanent for this item
            last = requests.HTTPError(f"{resp.status_code}: {resp.text[:200]}")
        if attempt < _MAX_ATTEMPTS:
            time.sleep(_BACKOFF_S * attempt)
    raise last  # type: ignore[misc]


def synthesize(text: str, api_key: str, model: str, voice: str) -> bytes:
    resp = _post(
        "/audio/speech",
        api_key,
        json={"model": model, "voice": voice, "input": text, "response_format": "mp3"},
    )
    return resp.content


def transcribe_segments(
    audio: bytes, api_key: str, language: str | None, prompt: str
) -> bytes:
    """Returns the raw verbose_json body (the mobile app parses its `segments`)."""
    data = {
        "model": "whisper-1",
        "response_format": "verbose_json",
        "timestamp_granularities[]": "segment",
        "prompt": prompt,
    }
    if language:
        data["language"] = language
    resp = _post(
        "/audio/transcriptions",
        api_key,
        data=data,
        files={"file": ("audio.mp3", audio, "audio/mpeg")},
    )
    return resp.content
