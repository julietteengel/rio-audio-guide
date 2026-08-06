# Rio Audio Guide — Mission

This is the living, always-current source of truth for what this product is, its scope, and where it
stands. Unlike `docs/superpowers/specs/*.md` (dated, append-only decision/research documents — one
per decision moment, never edited after the fact) and `docs/superpowers/plans/*.md` (per-feature
implementation plans), **this file gets edited in place** as the project moves. When a new decision
changes scope or sequencing, update this file directly; only write a new dated spec when the decision
itself needs a documented rationale worth preserving verbatim.

## Vision

A hands-free, multilingual (EN/FR/PT/ES) geolocation-triggered audio guide for Rio de Janeiro's
cultural and heritage sites, sold white-label to hotels/agencies (B2B), while also serving as a
technical portfolio piece (fullstack + cloud + AI).

**Double objective, explicit and not fully overlapping**: a real, testable/sellable product in Rio,
*and* a skills demonstration for job applications/freelance work. Where the two pull in different
directions (see `docs/superpowers/specs/2026-08-04-backend-stack-decision.md` for a concrete case —
Kafka/MongoDB/Redis proposed purely to match a job posting's keyword list, then cut), the governing
rule is: **a technology only enters scope for a genuine product need; portfolio value is a tie-breaker,
never the primary justification.** The exception is the guide runtime and backend, which are
deliberately hand-written (not AI-generated) specifically for their learning/portfolio value — see
Phases below.

## What this is not

- Not a live RAG chat / research copilot — explicitly considered and rejected as feature creep for a
  tourist audio guide. AI here is a content-production tool, not the product itself. (Reconsiderable
  in Phase 2 if a real user need emerges — see Guide runtime.)
- Not a copy of any specific existing project — "AI geolocated audio guide" is an established product
  category (VoiceMap, izi.TRAVEL, GPSmyCity, Summer AI...). Differentiation is real multilingual
  content (not just translated UI), audio as the primary experience, locally-verified content, and
  B2B hotel distribution — not the concept itself.

## Scope (current)

- **Geography**: all of Rio de Janeiro municipality (expanded from the original Santa Teresa/Lapa-only
  MVP scope — see roadmap v2 spec).
- **Languages**: English, French, Portuguese, Spanish.
- **Voice**: single adult voice for v1 (cloned, consent-based). Teen/child voices explicitly deferred
  to Phase 2 after cost analysis.
- **Explicitly out of scope for v1**: bars/restaurants, live chat/Q&A, teen/child voices, affiliate
  monetization, self-hosted TTS infrastructure, city-wide coverage beyond what the sourcing pipeline
  actually grounds (partial coverage is acceptable at launch — see Content pipeline status).

## Architecture at a glance

| Sub-system | Stack | Status |
|---|---|---|
| Sourcing pipeline | Python (Overture Maps, Wikidata, feiras registry) | Built, tested, **not yet merged to master** (pending human code review) |
| Content pipeline (batch) | Python (grounding, narration, translation, anti-hallucination judge) | In progress — see status below |
| Guide runtime (the agentic core) | TBD at build time — memory, tool calling, human-in-the-loop | Not started. **Highest learning priority — written by hand, not AI-generated**, per `docs/superpowers/specs/2026-07-23-roadmap-v2-agentic-architecture.md` |
| Backend | Go, hexagonal architecture + DDD, PostgreSQL/PostGIS, RabbitMQ (TTS job queue), K8s (EKS demo)/Scaleway (real prod) | Not started. **Written by hand**, per `docs/superpowers/specs/2026-08-04-backend-stack-decision.md` — the two skills the target job (Powens) actually evaluates |
| Ops depth | CI/CD canary+rollback, distributed observability, security guardrails, compliance | Not started — explicitly scoped in depth, not minimal, per roadmap v2 |
| Admin dashboard + mobile app | React Native app, admin dashboard | Not started |

Redis and Kafka were both proposed and then explicitly cut (2026-08-04) for lack of a measured,
current need — see the backend-stack-decision spec before re-adding either.

## Phases / sequencing

1. **Sourcing** — done, unmerged (`sourcing-pipeline` branch). Needs a real human review pass before
   merge; an automated subagent review approved it, which is not the same thing.
2. **Content pipeline** — in progress. See status below.
3. **Guide runtime** — next major phase after content pipeline reaches an acceptable coverage level.
4. **Backend** — in parallel with or after Guide runtime.
5. **Ops depth** — built as its own deliberate phase, not bolted on at the end.
6. **Admin dashboard + mobile app** — last.

## Current status (as of 2026-08-06 — keep this section updated, don't let it drift)

- **Sourcing**: 2230 candidate places in Rio, deduplicated. Branch complete but unmerged.
- **Content pipeline**: 441/2230 places narrated in FR, 422 in all 4 languages. Anti-hallucination
  judge has only checked 131 of those. The `landmark_and_historical_building` category (1374 of the
  1789 still-unnarrated places) was found to be ~78% noise (real-estate/business/street-address
  entries, not landmarks) and triaged down to 307 real candidates
  (`pipeline/curation/landmark_classification.csv`). A 60-place grounding-search canary on that
  triaged set is done: **30/60 found (50%)**. New finding from the canary, not previously known: at
  least 3 of the "found" places (Gragoatá, Morro do Embaixador, Serra do Vulcão) have a real source
  but sit outside Rio de Janeiro's actual municipal boundary (Niterói, São João de Meriti, Nova
  Iguaçu respectively) — the same failure mode that already hit once during sourcing (Casa de Cultura
  de Nova Iguaçu). This is now a confirmed recurring pattern, not a one-off — an explicit municipal-
  boundary check needs to be added to the grounding pipeline before scaling to the remaining ~249
  places, not fixed case-by-case. The other ~415 unnarrated places (museums, cultural centers,
  monuments, etc.) haven't been through this triage yet. Photos and audio/TTS generation — both fully
  specced in the design doc — have **not started at all**.
- **Everything else** (guide runtime, backend, ops, admin/mobile): not started.

## Where to find more

- `docs/superpowers/specs/` — dated decisions and research (design doc, roadmap v2, backend stack
  decision, AI-judgment research), each a snapshot of reasoning at the time.
- `docs/superpowers/plans/` — per-feature TDD implementation plans.
- `CLAUDE.md` — engineering conventions, commands, repo structure (including why the sourcing code
  lives on a separate branch/worktree, not `master`).
