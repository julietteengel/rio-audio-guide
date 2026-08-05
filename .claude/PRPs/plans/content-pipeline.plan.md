# Feature: Content Pipeline — Phase 2 (grounding → narration → translation → verification)

> Retroactive plan: this phase was already ~40% underway (441/2230 places narrated, methodology
> established) before this plan was written. It documents the current approach and the concrete
> remaining work, per the PRD's Phase 2 entry, rather than re-deriving a methodology already proven
> in production use. Where work already happened without a plan (the July grounding/narration passes),
> this plan does not redo it — it picks up from the actual current state.

## Summary

Bring the sourced 2230-place candidate list to a credible v1 launch state: every place that has a real,
verifiable source gets a grounded, zero-hallucination narration in FR, translated to EN/ES/PT, and
checked by the anti-hallucination judge before being considered publishable. Places with no findable
source get no narration — routed to a manual-write queue instead, not skipped silently.

## User Story

As a tourist walking past a covered site
I want the narration I hear to be factually accurate and specific to that place
So that I trust the app enough to keep using it, instead of getting generic or wrong information

## Problem Statement

At the last checkpoint (2026-07-24), 441/2230 places (20%) had FR narration, 422 had all 4 languages,
and only 131 of those had passed the anti-hallucination judge. Work paused for two weeks. Of the 1789
remaining unnarrated places, one category alone (`landmark_and_historical_building`, 1374 places) was
just confirmed 78% noise by direct classification — meaning the honest remaining grounding-worthy pool
is smaller and more specific than the raw unnarrated count suggested.

## Solution Statement

1. Finish triaging noise out of the remaining categories the same way `landmark_and_historical_building`
   was triaged (sample-audit first, classify at scale only if the category shows real noise).
2. Scale the grounding-search canary (in progress) to the full triaged candidate pool, using cached
   Wikidata extracts first (free), then Wikipedia geosearch, then targeted web search as last resort —
   in the order already validated to work, spacing batches to avoid the documented WebSearch quota wall.
3. Generate FR narration for everything grounded, following the already-calibrated style (3 reference
   examples, varied openings, source-density-proportional length, zero invented facts).
4. Translate to EN/ES/PT using the already-established method (oral register, duration-equivalent,
   untranslated proper nouns).
5. Extend the anti-hallucination judge to every narration that hasn't been checked yet — establishing
   the *true* source per narration first, since the first judge pass had ~50% false positives from
   comparing against the wrong stored source-extract.

## Metadata

| Field | Value |
|---|---|
| Type | ENHANCEMENT (extends existing, working pipeline) |
| Complexity | MEDIUM — mostly orchestration/scale, methodology already proven |
| Systems Affected | `pipeline/curation/` (scripts + data snapshots); no backend/app systems yet |
| Dependencies | Wikidata SPARQL, Wikipedia API, WebSearch tool, Nominatim (indirect, via sourcing) |
| Estimated Tasks | 7 (below) |

---

## Current State vs Target State

**BEFORE (now)**
- 441/2230 places narrated FR (20%), 422 in all 4 languages
- 131 narrations anti-hallucination-checked
- `landmark_and_historical_building`: 1374 unnarrated, now triaged to 307 real candidates
  (`pipeline/curation/landmark_classification.csv`)
- 60-place grounding canary on the triaged set: batch A done (17/30 found, 57%), batch B stuck/pending
- Other categories (~415 places: museums, cultural centers, monuments, etc.) not yet triaged

**AFTER (target for this phase, not necessarily 100%)**
- A defined, non-noisy subset of the 2230 has complete, judge-verified 4-language narration
- Every remaining category has been through the same noise-triage `landmark_and_historical_building` got
- Places without a real source are in an explicit manual-write queue, not silently absent
- Coverage target itself is **open** — see PRD "Open Questions"; this plan produces the pipeline
  capability, not a committed final percentage

---

## Mandatory Reading

| Priority | File | Why |
|---|---|---|
| P0 | `docs/superpowers/specs/2026-07-23-roadmap-v2-agentic-architecture.md` | Full grounding/narration/translation/judge methodology and every known failure mode (rate limits, quota exhaustion, judge false-positive cause) |
| P0 | `pipeline/curation/landmark_classification.csv` | The noise-triage output driving current Phase 2 work |
| P1 | `pipeline/curation/build_narrations_multi.py`, `narrations_data_part1-3.py` | Actual narration style/structure to mirror for new entries |
| P1 | `pipeline/curation/fetch_extracts_batched.py`, `enrich_grounding_geosearch.py` | Grounding fetch patterns already fixed for known bugs (missing `dist` field, `exlimit`, distance-based matching) |
| P2 | `pipeline/curation/wikidata_bulk_extracts.json` | 453-entry grounding cache — check before any new network call |

---

## Patterns to Mirror

**ZERO_HALLUCINATION_RULE** (from roadmap v2 spec, non-negotiable):
> Narration FR strictement à partir du texte de grounding récupéré (jamais de fait, date ou chiffre non
> présent dans la source). Un lieu sans grounding réel ne reçoit pas de narration.

**GROUNDING_SOURCE_PRIORITY** (established order, don't reorder without reason):
1. Check `wikidata_bulk_extracts.json` cache by name — free, no network.
2. Wikidata SPARQL bulk query (if a whole new zone/category needs grounding at scale) — one query
   covers the area, not one per place.
3. Wikipedia geosearch (pt, fallback en) — per-place, with the haversine-distance fix and stopword-
   filtered name matching already applied in `enrich_grounding_geosearch.py`.
4. Targeted web search (WebSearch, falling back to WebFetch against Brave/DuckDuckGo results pages
   when WebSearch quota is exhausted) — last resort, most expensive, most quota-constrained.

**NARRATION_STYLE** (from roadmap v2, second calibration pass):
- No fixed length — richness follows real source density, never padded.
- Varied opening per place (visual incongruity, sensory invitation, human angle, past/present
  contrast, question, in medias res) — never the same opening formula twice in a batch.
- Real narrative arc, not a list of facts.

---

## Files to Change

| File | Action | Justification |
|---|---|---|
| `pipeline/curation/landmark_classification.csv` | READ | Source of truth for which landmark-category places to search |
| `pipeline/curation/wikidata_bulk_extracts.json` | UPDATE | Append any new bulk-fetched extracts (other categories) |
| `pipeline/curation/places_clean_v10.csv` (new snapshot) | CREATE | Next checkpoint after this phase's narration/translation work — don't overwrite v9 |
| `pipeline/curation/*_classification.csv` (new, per other category) | CREATE | Same noise-triage pattern applied to museum/cultural_center/monument/etc. if a sample audit shows real noise there too |
| `pipeline/curation/anti_hallucination_source_map.csv` (new) | CREATE | Explicit narration→true-source mapping, to fix the judge's false-positive cause before extending it |

---

## NOT Building (Scope Limits)

- No audio/TTS generation in this phase — text content only.
- No backend/database work — output stays in CSV snapshots, same as today.
- No changes to the sourcing pipeline itself (Phase 1, separate branch/plan).
- No guaranteed 100% coverage — partial, honest coverage is the accepted v1 posture per the PRD.

---

## Step-by-Step Tasks

### Task 1: Resolve the grounding-search canary
- **ACTION**: Get batch B's actual 30-line result (it stalled twice returning status updates instead
  of data); if it can't produce a clean result, either retry it once more or treat batch A's 30-place,
  57%-found result as sufficient signal.
- **VALIDATE**: A combined hit-rate number exists for the 60-place canary, with the failure-pattern
  notes batch A already surfaced (micro-toponyms in under-documented areas fail search reliably;
  try IPHAN/INEPAC/prefeitura.rio registries directly for those instead of generic web search).

### Task 2: Sample-audit the remaining ~415 places in other categories
- **ACTION**: Same method as the landmark-category audit — pull a ~50-80 place random sample across
  museum/cultural_center/monument/topic_concert_venue/etc., manually or via a small classification
  pass, and decide whether full triage is warranted per category (it may not be — these are more
  specific Overture categories, expected to be cleaner).
- **VALIDATE**: A noise percentage per category, decision documented on whether to triage or search directly.

### Task 3: Scale grounding search to the full triaged pool
- **ACTION**: Run the established source-priority order (cache → SPARQL → Wikipedia geosearch → web
  search) across the ~249 landmark-category candidates needing fresh grounding (196 minus canary'd 53
  once Task 1 resolves) plus whatever categories Task 2 clears for direct search — batched, spaced out
  per the documented quota constraint (2-3 concurrent lots max).
- **VALIDATE**: A grounding-found/not-found count per batch, logged the same way the canary was.

### Task 4: Generate FR narration for everything grounded
- **ACTION**: Apply the calibrated style (see Patterns to Mirror) to every newly-grounded place.
- **VALIDATE**: Spot-check a sample against the zero-hallucination rule — every fact traceable to the
  stored source text.

### Task 5: Translate to EN/ES/PT
- **ACTION**: Apply the established translation method to the new FR narrations only (not re-translate
  existing ones).
- **VALIDATE**: Duration-equivalence spot-check (±15%), proper nouns left untranslated.

### Task 6: Build the true-source map, then extend the anti-hallucination judge
- **ACTION**: Before running the judge on the ~310 already-narrated-but-unchecked places plus all new
  ones, build an explicit mapping from narration → the actual source text it was written from (fixing
  the root cause of the first pass's ~50% false-positive rate — it assumed one reference file for the
  whole corpus).
- **VALIDATE**: Judge run produces a CLEAN/ISSUE verdict per place with the correct source cited in its
  reasoning, not a mismatched one.

### Task 7: Route ungrounded places to a manual-write queue
- **ACTION**: For every place that exhausted the full search-priority order with no credible source,
  add it to an explicit queue file rather than letting it silently disappear from the narrated set.
- **VALIDATE**: Queue file exists, is non-empty (it will be — most places won't ground), and every
  entry has a reason (e.g. "no Wikipedia, no web search hit, tried registries X/Y").

---

## Testing Strategy

There is no automated test suite for `pipeline/curation/` (unlike `pipeline/sourcing/`, which has
`pytest`) — this is intentionally ad-hoc data-processing work, not a tested package. Validation here
means: sample spot-checks against the zero-hallucination rule, before/after counts logged per task,
and the judge pass itself acting as the closest thing to an automated check this phase has.

### Edge Cases Checklist

- [ ] A place with a source that's about a *different* nearby place (name/coordinate mismatch) — already
      hit twice (Gragoatá/Niterói, Paço Real coordinates) — needs a scope-check step before publishing.
- [ ] A place whose only source is thin (a few words) — narration must stay honestly short, not padded.
- [ ] A grounded place whose source turns out to be about the wrong entity entirely on closer read.
- [ ] WebSearch quota hit mid-batch — fall back to WebFetch against search engine result pages.

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| WebSearch quota exhaustion stalls Task 3 | HIGH | MEDIUM (delays, not data loss) | Space batches 2-3 concurrent max; WebFetch fallback engines already identified |
| Judge false positives repeat because true-source mapping (Task 6) is skipped | MEDIUM | HIGH (bad narrations ship as "verified") | Task 6 is ordered before the judge extension specifically to prevent this |
| Coordinate/perimeter errors ship unnoticed | MEDIUM | MEDIUM (wrong-city content in the guide) | Flag any FOUND source whose described location doesn't match the stored coordinates, per Task 3 validation |
| Sample audit (Task 2) shows other categories are noisy too, expanding scope | LOW-MEDIUM | LOW (more triage work, not wasted work) | Accept and triage; this plan already budgets for it as a real task, not a surprise |

## Notes

Redis/Kafka/Mongo were considered and cut from the *backend* (Phase 4), not this phase — irrelevant
here since this phase has no database or queue involved yet, but noted to avoid confusion since both
decisions were made close together in the project timeline.
