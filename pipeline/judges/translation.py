"""Judge a translation against the French narration it was translated from.

The French narration is the ground truth (it is the one checked against the real source), so EN, ES and PT are
never re-grounded: they are only checked for facts added, dropped or altered in translation. A mechanical
precheck compares the numbers and the length; the model then lists the issues, and every quote it gives is
checked against the text it claims to come from.
"""
import re
from dataclasses import dataclass

from .common import json_list, quote_in

KINDS = {"added", "dropped", "altered"}
# English and Spanish run a little shorter or longer than French; far outside this is content added or lost.
MIN_RATIO, MAX_RATIO = 0.7, 1.4

_NUMBER = re.compile(r"\d{1,3}(?:[  .,]\d{3})+|\d+(?:[.,]\d+)?")


@dataclass(frozen=True)
class Precheck:
    numbers_added: list[str]  # only in the translation
    numbers_dropped: list[str]  # only in the French
    length_ratio: float
    suspicious: bool


@dataclass(frozen=True)
class Issue:
    kind: str  # "added" | "dropped" | "altered"
    translation_quote: str | None
    fr_quote: str | None
    reason: str
    quote_verified: bool  # every quote given really is in its text


def _numbers(text: str) -> set[str]:
    """Numbers as bare digits, so 8 000 / 8,000 / 8000 and 85,4 / 85.4 are each the same number."""
    return {re.sub(r"\D", "", m) for m in _NUMBER.findall(text)}


def precheck(fr: str, translation: str) -> Precheck:
    fr_nums, tr_nums = _numbers(fr), _numbers(translation)
    added, dropped = sorted(tr_nums - fr_nums), sorted(fr_nums - tr_nums)
    ratio = len(translation) / len(fr) if fr else 0.0
    return Precheck(added, dropped, ratio, bool(added or dropped or not MIN_RATIO <= ratio <= MAX_RATIO))


def build_prompt(fr: str, translation: str, language: str, pre: Precheck | None = None) -> str:
    hints = ""
    if pre and (pre.numbers_added or pre.numbers_dropped):
        hints = (
            f"\nA mechanical check found numbers only in the translation: {pre.numbers_added}, "
            f"and only in the French: {pre.numbers_dropped}. Look at those first.\n"
        )
    return (
        f"Below is a French audio-guide narration (the ground truth) and its translation into '{language}'. "
        "List every fact that was ADDED (in the translation, not in the French), DROPPED (in the French, missing "
        "from the translation) or ALTERED (changed: a name, date, number, place or claim). Ignore style, phrasing "
        "and idiomatic choices."
        f"{hints}\n\nFRENCH:\n{fr}\n\nTRANSLATION ({language}):\n{translation}\n\n"
        'Reply with only a JSON array of {"type": "added"|"dropped"|"altered", "translation_quote": string or '
        'null, "fr_quote": string or null, "reason": short string}. Quotes are exact passages copied verbatim: '
        "added needs translation_quote, dropped needs fr_quote, altered needs both. Reply [] if there is none."
    )


def parse_issues(body: str, fr: str, translation: str) -> list[Issue]:
    out: list[Issue] = []
    for item in json_list(body):
        if not isinstance(item, dict) or item.get("type") not in KINDS:
            raise ValueError(f"bad issue entry: {item!r}")
        kind = item["type"]
        tq = item.get("translation_quote") if isinstance(item.get("translation_quote"), str) else None
        fq = item.get("fr_quote") if isinstance(item.get("fr_quote"), str) else None
        needs_t, needs_f = kind in ("added", "altered"), kind in ("dropped", "altered")
        if (needs_t and not (tq and tq.strip())) or (needs_f and not (fq and fq.strip())):
            raise ValueError(f"{kind} issue without the quote it needs: {item!r}")
        verified = (not needs_t or quote_in(tq, translation)) and (not needs_f or quote_in(fq, fr))
        out.append(Issue(kind, tq, fq, str(item.get("reason", "")), verified))
    return out


def judge_translation(fr: str, translation: str, language: str, complete, skip_clean: bool = False):
    """(issues, precheck). `skip_clean` skips the model when the numbers and length already agree: cheaper,
    but a changed name with the same numbers would go unseen, so it is off by default."""
    pre = precheck(fr, translation)
    if skip_clean and not pre.suspicious:
        return [], pre
    return parse_issues(complete(build_prompt(fr, translation, language, pre)), fr, translation), pre
