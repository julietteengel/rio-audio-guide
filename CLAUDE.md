# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository structure — read this first

As of 2026-08-19, this is a monorepo: the `sourcing-pipeline`, `frontend`, and `backend` branches (each
previously reviewed and merged independently, PRs #1-#3) are all merged into `master`, which is now the
single integration branch. **Going forward, work happens on a short-lived branch per change** (a
feature/fix, not a whole subsystem) **with a PR into `master`** — not direct pushes to `master`, and not
one long-lived branch per subsystem anymore. CI (`.github/workflows/`) triggers on `master`, path-
filtered per subsystem so an unrelated change doesn't run irrelevant checks.

- **`backend/`** — the Go backend (hexagonal architecture + DDD), `go.mod`/`cmd/`/`internal/` sitting
  directly under it (the Go module root). See `docs/superpowers/specs/2026-08-12-backend-domain-model-design.md`
  for the domain model and `docs/superpowers/plans/2026-08-12-backend-domain-model.md` for the
  implementation plan. Originally merged flat into the repo root (no subfolder); reorganized into
  `backend/` on 2026-08-19 to match `web/`/`mobile/`/`pipeline/`'s existing pattern — each subsystem
  now a clean top-level folder with its own README.
  **Authorship split (see `mission.md` for the full note)**: `internal/domain/` and `internal/ports/`
  are hand-written by the founder — Claude's role there stays explanation/review only, never editing
  those files directly. Everything from `internal/adapters/` onward (Postgres, RabbitMQ, HTTP, CI/CD)
  switched to AI-written/human-reviewed on 2026-08-15, a deliberate time-boxed call, not the default.
- **`pipeline/`** — the location-sourcing pipeline and its curation scripts (Python).
- **`web/`** — the Next.js landing page (started 2026-08-18, see
  `docs/superpowers/specs/2026-08-18-landing-page-design.md`). Purely static (no backend calls), hosted
  on Vercel — deliberately decoupled from the backend's AWS infrastructure.
- **`mobile/`** — the React Native (Expo) app targeting the App Store, iOS first (started 2026-08-18,
  see `2026-08-18-mobile-app-design.md` for screens/navigation/architecture; see
  `2026-08-19-mobile-real-backend-wiring-design.md` for the follow-up that wired it to the backend's
  real routes, added a real map with a web fallback, and fixed a download-language/UI-language UX
  bug; see `2026-08-19-mobile-account-design.md` for login/edit-profile/logout/delete-account, wired
  to auth routes the founder built in parallel on `backend` — read all three, none repeats the last).
  Both `web/` and `mobile/` reuse the brand, copy, and visual design already validated in an earlier
  Claude Design prototype (ephemeral session scratch files, not part of this repo) — that prototype is
  not re-designed here, and is no longer needed to continue this work: the specs above and the code
  itself are the durable source of truth going forward.

```
docs/superpowers/specs/   dated, point-in-time decision/research documents (design doc, roadmap
                           updates, stack decisions, product PRD) — append-only history, not living
                           docs. The single planning-doc convention this project uses — an earlier,
                           short-lived parallel attempt under .claude/PRPs/ (a different plugin's own
                           convention) was consolidated in here 2026-08-19, see
                           2026-08-06-product-prd.md and 2026-08-06-content-pipeline.plan.md.
docs/superpowers/plans/   per-feature TDD implementation plans (task-by-task, checkbox-tracked)
pipeline/sourcing/        the tested package: one module per data source (Overture, Wikidata,
                           feiras livres) + pure dedup logic + orchestrator
pipeline/curation/        one-off/ad-hoc scripts for content enrichment, narration generation,
                           and data cleanup — NOT a tested package, run manually, no test coverage
```

## Commands (run from `pipeline/`)

```bash
python3.12 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"

pytest -v                        # full suite
pytest tests/test_dedup.py -v    # single file
pytest -k test_name -v           # single test
pytest -m "not integration" -v   # skip tests requiring live network access (Overture S3 parquet, etc.)

python -m sourcing.pipeline      # run the full sourcing pipeline, writes pipeline/places.json
```

## Architecture — sourcing pipeline

Each data source (`overture.py`, `wikidata.py`, `feiras.py`) separates pure/testable logic (parsing,
filtering) from its network call, so the pure logic is unit-tested without a network round-trip and
only one function per module needs an `integration`-marked test. All three converge on a shared
`Place` dataclass (`sourcing/models.py`); `sourcing/pipeline.py` fetches from all sources, runs
`sourcing/dedup.py`, and writes the combined JSON.

Deduplication (`dedup.py`) prioritizes a shared Wikidata QID as the strongest merge signal; otherwise
it merges on normalized-name + proximity (≤100m), and deliberately does **not** auto-merge on weak/
ambiguous signals — false merges silently drop real places, which has bitten this project before (see
`docs/superpowers/specs/2026-07-21-rio-audio-guide-design.md`, dedup section).

## Content curation (`pipeline/curation/`)

Not a package — a sequence of scripts run manually against evolving `places_clean_vN.csv` snapshots
(each version is a checkpoint, not overwritten in place). The core rule enforced throughout: **a place
without real grounding (a Wikipedia/Wikidata extract or a credible web source) gets no narration** —
never invent facts to fill a gap. Overture's `landmark_and_historical_building` category is known to
be extremely noisy (mostly residential/condo branding, not landmarks) and should be filtered by
`landmark_classification.csv`-style triage before spending web-search budget on it, not searched
wholesale. See `docs/superpowers/specs/2026-07-23-roadmap-v2-agentic-architecture.md` for the full
grounding/narration/translation/anti-hallucination-judge methodology and its known failure modes
(Wikimedia rate limits, WebSearch quota exhaustion, coordinate mismatches placing a candidate outside
Rio's actual municipal boundary).
