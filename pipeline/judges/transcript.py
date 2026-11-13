"""Judge for Whisper transcripts of narration audio, in two layers.

The audio is generated from a script, so the script is what was spoken and the
transcript is the part that can be wrong (Whisper once heard "Praça XV" as
"Traça 15" across a whole narration). Layer 1 is a pure word alignment between
the two that keeps only real differences. Layer 2 asks an LLM to decide, for
each remaining difference, whether it matters and what the transcript should
say. Splitting it this way is what makes the judge usable: comparing every
capitalized word, as the first audit did, produced 481 findings that were
almost all noise.

Everything here is pure (no network, no DB) so it is unit-tested directly.
"""
import copy
import json
import re
import unicodedata
from dataclasses import dataclass
from difflib import SequenceMatcher

VERDICTS = {"benign", "error"}

_EDGE_PUNCT = re.compile(r"^\W+|\W+$")
_ROMAN = re.compile(r"^[IVXLCDM]+$")
_ROMAN_VALUES = {"I": 1, "V": 5, "X": 10, "L": 50, "C": 100, "D": 500, "M": 1000}


@dataclass(frozen=True)
class Word:
    shown: str  # the word without surrounding punctuation
    norm: str  # lower-case, accent-free, alphanumeric only: what gets compared
    seg: int  # index of the Whisper segment it came from (0 for plain text)
    start: int  # character span of the raw word (with punctuation) in its segment's text
    end: int


@dataclass(frozen=True)
class Mismatch:
    kind: str  # "replace" | "missing" (in the transcript) | "extra" (in the transcript)
    script_span: str
    transcript_span: str
    script_context: str
    transcript_context: str
    t_start: int = 0  # position of the differing words among the transcript's words
    t_end: int = 0


@dataclass(frozen=True)
class Verdict:
    index: int
    verdict: str  # "benign" | "error"
    correction: str | None  # the model's proposal, informational: fixes use the script's own words
    reason: str


def _normalize(word: str) -> str:
    folded = unicodedata.normalize("NFKD", word).encode("ascii", "ignore").decode()
    return re.sub(r"[^a-z0-9]", "", folded.lower())


def _words(text: str, seg: int = 0) -> list[Word]:
    """Words of `text`, dropping tokens that are only punctuation."""
    out: list[Word] = []
    for m in re.finditer(r"\S+", text):
        shown = _EDGE_PUNCT.sub("", m.group())
        norm = _normalize(shown)
        if norm:
            out.append(Word(shown, norm, seg, m.start(), m.end()))
    return out


def _segment_words(segments: list[dict]) -> list[Word]:
    return [w for i, seg in enumerate(segments) for w in _words(seg.get("text", ""), i)]


def _roman_value(word: str) -> int | None:
    """Value of an upper-case roman numeral like "XV"; None for anything else.

    Lower-case and the lone "I" are rejected so ordinary words ("vi", "mil",
    "I") are never read as numbers.
    """
    if not _ROMAN.match(word) or word == "I":
        return None
    total = 0
    for i, ch in enumerate(word):
        value = _ROMAN_VALUES[ch]
        nxt = _ROMAN_VALUES[word[i + 1]] if i + 1 < len(word) else 0
        total += -value if value < nxt else value
    return total if _to_roman(total) == word else None


def _to_roman(n: int) -> str:
    out = ""
    for value, symbol in [(1000, "M"), (900, "CM"), (500, "D"), (400, "CD"), (100, "C"), (90, "XC"),
                          (50, "L"), (40, "XL"), (10, "X"), (9, "IX"), (5, "V"), (4, "IV"), (1, "I")]:
        while n >= value:
            out += symbol
            n -= value
    return out


def _same_token(a: Word, b: Word) -> bool:
    if a.norm == b.norm:
        return True
    for roman, digits in ((a.shown, b.norm), (b.shown, a.norm)):
        value = _roman_value(roman)
        if value is not None and digits.isdigit() and int(digits) == value:
            return True
    return False


def _clip(words: list[Word], start: int, end: int, context: int) -> str:
    return " ".join(w.shown for w in words[max(0, start - context) : end + context])


def _align(s_words: list[Word], t_words: list[Word], context_words: int) -> list[Mismatch]:
    out: list[Mismatch] = []
    matcher = SequenceMatcher(None, [w.norm for w in s_words], [w.norm for w in t_words], autojunk=False)
    for tag, i1, i2, j1, j2 in matcher.get_opcodes():
        if tag == "equal":
            continue
        if tag == "replace" and i2 - i1 == j2 - j1 and all(
            _same_token(s_words[i1 + k], t_words[j1 + k]) for k in range(i2 - i1)
        ):
            continue
        out.append(
            Mismatch(
                kind={"replace": "replace", "delete": "missing", "insert": "extra"}[tag],
                script_span=" ".join(w.shown for w in s_words[i1:i2]),
                transcript_span=" ".join(w.shown for w in t_words[j1:j2]),
                script_context=_clip(s_words, i1, i2, context_words),
                transcript_context=_clip(t_words, j1, j2, context_words),
                t_start=j1,
                t_end=j2,
            )
        )
    return out


def find_mismatches(script: str, transcript: str, context_words: int = 6) -> list[Mismatch]:
    """Real differences between a script and the transcript of its audio.

    Case, accents, punctuation and roman-vs-arabic numerals ("XV" / "15") are
    not differences.
    """
    return _align(_words(script), _words(transcript), context_words)


def find_mismatches_in_body(script: str, whisper_body: dict, context_words: int = 6) -> list[Mismatch]:
    """Like find_mismatches, but against Whisper's segments, so each difference knows where it sits."""
    return _align(_words(script), _segment_words(whisper_body.get("segments", [])), context_words)


def build_prompt(mismatches: list[Mismatch], language: str) -> str:
    items = "\n".join(
        f"[{i}] {m.kind}\n    script:     ...{m.script_context}...\n    transcript: ...{m.transcript_context}...\n"
        f"    differing script words: {m.script_span!r}; differing transcript words: {m.transcript_span!r}"
        for i, m in enumerate(mismatches)
    )
    return (
        f"A narration in language '{language}' was generated by text-to-speech from a script, then the audio was "
        "transcribed with Whisper. The script is what was actually spoken, so any difference is a transcription "
        "mistake. Each item below is a place where the transcript differs from the script.\n\n"
        "For each item decide:\n"
        '- "benign": spelling, number or date formatting, punctuation, or a harmless inflection that changes no name, '
        "date, number or fact.\n"
        '- "error": a person, place or institution name, a date, a number or a fact was misheard or changed.\n'
        'For an "error", give in "correction" the exact script words the transcript should say instead.\n\n'
        f"{items}\n\n"
        'Reply with only a JSON array, one object per item, in order: '
        '[{"index": 0, "verdict": "benign" or "error", "correction": string or null, "reason": short string}]'
    )


def parse_verdicts(body: str, expected: int) -> list[Verdict]:
    """Strictly parse the judge's reply; any malformed or incomplete answer raises ValueError."""
    text = body.strip()
    text = re.sub(r"^```(?:json)?\s*|\s*```$", "", text)
    try:
        data = json.loads(text)
    except ValueError as exc:
        raise ValueError(f"judge reply is not JSON: {body[:200]!r}") from exc
    if not isinstance(data, list):
        raise ValueError(f"judge reply is not a list: {body[:200]!r}")
    verdicts: list[Verdict] = []
    for item in data:
        if not isinstance(item, dict) or not isinstance(item.get("index"), int):
            raise ValueError(f"bad verdict entry: {item!r}")
        verdict, correction = item.get("verdict"), item.get("correction")
        if verdict not in VERDICTS:
            raise ValueError(f"unknown verdict {verdict!r}")
        verdicts.append(Verdict(item["index"], verdict, correction, str(item.get("reason", ""))))
    if sorted(v.index for v in verdicts) != list(range(expected)):
        raise ValueError(f"expected verdicts for exactly items 0..{expected - 1}, got {[v.index for v in verdicts]}")
    return sorted(verdicts, key=lambda v: v.index)


def apply_fixes(
    whisper_body: dict, verdicts: list[tuple[Mismatch, Verdict]]
) -> tuple[dict, int, list[tuple[Mismatch, Verdict]]]:
    """Fix the misheard words of "error" verdicts inside Whisper's segments, leaving all timing alone.

    The text written is the script's own words at that spot (`script_span`), never
    what the model proposed, since the script is what the voice read. Each fix is
    made at the exact words where the difference was found (never a
    search-and-replace, which would also hit legitimate uses of a short word
    like "O" or "Alma"). Only "replace" differences inside one segment are
    applied; a difference spanning two segments, a missing or extra word, or
    a position that no longer holds the words that were judged is returned
    unapplied instead of guessed at.

    Returns (fixed copy, number applied, unapplied verdicts).
    """
    fixed = copy.deepcopy(whisper_body)
    segments = fixed.get("segments", [])
    words = _segment_words(segments)
    unapplied: list[tuple[Mismatch, Verdict]] = []
    todo: list[tuple[Mismatch, Verdict]] = []
    for m, v in verdicts:
        if v.verdict != "error":
            continue
        run = words[m.t_start : m.t_end]
        in_place = (
            m.kind == "replace"
            and run
            and len({w.seg for w in run}) == 1
            and " ".join(w.shown for w in run) == m.transcript_span
        )
        (todo if in_place else unapplied).append((m, v))
    applied = 0
    # Right to left, so an edit never shifts the character offsets of one still to do.
    for m, v in sorted(todo, key=lambda mv: -mv[0].t_start):
        run = words[m.t_start : m.t_end]
        seg = segments[run[0].seg]
        text = seg["text"]
        first = run[0]
        last = run[-1]
        raw_first, raw_last = text[first.start : first.end], text[last.start : last.end]
        lead = re.match(r"\W*", raw_first).group()  # punctuation the correction must not eat
        trail = re.search(r"\W*$", raw_last).group()
        seg["text"] = text[: first.start] + lead + m.script_span + trail + text[last.end :]
        applied += 1
    joined = " ".join(seg.get("text", "").strip() for seg in whisper_body.get("segments", []))
    if applied and whisper_body.get("text", "").strip() == joined:
        fixed["text"] = " ".join(seg.get("text", "").strip() for seg in segments)
    return fixed, applied, unapplied


def judge(script: str, transcript, language: str, complete, batch: int = 40) -> list[tuple[Mismatch, Verdict]]:
    """Run both layers: align, then have the model rule on each difference.

    `transcript` is plain text or a Whisper verbose_json dict (then every
    difference carries its position, which apply_fixes needs). `complete` is a
    prompt -> reply-text callable. No differences means no model call. Identical
    differences (the same misheard name over and over) are judged once and share
    the verdict, and the rest go in batches so a long narration cannot overflow
    a single reply.
    """
    mismatches = (
        find_mismatches_in_body(script, transcript) if isinstance(transcript, dict) else find_mismatches(script, transcript)
    )
    unique: dict[tuple[str, str, str], Mismatch] = {}
    for m in mismatches:
        unique.setdefault((m.kind, m.script_span, m.transcript_span), m)
    firsts = list(unique.values())
    ruled: dict[tuple[str, str, str], Verdict] = {}
    for start in range(0, len(firsts), batch):
        chunk = firsts[start : start + batch]
        verdicts = parse_verdicts(complete(build_prompt(chunk, language)), len(chunk))
        for m, v in zip(chunk, verdicts):
            ruled[(m.kind, m.script_span, m.transcript_span)] = v
    return [(m, ruled[(m.kind, m.script_span, m.transcript_span)]) for m in mismatches]
