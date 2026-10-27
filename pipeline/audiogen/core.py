"""Pure logic for audio generation -- no network, no DB, so it is unit-tested directly."""
import re

# OpenAI's /audio/speech rejects input over 4096 characters; stay under it.
MAX_TTS_CHARS = 4000

_SENTENCE_END = re.compile(r"(?<=[.!?…])\s+")


class FatalProviderError(Exception):
    """The provider will keep failing for every remaining script (bad key, no credit).

    Stops the whole run instead of retrying each script -- the same
    permanent-vs-transient split the Go worker makes with ports.PermanentError.
    """


class PermanentItemError(Exception):
    """This one script/audio can never succeed as it is (rejected input, bad audio).

    Unlike FatalProviderError the rest of the batch is fine, so the run goes on;
    unlike a transient error it is not worth trying again on the next run.
    """


def split_for_tts(text: str, limit: int = MAX_TTS_CHARS) -> list[str]:
    """Split on sentence boundaries into chunks of at most `limit` characters."""
    chunks: list[str] = []
    current = ""
    for sentence in _SENTENCE_END.split(text.strip()):
        for piece in _hard_split(sentence, limit):
            if current and len(current) + 1 + len(piece) > limit:
                chunks.append(current)
                current = piece
            else:
                current = f"{current} {piece}".strip()
    if current:
        chunks.append(current)
    return chunks


def _hard_split(sentence: str, limit: int) -> list[str]:
    """A single sentence longer than the limit is split on whitespace."""
    if len(sentence) <= limit:
        return [sentence] if sentence else []
    pieces, current = [], ""
    for word in sentence.split():
        if current and len(current) + 1 + len(word) > limit:
            pieces.append(current)
            current = word
        else:
            current = f"{current} {word}".strip()
    if current:
        pieces.append(current)
    return pieces


def voice_label(provider: str, voice: str) -> str:
    """Stored in audio_files.voice_id and used as an S3 key segment."""
    return f"{provider}-{voice}"


def audio_key(label: str, script_id: str) -> str:
    return f"audio/{label}/{script_id}.mp3"


def timestamps_key(label: str, audio_file_id: str) -> str:
    return f"timestamps/{label}/{audio_file_id}.json"


def storage_url(bucket: str, key: str) -> str:
    # The backend's parseS3Key requires exactly this s3://bucket/key shape.
    return f"s3://{bucket}/{key}"


def whisper_language(script_language: str) -> str | None:
    """Whisper wants an ISO-639-1 code; anything else is left to auto-detection."""
    code = script_language.strip().lower()[:2]
    return code if code in {"en", "fr", "pt", "es"} else None


def whisper_prompt(script_text: str, max_chars: int = 500) -> str:
    """Whisper's prompt biases spelling of proper nouns toward the script's own.

    Added because Whisper once heard "Praça XV" as "Traça 15" across a whole
    narration (docs/superpowers/specs/2026-10-18-live-testing-session-status.md).
    """
    return script_text.strip()[:max_chars]


def parse_s3_key(url: str) -> str:
    """Inverse of storage_url: "s3://bucket/some/key" -> "some/key"."""
    rest = url.removeprefix("s3://")
    if rest == url or "/" not in rest or rest.endswith("/"):
        raise ValueError(f"not an s3://bucket/key url: {url!r}")
    return rest.split("/", 1)[1]


def validate_whisper(body: bytes) -> float:
    """Returns the audio duration (seconds) if body is a usable verbose_json, else raises.

    A 200 with an empty or segment-less body would otherwise be stored as
    timestamps_url, and since that column is what marks an audio as done it
    could then never be repaired by a later run.
    """
    import json

    try:
        data = json.loads(body)
    except ValueError as exc:
        raise ValueError(f"whisper body is not JSON: {body[:200]!r}") from exc
    segments = data.get("segments") if isinstance(data, dict) else None
    duration = data.get("duration") if isinstance(data, dict) else None
    if not isinstance(segments, list) or not segments:
        raise ValueError(f"whisper body has no segments: {body[:200]!r}")
    if not isinstance(duration, (int, float)) or duration <= 0:
        raise ValueError(f"whisper body has no usable duration: {body[:200]!r}")
    return float(duration)
