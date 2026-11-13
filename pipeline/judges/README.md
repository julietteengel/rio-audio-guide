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

## grounding — narration vs. its real source

```bash
export DATABASE_URL=... ANTHROPIC_API_KEY=...
python -m judges.judge_content grounding --limit 3                     # dry run
python -m judges.judge_content grounding --apply --place "Jardim Bot"  # ask the model, write a JSON report
```

The model splits the narration into claims (block by block, each against the full source) and marks each
supported, unsupported or contradicted. **A "supported" claim only counts if its citation is a verbatim passage
of the source** (case, accents and whitespace aside): a quote the model made up downgrades the claim to
unsupported. That also means a correct but loosely quoted claim can be downgraded, so read "citation not found"
as "check this", not as proof of a fabrication. A place is `clean` only if no claim is unsupported or
contradicted *and* at least 60% of the narration's sentences got a claim, so a judge that skips content cannot
pass a place.

Sources come from `pipeline/curation/places_grounding_full_articles_v1.csv` (the database keeps none:
`scripts.source_text` is empty). 161 of 252 places have one. **A place with no source is reported as
unverifiable, never as clean.**

## translation — EN/ES/PT vs. the French

```bash
python -m judges.judge_content translation --apply --language en --place Selar
```

The French is the ground truth, so the other languages are never re-grounded against the source, only checked
for facts added, dropped or altered. A mechanical precheck compares the numbers (8 000 = 8,000 = 8000) and the
length of the two texts; the model then lists the issues, and each quote it gives is checked against the text it
claims to come from (`quote_verified`).

**Status of both:** unit-tested without network; run live on a few real places with `claude-haiku-5-5`
(Jardim Botânico grounding: 158 supported, 9 unsupported, 1 contradicted, including "Napoleon" and "Ministry of the
Empire" that the source does not support; Escadaria Selarón translations en/es). Report only, not run on the
whole corpus, and the model is not deterministic.
