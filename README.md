# Memória Carioca — Rio Audio Guide

A hands-free, multilingual (EN/FR/PT/ES) geolocation-triggered audio guide for Rio de Janeiro's
cultural and heritage sites. Real, sourced, verified content (grounded in Wikidata/Wikipedia/Overture,
checked by an anti-hallucination pass) — not AI-invented history.

See [`mission.md`](mission.md) for the full vision, scope, and current project status.

## Repository structure

- **`pipeline/`** — the Python sourcing pipeline (Overture Maps, Wikidata, feiras livres registry,
  deduplication) and the content curation scripts (grounding, narration, translation,
  anti-hallucination checks). Merged from the `sourcing-pipeline` branch.
- **`web/`** — the Next.js landing page.
- **`mobile/`** — the React Native/Expo mobile app.

The Go backend is the one piece still developed on its own branch, not merged here yet:
[`backend`](https://github.com/julietteengel/rio-audio-guide/tree/backend) — hexagonal architecture +
DDD, PostgreSQL/PostGIS, RabbitMQ (TTS job queue), S3, real ElevenLabs voice generation. Has its own
[README](https://github.com/julietteengel/rio-audio-guide/blob/backend/README.md) with setup/run
instructions. It's checked out as a git worktree at `.worktrees/backend/` alongside this checkout —
see `CLAUDE.md` for why, and for the convention this project follows before merging a subsystem here
(a real human code review, not just an automated one).

## Where to find more

- [`mission.md`](mission.md) — the living, always-current source of truth for scope and status. Edited
  in place as the project moves, unlike the dated documents below.
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — dated decision/research documents (design
  doc, roadmap updates, stack decisions), append-only history, never edited after the fact.
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — per-feature TDD implementation plans.
- [`CLAUDE.md`](CLAUDE.md) — engineering conventions and repo structure, for anyone (human or AI)
  working in this codebase.
