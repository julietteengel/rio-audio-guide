# audiogen — narration audio generation

Turns reviewed scripts into published narration audio, and produces the per-sentence timestamps the
mobile app uses for synced captions. Run from `pipeline/`:

```bash
pip install -e ".[audio]"
export DATABASE_URL=... S3_BUCKET=... OPENAI_API_KEY=...   # plus AWS credentials for S3

python -m audiogen.generate                  # dry run: how many scripts need audio
python -m audiogen.generate --apply --limit 10 --voice onyx
python -m audiogen.transcribe --apply --limit 10   # Whisper timestamps for ready audio
```

## What it guarantees
- **Only human-reviewed scripts are voiced** (`status` `reviewed` or `published`). Drafts are never touched.
- **`audio_files` → `ready` and `scripts` → `published` in one DB transaction.** The Go code does
  these as two separate saves and `getPlaceAudio` refuses to serve a ready audio whose script is not
  published, so a crash between the two used to be able to hide an audio. Here that cannot happen.
- **No unbounded retries.** Transient errors (network, 5xx, 429) retry 3 times; a bad key or exhausted
  credit (`insufficient_quota`) stops the whole run with exit code 2 instead of failing script by script.
- Dry run by default, like `backend/cmd/batchreview`.
- **A script the provider rejects for good is recorded as a `failed` audio file** (same as the Go worker,
  so the API reports it and `POST /audio-files/:id/retry` can requeue it) and skipped on later runs
  unless `--retry-failed` is passed. Without that it would sit at the head of the queue forever.
  `transcribe.py` has no such column, so it prints the failed ids and takes `--offset` to step past them.
- **A lost database stops the run** (exit 2) rather than paying for more TTS that cannot be saved.
- **Whisper output is validated** (JSON, non-empty `segments`, positive duration) before it is stored,
  because `timestamps_url` being set is what marks an audio as done. `transcribe.py` also writes
  Whisper's real duration, replacing the word-count estimate the worker uses when Whisper failed.
- S3 keys and `s3://bucket/key` storage URLs match what `parseS3Key` in the backend expects; the
  timestamps file is Whisper's raw `verbose_json`, which `mobile/src/utils/syncedText.ts` parses.

## Status: reconstructed, not re-validated
The original scripts that produced the audio in the dev/prod database were never committed and are
lost. These were rewritten in October 2026 from the contract documented in `backend/README.md`.
- The pure logic and the error handling are unit-tested (`tests/test_audiogen.py`, no network).
- **Never run against the live OpenAI API, S3 or Postgres** — there was no OpenAI credit to do so.
  Do a `--limit 1` run before trusting it.
- The original also used **Google Cloud TTS** for some audio; only OpenAI TTS is reconstructed here.
- The voice (`onyx`) and model (`tts-1`) defaults are choices made in the rewrite, not recovered values.
- Whisper is given the start of the script as a `prompt` to bias proper nouns (it once heard
  "Praça XV" as "Traça 15"). That mitigation is untested.
- The backend worker's `openaitts` adapter (`backend/internal/adapters/openaitts`) does the same thing for
  jobs coming through the queue, and shares this README's caveat: tested against a fake server only.
- The place-image step (`places.image_url`, Wikidata P18) is not reconstructed.
