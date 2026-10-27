"""Generate narration audio for reviewed scripts and publish them.

Reconstruction of the standalone script that produced the audio in the dev/prod
database (the original was never committed -- see backend/README.md). Not
re-validated against live APIs.

Reproduces CompleteAudioGeneration's contract (backend/internal/application/
publish_script.go): the audio_files row becomes 'ready' AND the script becomes
'published'. Both happen in ONE transaction here, unlike the Go code's two
separate saves, so a crash can never leave a ready audio on an unpublished
script (which getPlaceAudio refuses to serve).

Only scripts a human already reviewed are processed -- drafts are never voiced.

    python -m audiogen.generate                # dry run: counts what it would do
    python -m audiogen.generate --apply --limit 10
Env: DATABASE_URL, S3_BUCKET, OPENAI_API_KEY (+ AWS credentials for boto3).
"""
import argparse
import io
import os
import sys
import uuid

from .core import (
    FatalProviderError,
    PermanentItemError,
    audio_key,
    split_for_tts,
    storage_url,
    voice_label,
)
from .openai_client import synthesize

PENDING_SQL = """
SELECT s.id, s.place_id, s.language, s.text
FROM scripts s
WHERE s.status IN ('reviewed', 'published')
  AND (%(languages)s::text[] IS NULL OR s.language = ANY(%(languages)s))
  AND NOT EXISTS (
      SELECT 1 FROM audio_files a WHERE a.script_id = s.id AND a.status = 'ready'
  )
  -- a script rejected for good is skipped, or it would sit at the head of the
  -- queue and starve everything behind it on every run
  AND (%(retry_failed)s OR NOT EXISTS (
      SELECT 1 FROM audio_files f WHERE f.script_id = s.id AND f.status = 'failed'
  ))
ORDER BY s.place_id, s.language
LIMIT %(limit)s
"""


def duration_ms(mp3: bytes) -> int:
    from mutagen.mp3 import MP3

    return int(MP3(io.BytesIO(mp3)).info.length * 1000)


def record_failure(conn, script_id: str, label: str, reason: str) -> None:
    """Persist a permanent rejection as a failed audio_files row (what the Go worker does too,
    so GET /places/:id/audio reports it and POST /audio-files/:id/retry can requeue it)."""
    row = conn.execute(
        "SELECT id FROM audio_files WHERE script_id = %s ORDER BY created_at DESC LIMIT 1",
        (script_id,),
    ).fetchone()
    reason = reason[:500]
    if row:
        conn.execute(
            "UPDATE audio_files SET status='failed', failure_reason=%s, updated_at=now() WHERE id=%s",
            (reason, row[0]),
        )
    else:
        conn.execute(
            """INSERT INTO audio_files (id, script_id, voice_id, status, failure_reason)
               VALUES (%s, %s, %s, 'failed', %s)""",
            (str(uuid.uuid4()), script_id, label, reason),
        )


def save_ready(conn, script_id: str, label: str, url: str, dur_ms: int) -> None:
    """audio_files -> ready and script -> published, atomically."""
    with conn.transaction():
        row = conn.execute(
            "SELECT id FROM audio_files WHERE script_id = %s ORDER BY created_at DESC LIMIT 1",
            (script_id,),
        ).fetchone()
        if row:
            conn.execute(
                """UPDATE audio_files SET voice_id=%s, status='ready', storage_url=%s,
                   duration_ms=%s, failure_reason=NULL, updated_at=now() WHERE id=%s""",
                (label, url, dur_ms, row[0]),
            )
        else:
            conn.execute(
                """INSERT INTO audio_files (id, script_id, voice_id, status, storage_url, duration_ms)
                   VALUES (%s, %s, %s, 'ready', %s, %s)""",
                (str(uuid.uuid4()), script_id, label, url, dur_ms),
            )
        published = conn.execute(
            """UPDATE scripts SET status='published', published_at=COALESCE(published_at, now()),
               updated_at=now() WHERE id=%s""",
            (script_id,),
        )
        if published.rowcount != 1:
            # raising inside the transaction rolls the audio_files write back too
            raise RuntimeError(f"script {script_id} not found, nothing published")


def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    p.add_argument("--apply", action="store_true", help="actually call OpenAI/S3 and write the DB (default: dry run)")
    p.add_argument("--limit", type=int, default=20)
    p.add_argument("--languages", nargs="*", default=None, help="e.g. fr en (default: all)")
    p.add_argument("--voice", default="onyx", help="OpenAI voice (a choice made here, not recovered from the original)")
    p.add_argument("--model", default="tts-1")
    p.add_argument("--retry-failed", action="store_true", help="also retry scripts previously rejected for good")
    args = p.parse_args(argv)

    import psycopg

    conn = psycopg.connect(os.environ["DATABASE_URL"], autocommit=True)
    pending = conn.execute(PENDING_SQL, {"languages": args.languages, "limit": args.limit, "retry_failed": args.retry_failed}).fetchall()
    total_chars = sum(len(r[3]) for r in pending)
    print(f"{len(pending)} script(s) need audio, {total_chars} characters")
    if not args.apply:
        print("dry run -- pass --apply to generate")
        return 0

    import boto3

    bucket = os.environ["S3_BUCKET"]
    api_key = os.environ["OPENAI_API_KEY"]
    s3 = boto3.client("s3")
    label = voice_label("openai", args.voice)

    done = 0
    failed: list[str] = []
    for script_id, place_id, language, text in pending:
        try:
            mp3 = b"".join(synthesize(c, api_key, args.model, args.voice) for c in split_for_tts(text))
            key = audio_key(label, script_id)
            s3.put_object(Bucket=bucket, Key=key, Body=mp3, ContentType="audio/mpeg")
            save_ready(conn, script_id, label, storage_url(bucket, key), duration_ms(mp3))
            done += 1
            print(f"ok   {place_id} [{language}]")
        except FatalProviderError as exc:
            print(f"STOP provider unusable ({exc}); {done} done, rerun after fixing", file=sys.stderr)
            return 2
        except (psycopg.OperationalError, psycopg.InterfaceError) as exc:
            # Postgres is gone: carrying on would pay for TTS on every remaining script,
            # fail to save each one, and pay again on the next run.
            print(f"STOP database unavailable ({exc}); {done} done", file=sys.stderr)
            return 2
        except PermanentItemError as exc:
            record_failure(conn, script_id, label, str(exc))
            failed.append(script_id)
            print(f"REJECTED {place_id} [{language}] (recorded as failed): {exc}", file=sys.stderr)
        except Exception as exc:  # transient: one bad script must not sink the batch
            failed.append(script_id)
            print(f"FAIL {place_id} [{language}]: {exc}", file=sys.stderr)
    print(f"done={done} failed={len(failed)}" + (f" ids={','.join(failed)}" if failed else ""))
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
