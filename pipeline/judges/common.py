"""Shared helpers for the judges: verifying that a quote really is in a text, and how much of it quotes cover.

A model asked to cite evidence can invent a plausible-looking quote. Checking every quote against the text it
claims to come from is what makes a judge's "supported" worth anything, and it needs no model.
"""
import json
import re
import unicodedata

_QUOTES = str.maketrans({"‘": "'", "’": "'", "“": '"', "”": '"'})


def _norm(text: str) -> str:
    """Case, accents, curly quotes and whitespace folded away; nothing else changes."""
    decomposed = unicodedata.normalize("NFKD", text.translate(_QUOTES))
    stripped = "".join(c for c in decomposed if not unicodedata.combining(c))
    return re.sub(r"\s+", " ", stripped.casefold()).strip()


def quote_in(quote: str, text: str) -> bool:
    q = _norm(quote)
    return bool(q) and q in _norm(text)


def coverage(quotes: list[str], text: str) -> float:
    """Share of `text`'s words that fall inside at least one of the quotes (1.0 for an empty text)."""
    words = _norm(text).split()
    if not words:
        return 1.0
    covered = [False] * len(words)
    for quote in quotes:
        q = _norm(quote).split()
        if not q:
            continue
        for i in range(len(words) - len(q) + 1):
            if words[i : i + len(q)] == q:
                covered[i : i + len(q)] = [True] * len(q)
                break
    return sum(covered) / len(words)


def json_list(body: str) -> list:
    """A model reply that should be a JSON array, possibly inside a code fence; anything else raises ValueError."""
    text = re.sub(r"^```(?:json)?\s*|\s*```$", "", body.strip())
    try:
        data = json.loads(text)
    except ValueError as exc:
        raise ValueError(f"judge reply is not JSON: {body[:200]!r}") from exc
    if not isinstance(data, list):
        raise ValueError(f"judge reply is not a list: {body[:200]!r}")
    return data


_SENTENCE_END = re.compile(r"(?<=[.!?\u2026])\s+")


def sentence_coverage(quotes: list[str], text: str) -> float:
    """Share of `text`'s sentences that contain at least one of the quotes.

    A judge that copies short fragments covers few words but, if it did its job, touches every sentence; a
    sentence with no quote at all is one it skipped.
    """
    sentences = [_norm(s) for s in _SENTENCE_END.split(text.strip()) if s.strip()]
    if not sentences:
        return 1.0
    normed = [q for q in (_norm(q) for q in quotes) if q]
    return sum(any(q in s for q in normed) for s in sentences) / len(sentences)
