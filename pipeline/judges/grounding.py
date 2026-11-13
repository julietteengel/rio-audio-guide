"""Judge a narration against the real source it was written from.

The model splits the narration into claims and, for each, either cites a passage of the source that supports it,
or says it is unsupported or contradicted. A "supported" claim only counts if its citation is a verbatim passage
of the source: a quote the model made up downgrades the claim to unsupported. The claims must also cover most of
the narration, so a judge that skips content cannot report a place clean.
"""
from dataclasses import dataclass

from audiogen.core import split_for_tts

from .common import json_list, quote_in, sentence_coverage

STATUSES = {"supported", "unsupported", "contradicted"}
MIN_COVERAGE = 0.6


@dataclass(frozen=True)
class ClaimVerdict:
    claim: str  # a passage of the narration
    status: str  # "supported" | "unsupported" | "contradicted"
    quote: str | None  # a passage of the source: the support, or what contradicts the claim
    reason: str
    citation_verified: bool  # the quote really is in the source
    located: bool = True  # the claim is a verbatim passage of the narration (False: the model paraphrased it)


def build_prompt(script: str, source: str, language: str) -> str:
    return (
        f"Below is a narration for an audio guide (language '{language}') and the source text it must be based on. "
        "Check every factual claim of the narration against the source, and only the source: do not use what you "
        "know yourself.\n\n"
        "Go through the whole narration in order and split it into claims (a name, a date, a number, an event, a "
        "description). Skip only pure framing with no fact in it. For each claim give:\n"
        '- "claim": the exact words of the narration, copied verbatim;\n'
        '- "status": "supported" if the source says it, "contradicted" if the source says otherwise, '
        '"unsupported" if the source says nothing about it;\n'
        '- "quote": for supported or contradicted, an exact passage of the source (copied verbatim, in the '
        "source's own language) that shows it; null for unsupported;\n"
        '- "reason": a short sentence.\n\n'
        f"NARRATION:\n{script}\n\nSOURCE:\n{source}\n\n"
        'Reply with only a JSON array of {"claim", "status", "quote", "reason"} objects, in order.'
    )


def parse_claims(body: str, script: str, source: str) -> list[ClaimVerdict]:
    out: list[ClaimVerdict] = []
    for item in json_list(body):
        if not isinstance(item, dict) or not isinstance(item.get("claim"), str):
            raise ValueError(f"bad claim entry: {item!r}")
        status = item.get("status")
        if status not in STATUSES:
            raise ValueError(f"unknown status {status!r}")
        quote = item.get("quote") if isinstance(item.get("quote"), str) else None
        reason = str(item.get("reason", ""))
        verified = bool(quote) and quote_in(quote, source)
        if status == "supported" and not verified:
            status, reason = "unsupported", f"citation not found in source ({reason})".strip()
        out.append(ClaimVerdict(item["claim"], status, quote, reason, verified, quote_in(item["claim"], script)))
    return out


def summarize(claims: list[ClaimVerdict], coverage_ratio: float, min_coverage: float = MIN_COVERAGE) -> dict:
    counts = {s: sum(c.status == s for c in claims) for s in STATUSES}
    incomplete = coverage_ratio < min_coverage
    return {
        **counts,
        "coverage": round(coverage_ratio, 3),
        "incomplete": incomplete,
        "clean": bool(claims) and counts["unsupported"] == 0 and counts["contradicted"] == 0 and not incomplete,
    }


def judge_grounding(
    script: str, source: str, language: str, complete, chunk_chars: int = 1800
) -> tuple[list[ClaimVerdict], dict]:
    """Judge the narration block by block (each against the whole source), so no single reply has to list
    every claim of a long narration. Claims are still checked against the full narration, and coverage
    is measured over all of it."""
    claims: list[ClaimVerdict] = []
    for block in split_for_tts(script, chunk_chars):
        prompt = build_prompt(block, source, language)
        try:
            parsed = parse_claims(complete(prompt), script, source)
        except ValueError:  # a malformed reply: ask once more before giving up on the place
            parsed = parse_claims(complete(prompt), script, source)
        claims.extend(parsed)
    located = [c.claim for c in claims if c.located]
    return claims, summarize(claims, sentence_coverage(located, script))
