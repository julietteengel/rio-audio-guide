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

Report only: it never writes the database or S3. `transcript.apply_corrections` turns a report into fixed
Whisper `segments` (text only, timing untouched).

**Status:** unit-tested without network; run live on three real pairs (Escadaria Selarón fr, Circo Voador es,
Theatro Municipal pt) with `claude-haiku-5-5`, where it found real misheard names. Not yet run on the whole
corpus, and the correction step is not wired to S3.
