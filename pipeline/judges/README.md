# judges — LLM judges for generated content

## transcript — Whisper transcript vs. script

The audio is spoken from the script, so the script is right and the Whisper transcript (the synced captions
in the app) is what can be wrong. Layer 1 aligns the two word by word and keeps only real differences (not
case, accents, punctuation or XV/15). Layer 2 asks a model, per difference, "benign" or "error", with the
script's wording as the correction. An "error" means: the caption shows a different name, date or fact than the
script. Names Whisper spells in the audio's locale ("Luis" for "Luiz") count, because captions should match the
script.

```bash
pip install -e ".[audio]"
export DATABASE_URL=... S3_BUCKET=... ANTHROPIC_API_KEY=...   # plus AWS credentials for S3
python -m judges.judge_transcripts --limit 3                           # dry run: counts only, no model call
python -m judges.judge_transcripts --apply --place Selar --language fr # asks the model, writes a JSON report
```

Without `--write-fixes` it only reports: nothing is written to S3 or the database.

`--write-fixes` (needs `--apply`) rewrites the caption text of a timestamps file, timing untouched. Each fix is
made at the exact words where the difference was found, never as a search-and-replace (which would also hit
legitimate uses of short words like "O" or "Alma"). The original is first copied to `<key>.orig`, and an
existing backup is never overwritten, so the very first original always survives; to undo, copy it back.
Only a misheard word inside one segment is fixed. A difference spanning two segments, or a missing or extra
word, is listed as `unapplied` for a human. The fixed file is validated before it replaces the original.

Identical differences (the same misheard name repeated) are judged once. The model is not deterministic: a
borderline difference can be ruled benign on one run and an error on the next, so treat the report as a
strong filter for a human to skim, not as ground truth.

**Status:** unit-tested without network; run live (report only) on three real pairs (Escadaria Selarón fr,
Circo Voador es, Theatro Municipal pt) with `claude-haiku-5-5`, where it found real misheard names. The
`--write-fixes` path is unit-tested against a fake S3 but has not yet written to the real bucket, and the whole
corpus has not been run.
