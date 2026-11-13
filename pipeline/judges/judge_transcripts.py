"""Judge Whisper transcripts of ready narration audio against their scripts.

    python -m judges.judge_transcripts --limit 3                 # dry run: count differences, no model call
    python -m judges.judge_transcripts --apply --limit 3         # ask the model, write a JSON report
    python -m judges.judge_transcripts --apply --write-fixes --place Selar --language fr

Without --write-fixes it never writes anything to S3 or the database. With it, each timestamps
file that has fixable errors is first copied to `<key>.orig` (kept if one already exists, so the
very first original is never lost), then rewritten with the corrected caption text; timing is
untouched. Only misheard words inside one segment are fixed; the rest is listed as unapplied.
Env: DATABASE_URL, S3_BUCKET, ANTHROPIC_API_KEY (+ AWS credentials for boto3).
"""
import argparse
import json
import os
import sys

from audiogen.core import FatalProviderError, PermanentItemError, parse_s3_key, validate_whisper
from .anthropic_client import complete
from .transcript import apply_fixes, find_mismatches_in_body, judge

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


def _write_fixed(s3, bucket: str, key: str, fixed: dict) -> None:
    """Back the original up once, then replace it with the fixed file (validated first)."""
    body = json.dumps(fixed, ensure_ascii=False).encode()
    validate_whisper(body)  # never store something the app could not parse
    backup = f"{key}.orig"
    try:
        s3.head_object(Bucket=bucket, Key=backup)
    except s3.exceptions.ClientError:
        s3.copy_object(Bucket=bucket, Key=backup, CopySource={"Bucket": bucket, "Key": key})
    s3.put_object(Bucket=bucket, Key=key, Body=body, ContentType="application/json")


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("--apply", action="store_true", help="call the model (default: dry run, no cost)")
    ap.add_argument("--write-fixes", action="store_true", help="with --apply: write corrected timestamps to S3")
    ap.add_argument("--limit", type=int, default=3)
    ap.add_argument("--offset", type=int, default=0)
    ap.add_argument("--place", help="substring of the place name")
    ap.add_argument("--language", choices=["fr", "en", "pt", "es"])
    ap.add_argument("--model", default=DEFAULT_MODEL)
    ap.add_argument("--out", default="transcript_judge_report.json")
    args = ap.parse_args(argv)
    if args.write_fixes and not args.apply:
        ap.error("--write-fixes needs --apply")

    import boto3
    import psycopg
    from botocore.exceptions import BotoCoreError, ClientError

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

    report, total_diffs, total_errors, total_fixed, skipped = [], 0, 0, 0, 0
    for audio_id, name, language, script, ts_url in rows:
        key = parse_s3_key(ts_url)
        try:
            body = json.loads(s3.get_object(Bucket=bucket, Key=key)["Body"].read())
        except (BotoCoreError, ClientError, ValueError) as exc:
            print(f"SKIP {name} [{language}]: cannot read {ts_url}: {exc}", file=sys.stderr)
            skipped += 1
            continue
        if not args.apply:
            n = len(find_mismatches_in_body(script, body))
            total_diffs += n
            print(f"{name} [{language}]: {n} difference(s)")
            continue
        try:
            results = judge(script, body, language, lambda prompt: complete(prompt, api_key, args.model))
        except FatalProviderError as exc:
            print(f"STOP provider unusable ({exc})", file=sys.stderr)
            return 2
        except (PermanentItemError, ValueError) as exc:
            print(f"SKIP {name} [{language}]: {exc}", file=sys.stderr)
            skipped += 1
            continue
        errors = [(m, v) for m, v in results if v.verdict == "error"]
        fixed, applied, unapplied = apply_fixes(body, results)
        total_diffs += len(results)
        total_errors += len(errors)
        total_fixed += applied
        print(f"{name} [{language}]: {len(results)} difference(s), {len(errors)} error(s), {applied} fixable")
        for m, v in errors:
            print(f"    heard {m.transcript_span!r} -> should be {v.correction!r}  ({v.reason})")
        wrote = False
        if args.write_fixes and applied:
            try:
                _write_fixed(s3, bucket, key, fixed)
                wrote = True
                print(f"    wrote {applied} fix(es) to {ts_url} (original kept as {ts_url}.orig)")
            except (BotoCoreError, ClientError, ValueError) as exc:
                print(f"    NOT written: {exc}", file=sys.stderr)
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
                "fixable": applied,
                "unapplied": [{"heard": m.transcript_span, "kind": m.kind} for m, _ in unapplied],
                "written": wrote,
            }
        )
    if args.apply:
        with open(args.out, "w") as fh:
            json.dump(report, fh, ensure_ascii=False, indent=1)
        print(
            f"{len(rows)} pair(s), {total_diffs} difference(s), {total_errors} error(s), "
            f"{total_fixed} fixable, {skipped} skipped -> {args.out}"
        )
    else:
        print(f"{len(rows)} pair(s), {total_diffs} difference(s), {skipped} skipped. dry run -- pass --apply to ask the model")
    return 0


if __name__ == "__main__":
    sys.exit(main())
