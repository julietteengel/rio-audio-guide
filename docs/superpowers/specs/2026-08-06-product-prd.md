# Rio Audio Guide

> **2026-08-19 note**: moved here from `.claude/PRPs/prds/` (the `prp-core` plugin's own convention),
> consolidating with this project's one actively-maintained documentation system. Frozen at 2026-08-06
> — the Implementation Phases table below is stale (Phase 2 is now well past what's described, Phase 4
> Backend has substantial real progress, `sourcing-pipeline`/`frontend` have since merged to `master`).
> Kept as a snapshot for the real research/evidence/decisions-log content, not as current status —
> see `mission.md` for that.

> Generated via the PRP methodology (`prp-core`), retroactively — synthesized from existing project
> artifacts (`docs/superpowers/specs/`, `mission.md`) rather than re-running the interactive discovery
> questions, per explicit instruction not to relitigate decisions already made. Sections marked
> **TBD** are genuine gaps, not filled in with invented plausible-sounding content.

## Problem Statement

Tourists visiting Rio de Janeiro who want real cultural/historical context on what they're seeing have
no good self-guided option: the one free incumbent (Passeio Carioca) has broad coverage but weak
execution — UI translated to English while content stays in Portuguese, almost no real audio, no
category filtering on a saturated map — and shows low real adoption (23 App Store reviews) despite
municipal backing. International competitors (VoiceMap, izi.TRAVEL, GPSmyCity, Summer AI...) have no
Rio-specific, natively multilingual, locally-verified offering.

## Evidence

- Direct manual review of Passeio Carioca (screenshots, not scraped — see legal note in design doc)
  confirms the UI/content-language mismatch and low review count.
- 7 independent place-sourcing datasets (Overture, Wikidata/IPHAN, feiras registry, IRPH, Riotur,
  MuseusBr, manual curation) were each checked and found individually incomplete — no single official
  source covers Rio's cultural sites, confirmed empirically (e.g. Santuário do Zé Pelintra, IPHAN-listed
  in 2022, appears correctly in none of the automated sources).
- International competitor list confirms "AI geolocated audio guide" is a validated, populated product
  category — not proof of demand for *this* specific product, but evidence the mechanism works.
- No direct evidence yet (survey, pilot, hotel LOI) that Rio hotels/agencies would actually buy a
  white-label version — **TBD, needs validation** before the B2B distribution bet is treated as proven.

## Proposed Solution

A hands-free, geolocation-triggered audio guide for Rio's cultural/heritage sites, in real (not
machine-literal) EN/FR/PT/ES, sold white-label to hotels/agencies, built with a batch AI content
pipeline (grounded, zero-hallucination narration) feeding a runtime app with offline-first maps and
proximity prompts — chosen over a live RAG/chat approach because the latter would be feature creep for
a tourist product (see `docs/superpowers/specs/2026-07-21-rio-audio-guide-design.md`, "Ce que ce n'est pas").

## Key Hypothesis

We believe locally-verified, natively multilingual audio content delivered via proximity prompts will
be preferred by tourists over Passeio Carioca's free-but-shallow alternative, enough that hotels/
agencies will pay to white-label it.
We'll know we're right when **TBD — needs a measurable target** (e.g. a pilot hotel signs on, or a
free-tier usage/completion-rate threshold is hit). Not yet defined; flagging rather than inventing a
number.

## What We're NOT Building

- Live RAG/chat during the visit (v1) — forcing it in would be feature creep for a tourist product;
  the batch content pipeline already produces grounded narration without it. Reconsider only if a real
  user need emerges once the runtime guide exists.
- Bars and restaurants — recentered on heritage sites + recurring cultural events only.
- Teen/child voices and scripts — costed at scope×3, not justified for a first launch; revisit Phase 2.
- Self-hosted TTS infrastructure — API cost is far below the self-hosting break-even point at MVP volume.
- City-wide *complete* coverage — partial coverage (whatever the content pipeline can honestly ground)
  is acceptable at launch; a place without a real source gets no narration, ever, rather than an
  invented one.
- Kafka, MongoDB, Redis in the backend — proposed once to match a target job posting's keyword list,
  cut for lack of a real measured need (`docs/superpowers/specs/2026-08-04-backend-stack-decision.md`).

## Success Metrics

| Metric | Target | How Measured |
|---|---|---|
| Content coverage (places with full 4-language narration) | **TBD** | count of narrated places / total sourced places |
| Anti-hallucination judge pass coverage | 100% of published narrations | judge-pass log vs. published set |
| Hotel/agency pilot signed | **TBD — no target set yet** | — |
| App usage / completion rate | **TBD — no target set yet** | — |

## Open Questions

- [ ] What's the actual go/no-go bar for "ready to pitch a hotel" — a place count? A city zone fully covered?
- [ ] Auth model for `users` table (email/password vs magic link) — flagged open in the design doc, still open.
- [ ] Final list of places for a first pilot zone, or full-city launch with partial coverage?
- [ ] Claude vs GPT-4o for narration generation — design doc calls for a 3-4 place side-by-side comparison that was never run; narration so far has used Claude by default without that comparison.

---

## Users & Context

**Primary User**
- **Who**: An international or domestic tourist in Rio without a Brazilian data plan, walking a
  neighborhood (e.g. Santa Teresa/Lapa) who wants real historical/cultural context on what they're
  seeing, in their own language.
- **Current behavior**: Either no guide at all, a generic paper/hotel-desk recommendation, or
  Passeio Carioca (free but shallow/wrong-language content).
- **Trigger**: Proximity to a heritage site or recurring cultural event while walking.
- **Success state**: Gets a short, accurate, well-narrated audio story in their language, offline,
  without having to search for it.

**Job to Be Done**
When I'm walking past something historically or culturally interesting in Rio, I want to understand
what it is and why it matters, in my own language, without needing data signal or research effort, so
I can get more out of the trip without breaking my walk.

**Secondary user**: hotel/agency partner who white-labels the guide for their guests (B2B distribution
channel, not a distinct product surface beyond branding/admin dashboard).

**Non-Users**
Locals (already know the content), budget-conscious backpackers unwilling to pay/download anything
premium (v1 has no confirmed pricing model — **TBD**), users wanting live conversational Q&A (out of
scope v1).

---

## Solution Detail

### Core Capabilities (MoSCoW)

| Priority | Capability | Rationale |
|---|---|---|
| Must | Geolocation proximity prompt (not auto-play) | Core mechanic; non-intrusive per design decision |
| Must | 4-language grounded, zero-hallucination narration | Differentiator vs. incumbent's language/quality gap |
| Must | Offline-first maps + downloaded audio | Most tourists lack Brazilian data plans — not a nice-to-have |
| Must | Category-filterable map | Named incumbent weakness (saturated, unfiltered map) |
| Should | Karaoke-style text/audio sync | Cheap given TTS "with-timestamps" endpoint; UI work remains |
| Should | Admin dashboard (editorial review, partner branding) | Needed for B2B model and for the mandatory human review step |
| Could | Teen/child voice variants | Deferred to Phase 2 after cost analysis |
| Could | Route/itinerary calculation | Nice-to-have, not core to the proximity-prompt mechanic |
| Won't | Live chat/RAG Q&A during the visit | See "What We're NOT Building" |
| Won't | Bars/restaurants coverage | Recentered scope, see design doc |

### MVP Scope

Whatever the content pipeline can honestly ground and narrate across Rio de Janeiro municipality, in
4 languages, single adult voice, with human-reviewed scripts before synthesis, delivered through an
offline-capable app with proximity prompts and a category-filtered map.

### User Flow

Tourist downloads the app/route at the hotel (wifi) → walks with GPS/geofencing running fully offline
→ gets a proximity prompt near a covered site → taps to hear the narration in their language → optional
route continues to the next covered site.

---

## Technical Approach

**Feasibility**: HIGH for the content and sourcing pipeline (already built/validated in production-like
conditions — see Decisions Log). MEDIUM for the runtime guide (not started; agentic memory/tool-calling
architecture is deliberately the least de-risked part, chosen for its learning value, not because it's
already proven here).

**Architecture Notes**
- Batch content pipeline (Python) is architecturally separate from the runtime guide (the actual
  agentic system) — see `docs/superpowers/specs/2026-07-23-roadmap-v2-agentic-architecture.md` for the
  full "don't over-agentify the batch" reasoning.
- Backend: Go, hexagonal architecture + DDD, PostgreSQL/PostGIS, RabbitMQ (TTS job queue), K8s (one-off
  EKS demo) / Scaleway (real prod) — see `docs/superpowers/specs/2026-08-04-backend-stack-decision.md`.
  Deliberately hand-written rather than AI-generated for its learning/portfolio value.

**Technical Risks**

| Risk | Likelihood | Mitigation |
|---|---|---|
| Wikimedia/Wikipedia rate limits or WebSearch session quota exhaustion mid-pipeline | HIGH (already hit twice) | Batched SPARQL queries over per-place calls; fallback search engines (Brave/DuckDuckGo via WebFetch); space out concurrent search batches |
| A large share of a source category is noise (real-estate/business listings mislabeled as landmarks) | HIGH (confirmed: 78% of `landmark_and_historical_building`) | Cheap LLM classification triage before spending search budget, validated by sample audit |
| Coordinate/perimeter errors placing a candidate outside Rio's actual municipal boundary | MEDIUM (confirmed twice: Casa de Cultura de Nova Iguaçu, Gragoatá) | Needs a dedicated geographic-scope audit pass before final publish, not yet scheduled |
| Anti-hallucination judge comparing against the wrong stored source-extract | MEDIUM (already caused ~50% false positive rate in first pass) | Establish true source per narration before judging, not yet systematized |
| Automated code review (subagent) mistaken for sufficient validation before merge | MEDIUM (already happened once — `sourcing-pipeline` branch) | Human review is now an explicit gate before any merge, not just SDD's own review loop |

---

## Implementation Phases

<!--
  STATUS: pending | in-progress | complete
  PARALLEL: phases that can run concurrently (e.g., "with 3" or "-")
  DEPENDS: phases that must complete first (e.g., "1, 2" or "-")
  PRP: link to plan file
-->

| # | Phase | Description | Status | Parallel | Depends | PRP Plan |
|---|---|---|---|---|---|---|
| 1 | Sourcing pipeline | Multi-source place candidates, deduplicated, city-wide | complete (code), **pending human review before merge** | - | - | `docs/superpowers/plans/2026-07-21-lieu-sourcing-pipeline.md` |
| 2 | Content pipeline | Grounding, narration, translation, anti-hallucination verification, at acceptable coverage | in-progress | - | 1 | `docs/superpowers/plans/2026-08-06-content-pipeline.plan.md` |
| 3 | Guide runtime (agentic core) | Turn memory, tool calling, unplanned-question handling, voice I/O, prompt-injection guard | pending | - | 2 (partial coverage acceptable) | - |
| 4 | Backend | Go/hexagonal/DDD API, Postgres/PostGIS, RabbitMQ TTS queue, K8s/Scaleway deploy | pending | with 3 | 1 | - |
| 5 | Ops depth | CI/CD canary+rollback, distributed observability, security guardrails, compliance | pending | with 3, 4 | 4 | - |
| 6 | Admin dashboard + mobile app | Editorial review UI, partner branding/stats, React Native app | pending | - | 3, 4 | - |

### Phase Details

**Phase 1: Sourcing pipeline**
- **Goal**: A deduplicated, city-wide candidate place list from all automated sources.
- **Scope**: Already delivered — 2230 candidate places.
- **Success signal**: Human (not just automated-subagent) code review completed and branch merged to `master`.

**Phase 2: Content pipeline**
- **Goal**: Ground, narrate (FR), translate (EN/ES/PT), and anti-hallucination-verify enough of the
  2230 places to be a credible v1 launch set.
- **Scope**: See `docs/superpowers/plans/2026-08-06-content-pipeline.plan.md` for current in-flight scope.
- **Success signal**: A defined, non-noisy subset of places has 4-language narration that has passed
  the anti-hallucination judge — exact coverage bar is one of this PRD's open questions.

**Phase 3: Guide runtime**
- **Goal**: Build the actual agentic differentiator — the one piece of this project the roadmap v2 spec
  identifies as deserving full agentic architecture (memory, tool calling, human-in-the-loop).
- **Scope**: TBD at build time — not yet planned in detail.
- **Success signal**: TBD.

**Phase 4: Backend**
- **Goal**: Serve content + geo queries + TTS job orchestration through a hexagonal/DDD Go API.
- **Scope**: TBD at build time.
- **Success signal**: TBD.

**Phase 5: Ops depth**
- **Goal**: Build production-grade delivery/observability/security depth as a deliberate phase, not an
  afterthought — explicit decision in roadmap v2.
- **Scope**: TBD at build time.
- **Success signal**: TBD.

**Phase 6: Admin dashboard + mobile app**
- **Goal**: The actual user-facing surfaces.
- **Scope**: TBD at build time.
- **Success signal**: TBD.

### Parallelism Notes

Phases 3 (guide runtime) and 4 (backend) can run in parallel once Phase 2 has *some* usable content —
they don't block each other technically, though a single developer working both hand-written phases
sequentially may be more realistic than true parallelism here.

---

## Decisions Log

| Decision | Choice | Alternatives | Rationale |
|---|---|---|---|
| Product scope beyond "audioguide" | Grounded batch content pipeline, not live RAG/chat | Live conversational guide | Avoids feature creep; live agentic value reserved for the runtime guide, not content generation |
| Geographic scope | All of Rio municipality | Santa Teresa/Lapa only (original MVP) | Expanded 2026-07-23, see roadmap v2 |
| V1 voice | Single adult voice, 4 languages | Adult+teen+child × 4 languages | Cost triples for teen/child scope; deferred to Phase 2 |
| TTS hosting | Cloud API (ElevenLabs-type) | Self-hosted GPU pod | MVP volume far below self-hosting break-even (~5-10M chars/month) |
| Production hosting | Scaleway VPS | Permanent EKS control plane | ~$73/mo control-plane cost disproportionate for a naissant product; EKS kept as a one-off technical demo only |
| Backend language/architecture | Go, hexagonal + DDD | — | Matches target job's actual evaluated skills, and is a genuine fit for the content-publication domain's real invariants |
| Message broker | RabbitMQ only | RabbitMQ + Kafka | Two brokers for one small app has no real justification; Kafka cut 2026-08-04 |
| Secondary datastore | None (JSONB in Postgres) | MongoDB for raw scraped payloads | A second database to operate isn't justified by a need Postgres already covers |
| Cache layer | Deferred (not v1) | Redis from day one | No measured slow query or real traffic yet to justify it; add only on a measured need |
| `sourcing-pipeline` branch merge | Blocked pending human review | Merge on automated subagent review approval alone | Automated review approval is not equivalent to human understanding/ownership of the code |

---

## Research Summary

**Market Context**
Passeio Carioca (free, municipal): broad coverage, weak execution (untranslated content, near-zero
real audio, unfiltered saturated map, low real adoption). International competitors (VoiceMap,
izi.TRAVEL, GPSmyCity, Summer AI, StreetPhonia, AI TourMate, MyGuide, Gamana) validate the product
category but have no Rio-specific, natively multilingual offering.

**Technical Context**
7-source place sourcing was empirically necessary (no single source ≥ ~60% complete). Grounding via
bulk Wikidata SPARQL queries (one query for the whole city) is far more efficient than per-place
Wikipedia calls, which hit Wikimedia's 2026 anonymous-traffic rate tier. Targeted web search for
places without a Wikipedia article works roughly 2/3 of the time when the WebSearch session quota
isn't already exhausted. A noisy Overture category (`landmark_and_historical_building`) was confirmed
78% noise by direct classification+audit, not just suspected.

---

*Generated: 2026-08-06 (synthesized from existing project artifacts, not a fresh interactive session)*
*Status: DRAFT — several Success Metrics and Phase 3-6 scopes are genuinely TBD, not yet defined*
