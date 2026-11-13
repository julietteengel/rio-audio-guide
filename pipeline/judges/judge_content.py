"""Judge narration scripts: against their real source (grounding) or against the French (translation).

    python -m judges.judge_content grounding --limit 3                  # dry run: what would be judged
    python -m judges.judge_content grounding --apply --place Botanico   # ask the model, write a JSON report
    python -m judges.judge_content translation --apply --language en --place Maracan

Report only: nothing is written to the database. Grounding sources come from the curation CSV (the database
does not keep them: scripts.source_text is empty); a place with no source is counted as unverifiable, never as
clean. Env: DATABASE_URL, ANTHROPIC_API_KEY.
"""
import argparse
import csv
import json
import os
import sys
from pathlib import Path

from audiogen.core import FatalProviderError, PermanentItemError
from .anthropic_client import complete
from .grounding import judge_grounding
from .translation import judge_translation

DEFAULT_MODEL = "claude-haiku-5-5"
SOURCES_CSV = Path(__file__).resolve().parent.parent / "curation" / "places_grounding_full_articles_v1.csv"

FR_SQL = """
SELECT p.id, p.name, s.text FROM scripts s JOIN places p ON p.id = s.place_id
WHERE s.language = 'fr' AND (%(place)s::text IS NULL OR p.name ILIKE '%%' || %(place)s::text || '%%')
ORDER BY p.name LIMIT %(limit)s OFFSET %(offset)s
"""
PAIR_SQL = """
SELECT p.id, p.name, fr.text, t.language, t.text
FROM scripts fr JOIN scripts t ON t.place_id = fr.place_id AND t.language <> 'fr'
JOIN places p ON p.id = fr.place_id
WHERE fr.language = 'fr' AND (%(language)s::text IS NULL OR t.language = %(language)s::text)
  AND (%(place)s::text IS NULL OR p.name ILIKE '%%' || %(place)s::text || '%%')
ORDER BY p.name, t.language LIMIT %(limit)s OFFSET %(offset)s
"""


def load_sources(path: Path = SOURCES_CSV) -> dict[str, str]:
    csv.field_size_limit(10**9)
    with open(path, encoding="utf-8") as fh:
        return {r["id"]: r["full_grounding_text"] for r in csv.DictReader(fh) if r.get("full_grounding_text")}


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("mode", choices=["grounding", "translation"])
    ap.add_argument("--apply", action="store_true", help="call the model (default: dry run, no cost)")
    ap.add_argument("--limit", type=int, default=3)
    ap.add_argument("--offset", type=int, default=0)
    ap.add_argument("--place", help="substring of the place name")
    ap.add_argument("--language", choices=["en", "es", "pt"], help="translation mode: only this language")
    ap.add_argument("--model", default=DEFAULT_MODEL)
    ap.add_argument("--out", default=None)
    args = ap.parse_args(argv)

    import psycopg

    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    params = {"place": args.place, "language": args.language, "limit": args.limit, "offset": args.offset}
    api_key = os.environ.get("ANTHROPIC_API_KEY", "")
    if args.apply and not api_key:
        print("ANTHROPIC_API_KEY is not set", file=sys.stderr)
        return 2
    ask = lambda prompt: complete(prompt, api_key, args.model)  # noqa: E731
    report, skipped = [], 0

    if args.mode == "grounding":
        sources = load_sources()
        for place_id, name, script in conn.execute(FR_SQL, params).fetchall():
            source = sources.get(place_id)
            if not source:
                print(f"{name}: no source -> unverifiable (not clean)")
                report.append({"place": name, "unverifiable": True})
                continue
            if not args.apply:
                print(f"{name}: would judge {len(script)} chars against a {len(source)}-char source")
                continue
            try:
                claims, summary = judge_grounding(script, source, "fr", ask)
            except FatalProviderError as exc:
                print(f"STOP provider unusable ({exc})", file=sys.stderr)
                return 2
            except (PermanentItemError, ValueError) as exc:
                print(f"SKIP {name}: {exc}", file=sys.stderr)
                skipped += 1
                continue
            bad = [c for c in claims if c.status != "supported"]
            print(f"{name}: {'CLEAN' if summary['clean'] else 'ISSUES'}  {summary}")
            for c in bad:
                print(f"    {c.status}: {c.claim!r}  ({c.reason})")
            report.append({"place": name, "summary": summary, "problems": [c.__dict__ for c in bad]})
    else:
        for place_id, name, fr, lang, text in conn.execute(PAIR_SQL, params).fetchall():
            if not args.apply:
                print(f"{name} [{lang}]: would judge {len(text)} chars against {len(fr)} French chars")
                continue
            try:
                issues, pre = judge_translation(fr, text, lang, ask)
            except FatalProviderError as exc:
                print(f"STOP provider unusable ({exc})", file=sys.stderr)
                return 2
            except (PermanentItemError, ValueError) as exc:
                print(f"SKIP {name} [{lang}]: {exc}", file=sys.stderr)
                skipped += 1
                continue
            print(f"{name} [{lang}]: {len(issues)} issue(s)  precheck added={pre.numbers_added} dropped={pre.numbers_dropped} ratio={pre.length_ratio:.2f}")
            for i in issues:
                print(f"    {i.kind}{'' if i.quote_verified else ' (QUOTE NOT VERIFIED)'}: {i.translation_quote or i.fr_quote!r}  ({i.reason})")
            report.append({"place": name, "language": lang, "precheck": pre.__dict__, "issues": [i.__dict__ for i in issues]})

    if args.apply:
        out = args.out or f"{args.mode}_judge_report.json"
        with open(out, "w") as fh:
            json.dump(report, fh, ensure_ascii=False, indent=1)
        print(f"{len(report)} judged, {skipped} skipped -> {out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
