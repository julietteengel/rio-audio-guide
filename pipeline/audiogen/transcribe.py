"""Produce per-sentence timestamps for ready audio with OpenAI Whisper.

Reconstruction of a standalone step that was never committed. Not re-validated
against the live API. Neither OpenAI TTS nor Google TTS return timing, so the
finished mp3 is re-transcribed (verbose_json, segment granularity) and the raw
response is stored as audio_files.timestamps_url -- the exact `segments` shape
mobile/src/utils/syncedText.ts parses.

    python -m audiogen.transcribe              # dry run
    python -m audiogen.transcribe --apply --limit 10
Env: DATABASE_URL, S3_BUCKET, OPENAI_API_KEY (+ AWS credentials for boto3).
"""
import argparse
import os
import sys

from .core import (
    FatalProviderError,
    storage_url,
    timestamps_key,
    whisper_language,
    whisper_prompt,
)
from .openai_client import transcribe_segments

PENDING_SQL = """
SELECT a.id, a.voice_id, a.storage_url, s.language, s.text
FROM audio_files a JOIN scripts s ON s.id = a.script_id
WHERE a.status = 'ready' AND (a.timestamps_url IS NULL OR a.timestamps_url = '')
ORDER BY a.created_at
LIMIT %s
"""


def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    p.add_argument("--apply", action="store_true", help="actually call Whisper and write the DB (default: dry run)")
    p.add_argument("--limit", type=int, default=20)
    args = p.parse_args(argv)

    import psycopg

    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    pending = conn.execute(PENDING_SQL, (args.limit,)).fetchall()
    print(f"{len(pending)} audio file(s) lack timestamps")
    if not args.apply:
        print("dry run -- pass --apply to transcribe")
        return 0

    import boto3

    bucket = os.environ["S3_BUCKET"]
    api_key = os.environ["OPENAI_API_KEY"]
    s3 = boto3.client("s3")

    done = failed = 0
    for audio_id, label, audio_url, language, text in pending:
        try:
            audio_s3_key = audio_url.removeprefix("s3://").split("/", 1)[1]
            audio = s3.get_object(Bucket=bucket, Key=audio_s3_key)["Body"].read()
            body = transcribe_segments(audio, api_key, whisper_language(language), whisper_prompt(text))
            key = timestamps_key(label, audio_id)
            s3.put_object(Bucket=bucket, Key=key, Body=body, ContentType="application/json")
            conn.execute(
                "UPDATE audio_files SET timestamps_url=%s, updated_at=now() WHERE id=%s",
                (storage_url(bucket, key), audio_id),
            )
            done += 1
            print(f"ok   {audio_id}")
        except FatalProviderError as exc:
            print(f"STOP provider unusable ({exc}); {done} done, rerun after fixing", file=sys.stderr)
            return 2
        except Exception as exc:
            failed += 1
            print(f"FAIL {audio_id}: {exc}", file=sys.stderr)
    print(f"done={done} failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
