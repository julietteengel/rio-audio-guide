# Memória Carioca — Rio Audio Guide

A hands-free, multilingual (EN/FR/PT/ES) geolocation-triggered audio guide for Rio de Janeiro's
cultural and heritage sites. Real, sourced, verified content (grounded in Wikidata/Wikipedia/Overture,
checked by an anti-hallucination pass) — not AI-invented history.

See [`mission.md`](mission.md) for the full vision, scope, and current project status.

## Repository structure

A monorepo: backend, frontend, and the sourcing pipeline used to previously live on separate branches,
reviewed and merged independently — as of 2026-08-19 all three are merged into `master`, which is now
the single integration branch (see `CLAUDE.md` for the review/branching convention this project follows
going forward — short-lived branches per change, not per subsystem).

- **`backend/`** — the Go backend. Hexagonal architecture (ports & adapters) + Domain-Driven Design,
  PostgreSQL/PostGIS, RabbitMQ (TTS job queue), S3, real ElevenLabs voice generation. Has its own
  [README](backend/README.md) with the full architecture trace and setup/run instructions.
- **`web/`** — the Next.js landing page. Makes no calls to the Go backend, deployed independently of
  its AWS infrastructure — not a static export though (it uses Next.js Middleware for locale routing,
  which needs a real Node.js runtime, `output: "standalone"`). Has its own [README](web/README.md).
- **`mobile/`** — the React Native (Expo) app, iOS first. Talks to the real backend
  (`EXPO_PUBLIC_API_BASE_URL` in a local `.env`, same rule — never commit real values).
- **`pipeline/`** — the Python sourcing pipeline (Overture Maps, Wikidata, feiras livres registry,
  deduplication — `pipeline/sourcing/`, a tested package) and the content curation scripts (grounding,
  narration, translation, anti-hallucination checks — `pipeline/curation/`, ad-hoc, not a tested
  package). Produces the CSVs `backend/cmd/import` consumes; has no runtime relationship to
  backend/frontend beyond that one-time import.

## Why Go for the backend, why Python for the pipeline

Not an arbitrary split — different jobs, different real fit, not just "use what's trendy" or "use what's
familiar." (One honest caveat on Go specifically: the project's governing principle, per `mission.md`,
is that *a technology only enters scope for a genuine product need — portfolio value is a tie-breaker,
never the primary justification.* Go passes that test on its own merits below; that it also happens to
demonstrate skills relevant to a job search is real, but secondary, not the reason.)

**Go, hexagonal architecture + DDD, for the backend**: the backend's actual domain — a content
*publication workflow* — has real invariants worth enforcing in code, not just convention: a place isn't
published without human review; a language variant isn't published without its audio actually generated;
deduplication stays deliberately conservative because a false merge silently drops a real place (a
mistake this project already made once, the hard way, during sourcing). That's a genuine case for
domain-driven design, not decoration. Go's static typing catches a whole class of "forgot to handle this
state transition" bugs at compile time rather than in production, and its concurrency primitives
(goroutines, channels) are a real, direct fit for a service that has to run an HTTP API and a RabbitMQ
worker consuming TTS jobs concurrently. See
[`docs/superpowers/specs/2026-08-04-backend-stack-decision.md`](docs/superpowers/specs/2026-08-04-backend-stack-decision.md)
for the full reasoning, including what got explicitly cut (Kafka, MongoDB, Redis — each proposed once,
each rejected for the same reason: no measured product need, only "it's on a skills list" to justify it,
which the same doc names as bad engineering reasoning regardless of context).

**Python for the sourcing/curation pipeline**: this was never formally litigated the way the backend
stack was — it's the practical, defensible default for what the job actually is. The pipeline is
one-off/occasional batch data work (SPARQL queries against Wikidata, web scraping, CSV/JSON wrangling,
geocoding), run manually and supervised by a human at each real checkpoint, never a long-running networked
service. Python's ecosystem (`requests`, geocoding libraries, quick iteration for scripts that get
written once and adjusted often) fits that shape well; Go's actual strengths — static typing for
long-lived invariants, concurrency for a service handling simultaneous requests — wouldn't buy anything
here, since there's no service to keep correct over time and nothing here runs concurrently by design
(see `CLAUDE.md`: `pipeline/sourcing/` is the one tested package in here, `pipeline/curation/` is
deliberately ad-hoc, run-by-hand, not a tested package).

## Where to find more

- [`mission.md`](mission.md) — the living, always-current source of truth for scope and status. Edited
  in place as the project moves, unlike the dated documents below.
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — dated decision/research documents (design
  doc, roadmap updates, stack decisions, product PRD), append-only history, never edited after the
  fact.
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — per-feature TDD implementation plans.
- [`CLAUDE.md`](CLAUDE.md) — engineering conventions and repo structure, for anyone (human or AI)
  working in this codebase.
