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
never the primary justification.** The exception is the guide runtime and the backend's domain/ports
layers, deliberately hand-written (not AI-generated) specifically for their learning/portfolio value —
see Phases below and the 2026-08-15 note on the rest of the backend.

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
| Guide runtime (v1 scope) | Mobile app logic (React Native) — proximity-triggered narration + local tour memory, no server-side agent | Not started. Scope resolved 2026-08-12 — live Q&A/tool calling/human-in-the-loop deferred to Phase 2, per `docs/superpowers/specs/2026-08-12-guide-runtime-v1-scope-design.md` |
| Backend | Go, hexagonal architecture + DDD, PostgreSQL/PostGIS, RabbitMQ (TTS job queue), K8s (EKS demo)/Scaleway (real prod) | In progress — domain layer (Place/Script/AudioFile) and ports **written by hand** by the founder. Remaining layers (Postgres/RabbitMQ adapters, HTTP API, CI/CD) switched to **AI-written, human-reviewed** on 2026-08-15 — a deliberate, time-boxed call, not a silent default. See note below. |
| Ops depth | CI/CD canary+rollback, distributed observability, security guardrails, compliance | Not started — explicitly scoped in depth, not minimal, per roadmap v2 |
| Admin dashboard + mobile app | React Native app, admin dashboard | Not started |

Redis and Kafka were both proposed and then explicitly cut (2026-08-04) for lack of a measured,
current need — see the backend-stack-decision spec before re-adding either. Raised again on
2026-08-15 and cut again for the same reason: adding either now would contradict the founder's own
documented reasoning, not extend it.

**Note on the 2026-08-15 hand-written → AI-written switch, refined same day:** domain/ports stayed
hand-written throughout. The remaining backend layers briefly switched to "AI writes by default" —
revised a few hours later, same day, back to **founder writes by default, with Claude's help; Claude
writes a given piece only when explicitly asked for that piece**, not as a standing default. Applies
to Postgres adapters (already AI-written under the brief window — kept as-is, not redone) and
everything after: RabbitMQ (publisher done under the old default; the worker/AWS S3/HTTP API/CI-CD
are unplanned as of this note — see "what's not yet planned" below). Revisit after 2026-08-18.

**What's not yet planned** (discussed in conversation, never through brainstorming/writing-plans):
RabbitMQ consumer/worker, AWS S3 adapter, HTTP API, CI/CD pipeline, pipeline-to-Postgres import
script. Needs its own spec/plan cycle before implementation, per the project's own convention — not
to be implemented ad hoc off conversational discussion alone.

## Phases / sequencing

1. **Sourcing** — done, unmerged (`sourcing-pipeline` branch). Needs a real human review pass before
   merge; an automated subagent review approved it, which is not the same thing.
2. **Content pipeline** — in progress. See status below.
3. **Guide runtime** — a mobile app feature (proximity + local memory only, see scope decision above),
   not a blocking server-side phase; can proceed alongside backend rather than before it.
4. **Backend** — independent of Guide runtime now that it has no server-side component; sequence by
   availability, not by dependency.
5. **Ops depth** — built as its own deliberate phase, not bolted on at the end.
6. **Admin dashboard + mobile app** — last.

## Current status (as of 2026-08-12 — keep this section updated, don't let it drift)

- **Sourcing**: rebuilt from scratch after an accidental data-loss (see
  `docs/superpowers/specs/2026-08-04-backend-stack-decision.md`-era commits and git history on
  `sourcing-pipeline` for context) — 3858 raw places, category allowlist restricted to strict cultural
  scope (nature/landscape categories dropped). Branch complete but unmerged, pending human review.
- **Content pipeline**: full CULTURAL/NATURAL/NOISE triage done on the three noisy Overture categories
  (`landmark_and_historical_building`, `topic_concert_venue`, `cultural_center`) plus the trusted
  categories → **773 CULTURAL places**. Municipal-boundary check (Nominatim reverse-geocode, not just
  proximity to stored coordinates) removed **77 places actually outside Rio** (mostly Niterói) →
  **696 boundary-verified places**, the real base for grounding. Grounding switched from a per-place
  Wikipedia geosearch (throttled by Wikimedia's anti-scraping tier even with a compliant User-Agent)
  to a **bulk SPARQL query** (`wikibase:around`, one request, ~1300 candidate Wikidata items) + local
  spatial/name-overlap matching — **193 places grounded** with a real source. Of those, 154 got fresh
  FR/EN/ES/PT narration this session (`narrations_data_part4.py`), on top of 33 that already had
  narration surviving from before the data loss — **187 places narrated across parts 1-4**. 7 places
  hit a real source that didn't actually describe them (wrong entity, thin/off-topic extract) and were
  routed to `ungrounded_queue_v1.csv` with a reason, not silently dropped. The remaining ~500
  boundary-verified places haven't been grounded yet (per-place fallback search or manual web search
  needed for places with no Wikidata/Wikipedia presence). Anti-hallucination judge has not been re-run
  on this new corpus yet. Photos and audio/TTS generation — both fully specced in the design doc —
  have **not started at all**.
- **Everything else** (guide runtime, backend, ops, admin/mobile): not started.

## Where to find more

- `docs/superpowers/specs/` — dated decisions and research (design doc, roadmap v2, backend stack
  decision, AI-judgment research), each a snapshot of reasoning at the time.
- `docs/superpowers/plans/` — per-feature TDD implementation plans.
- `CLAUDE.md` — engineering conventions, commands, repo structure (including why the sourcing code
  lives on a separate branch/worktree, not `master`).
