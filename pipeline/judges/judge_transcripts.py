"""Judge Whisper transcripts of ready narration audio against their scripts. Report only.

    python -m judges.judge_transcripts --limit 3                 # dry run: count differences, no model call
    python -m judges.judge_transcripts --apply --limit 3         # ask the model, write a report
    python -m judges.judge_transcripts --apply --place "Selar" --language fr

Never writes to the database or S3: it lists the transcript errors it found and
the correction for each (apply_corrections can then fix the timestamps files).
Env: DATABASE_URL, S3_BUCKET, ANTHROPIC_API_KEY (+ AWS credentials for boto3).
"""
import argparse
import json
import os
import sys

from audiogen.core import FatalProviderError, PermanentItemError, parse_s3_key
from .anthropic_client import complete
from .transcript import find_mismatches, judge

DEFAULT_MODEL = "claude-haiku-5-5"

PAIRS_SQL = """
SELECT a.id, p.name, s.language, s.text, a.timestamps_url
FROM audio_files a
JOIN scripts s ON s.id = a.script_id
JOIN places p ON p.id = s.place_id
WHERE a.status = 'ready' AND a.timestamps_url IS NOT NULL AND a.timestamps_url <> ''
  AND (%(place)s::text IS NULL OR p.name ILIKE '%%' || %(place)s::text || '%%')
  AND (%(language)s::text IS NULL OR s.language = %(language)s::text)
ORDER BY p.name, s.language
LIMIT %(limit)s OFFSET %(offset)s
"""


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("--apply", action="store_true", help="call the model (default: dry run, no cost)")
    ap.add_argument("--limit", type=int, default=3)
    ap.add_argument("--offset", type=int, default=0)
    ap.add_argument("--place", help="substring of the place name")
    ap.add_argument("--language", choices=["fr", "en", "pt", "es"])
    ap.add_argument("--model", default=DEFAULT_MODEL)
    ap.add_argument("--out", default="transcript_judge_report.json")
    args = ap.parse_args(argv)

    import boto3
    import psycopg

    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    rows = conn.execute(
        PAIRS_SQL, {"place": args.place, "language": args.language, "limit": args.limit, "offset": args.offset}
    ).fetchall()
    s3 = boto3.client("s3")
    bucket = os.environ["S3_BUCKET"]
    api_key = os.environ.get("ANTHROPIC_API_KEY", "")
    if args.apply and not api_key:
        print("ANTHROPIC_API_KEY is not set", file=sys.stderr)
        return 2

    report, total_diffs, total_errors = [], 0, 0
    for audio_id, name, language, script, ts_url in rows:
        transcript = json.loads(s3.get_object(Bucket=bucket, Key=parse_s3_key(ts_url))["Body"].read())["text"]
        if not args.apply:
            n = len(find_mismatches(script, transcript))
            total_diffs += n
            print(f"{name} [{language}]: {n} difference(s)")
            continue
        try:
            results = judge(script, transcript, language, lambda prompt: complete(prompt, api_key, args.model))
        except FatalProviderError as exc:
            print(f"STOP provider unusable ({exc})", file=sys.stderr)
            return 2
        except (PermanentItemError, ValueError) as exc:
            print(f"SKIP {name} [{language}]: {exc}", file=sys.stderr)
            continue
        errors = [(m, v) for m, v in results if v.verdict == "error"]
        total_diffs += len(results)
        total_errors += len(errors)
        print(f"{name} [{language}]: {len(results)} difference(s), {len(errors)} error(s)")
        for m, v in errors:
            print(f"    heard {m.transcript_span!r} -> should be {v.correction!r}  ({v.reason})")
        report.append(
            {
                "audio_id": audio_id,
                "place": name,
                "language": language,
                "errors": [
                    {"heard": m.transcript_span, "script": m.script_span, "correction": v.correction, "reason": v.reason}
                    for m, v in errors
                ],
                "benign": len(results) - len(errors),
            }
        )
    if args.apply:
        with open(args.out, "w") as fh:
            json.dump(report, fh, ensure_ascii=False, indent=1)
        print(f"{len(rows)} pair(s), {total_diffs} difference(s), {total_errors} error(s) -> {args.out}")
    else:
        print(f"{len(rows)} pair(s), {total_diffs} difference(s). dry run -- pass --apply to ask the model")
    return 0


if __name__ == "__main__":
    sys.exit(main())
