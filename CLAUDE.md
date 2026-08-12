# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository structure — read this first

`master` currently contains **only documentation** (`docs/`) — no application code. Each implemented
subsystem lives on its own branch, checked out as a git worktree, and is not merged into `master` until
it's had a real human code review pass (not just an automated one). When asked to work on one of these,
`cd` into its worktree (or work directly in that branch) rather than looking for it on `master`:

- **`sourcing-pipeline` branch**, `.worktrees/sourcing-pipeline/` — the location-sourcing pipeline and
  its curation scripts (Python).
- **`backend` branch**, `.worktrees/backend/` — the Go backend (hexagonal architecture + DDD). Started
  2026-08-12, hand-written (not AI-generated) per `mission.md` — see
  `docs/superpowers/specs/2026-08-12-backend-domain-model-design.md` for the domain model and
  `docs/superpowers/plans/2026-08-12-backend-domain-model.md` for the implementation plan. Claude's role
  on this branch is limited to explaining concepts and reviewing code already written — not writing
  `.go` files (the one-time empty package skeleton is the sole exception, already committed).

```
docs/superpowers/specs/   dated, point-in-time decision/research documents (design doc, roadmap
                           updates, stack decisions) — append-only history, not living docs
docs/superpowers/plans/   per-feature TDD implementation plans (task-by-task, checkbox-tracked)
pipeline/sourcing/        the tested package: one module per data source (Overture, Wikidata,
                           feiras livres) + pure dedup logic + orchestrator — sourcing-pipeline
                           branch only
pipeline/curation/        one-off/ad-hoc scripts for content enrichment, narration generation,
                           and data cleanup — NOT a tested package, run manually, no test coverage
backend/                  Go module (hexagonal: internal/domain, internal/application, internal/ports,
                           internal/adapters/{postgres,rabbitmq}, cmd/api) — backend branch only
```

## Commands (run from `pipeline/` on the `sourcing-pipeline` branch/worktree)

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
