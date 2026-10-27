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
never the primary justification.** Through 2026-09-06, the backend's domain/ports layers were an
exception to AI authorship, hand-written specifically for their learning/portfolio value — see Phases
below and the 2026-08-15 note on the rest of the backend. That exception was lifted 2026-09-06: Claude
may now write domain/ports code too, human-reviewed like every other layer.

## What this is not

- Through 2026-09-06, not a live RAG chat / research copilot — explicitly considered and rejected as
  feature creep for a tourist audio guide, AI here being a content-production tool, not the product
  itself. Reversed 2026-09-06: a real per-place Q&A assistant (grounded in that place's own narration
  content, with an honestly-marked general-knowledge fallback) is now in scope — see
  `docs/superpowers/specs/2026-09-06-place-assistant-design.md`. Distinct from the itineraries feature's
  own chat, which composes routes across places rather than answering about one.
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
- **Explicitly out of scope for v1**: bars/restaurants, teen/child voices, affiliate
  monetization, self-hosted TTS infrastructure, city-wide coverage beyond what the sourcing pipeline
  actually grounds (partial coverage is acceptable at launch — see Content pipeline status).

## Architecture at a glance

| Sub-system | Stack | Status |
|---|---|---|
| Sourcing pipeline | Python (Overture Maps, Wikidata, feiras registry) | Built, tested, merged to `master` (PR #2) |
| Content pipeline (batch) | Python (grounding, narration, translation, anti-hallucination judge) | Done for the ~252 places in the database: grounded, narrated EN/FR/PT/ES, verified (see Current status). The rest of the 696 boundary-verified places are not yet grounded |
| Guide runtime (v1 scope) | Mobile app logic (React Native) — proximity-triggered narration (background geofencing) + offline downloads, no server-side agent | Built (proximity notifications, real offline downloads); proximity needs a physical device to verify |
| Backend | Go, hexagonal architecture + DDD, PostgreSQL/PostGIS, RabbitMQ (TTS job queue), Redis, S3 | Built and tested (auth with email verification via SES, places, scripts, audio, AI itineraries, place assistant, featured itineraries). Deployed on AWS (EC2 + RDS, Terraform in `backend/deploy/terraform`) and exercised from a real iOS build in Oct 2026, then torn down to avoid cost. Helm/KEDA/Istio/blue-green manifests exist but were only validated on a local `kind` cluster, never on a real EKS cluster. No Scaleway deployment exists. |
| Ops depth | CI/CD canary+rollback, distributed observability, security guardrails, compliance | Partial — GitHub Actions CI (lint, test, govulncheck, integration tests against real services) exists. No metrics/alerting. Production AWS credentials are still root rather than the scoped `rio-cicd` user |
| Mobile app | React Native (Expo), iOS first | Built; standalone EAS preview build field-tested in Rio. Not on the App Store |
| Admin dashboard | — | Not started |

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

**2026-09-06 update:** the domain/ports hand-written exception itself is lifted — Claude may now write
`internal/domain/`/`internal/ports/` code too, human-reviewed like everything else. First exercised for
the new `Itinerary` domain concept (see `docs/superpowers/specs/2026-09-06-ai-itineraries-design.md`).
Existing hand-written domain code (Place/Script/AudioFile) is not being retrofitted or redone — this
only changes who may write *new* domain/ports code going forward.

**What's not yet planned** (discussed in conversation, never through brainstorming/writing-plans):
RabbitMQ consumer/worker, AWS S3 adapter, HTTP API, CI/CD pipeline, pipeline-to-Postgres import
script. Needs its own spec/plan cycle before implementation, per the project's own convention — not
to be implemented ad hoc off conversational discussion alone.

## Phases / sequencing

1. **Sourcing** — done and merged (PR #2).
2. **Content pipeline** — in progress. See status below.
3. **Guide runtime** — a mobile app feature (proximity + local memory only, see scope decision above),
   not a blocking server-side phase; can proceed alongside backend rather than before it.
4. **Backend** — independent of Guide runtime now that it has no server-side component; sequence by
   availability, not by dependency.
5. **Ops depth** — built as its own deliberate phase, not bolted on at the end.
6. **Admin dashboard + mobile app** — last.

## Current status (as of 2026-10-27 — keep this section updated, don't let it drift)

- **Sourcing**: 3,858 raw places (Overture Maps, Wikidata, feiras livres registry), deduplicated
  conservatively after an earlier false merge dropped a real place. Merged to `master`.
- **Content pipeline**: triage down to 773 cultural places, 696 after a Nominatim municipal-boundary
  check (77 were outside Rio, mostly Niterói). About 252 places have grounded, narrated FR/EN/PT/ES
  scripts in the database. Anti-hallucination verification report: 132 clean, 28 with issues, 92
  unverifiable; the 9 severe findings were fixed and the unverifiable set triaged
  (`pipeline/curation/`). A translation-fidelity judge pass was also run. Content verification at
  larger scale is researched but parked (`2026-09-07-content-verification-research.md`).
- **Audio**: 203 ready audio files as of 2026-10-18, roughly 205 places still without audio, blocked on
  OpenAI credit. The audio was produced by standalone scripts (Google Cloud TTS and OpenAI TTS, Whisper
  for caption timing) that were **never committed** and are no longer on the dev machine. The worker in
  `backend/` still targets AWS Polly, so the committed code path and the audio actually in the database
  do not match. Reconstructing these scripts in the repo is open work.
- **Deployment**: AWS (EC2 + RDS + S3) was live in October 2026 and torn down with Terraform to avoid
  idle cost. It can be recreated from `backend/deploy/terraform`.
- **Open**: rotate production credentials from root to the scoped `rio-cicd` IAM user; bulk audio and
  Whisper timestamps for the remaining places; API pagination and rate limiting; retry/backoff on the
  mobile download path.

## Where to find more

- `docs/superpowers/specs/` — dated decisions and research (design doc, roadmap v2, backend stack
  decision, AI-judgment research), each a snapshot of reasoning at the time.
- `docs/superpowers/plans/` — per-feature TDD implementation plans.
- `CLAUDE.md` — engineering conventions, commands, repo structure and branching convention.
