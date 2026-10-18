# Anti-hallucination verification pass — 2026-10-17

## Why this exists

The project's own methodology doc (`docs/superpowers/specs/2026-07-23-roadmap-v2-agentic-architecture.md`)
records a judge pass completed 2026-07-24 on 131 places, reaching a "254/254 verified" checkpoint on
2026-08-19 (commit `2f76cf4`). But the backend database — where `cmd/import` actually loaded content
from `narrations_multi_full.csv`, an untracked, never-committed pipeline artifact — has no record of
which source each of its 994 scripts (252 places × up to 4 languages) was generated from:
`cmd/import` hard-codes `source_text` to an empty string on every import. There was no way to confirm
the checkpointed verification and the currently-imported content were the same thing, so this pass
re-verifies the actual database content against freshly re-resolved real sources, from scratch.

## Methodology

1. **Export** all 252 places currently in the backend DB (`places_export_for_verification.csv`).
2. **Reconstruct grounding** via each place's `wikidata_qid` (or a name+200m-distance Wikidata search
   fallback), reusing `enrich_grounding.py`'s exact matching method — the same one originally used to
   source narration grounding, to avoid reintroducing the "compared against the wrong source" bug this
   project has already been bitten by once. **161/252 (64%) matched a source; 91/252 (36%) found no
   close-enough match** (`places_grounding_for_verification_v1.csv`).
3. **First judge pass** (8 parallel agents, FR narration vs. the matched source) came back with ~140/160
   ISSUE — almost all false positives. Root cause, found by manually checking one flagged place
   (Biblioteca Nacional do Brasil) against the real, full Wikipedia article: `enrich_grounding.py`'s
   `wp_extract()` passes `exintro=1`, fetching only the lead paragraph. Every fact the judge had flagged
   as "absent" was actually present in the full article — the tool was comparing rich narration against
   a one-sentence stub, not a representative source.
4. **Fixed**: `refetch_full_grounding.py` reuses the already-resolved `matched_qid` (no need to redo
   Wikidata matching) and fetches the **full** article instead
   (`places_grounding_full_articles_v1.csv`, 161/161 succeeded).
5. **Second judge pass**, same 8-way parallel split, now against full articles — the results below.

## Results

| | Count |
|---|---|
| Judged CLEAN | 132 |
| Judged ISSUE (real finding) | 28 |
| **Cannot verify** (no matching source found, or no FR narration to check) | **92** |
| Total places in DB | 252 |

**The 92 unverifiable places are listed in full at the bottom of this report. They are not clean —
they are simply not checked, and must not be treated as passed.**

### Real findings (28), by severity

#### Severe — substantial invented content (names, figures, entire episodes)

- **National Museum of Brazil** — invented naturalist names (Maximilian zu Wied-Neuwied, Ludwig Riedel),
  an invented "World Bank loan refused" episode, a fabricated technical fire-cause account, invented
  post-fire restitution donors/figures, and the reopening year stated as 2027 vs. the source's 2026.
- **Palácio de são cristóvão** — Elias Antônio Lopes called a "marchand d'esclaves" (source: just a rich
  merchant), wrong renovation start year, invented painter/room detail, invented constitutional history,
  invented imperial collection donations, **the same fabricated air-conditioning fire-cause passage as
  the National Museum entry above** (see note below), wrong damage percentage (92.5% vs. source's ">90%").
- **Panteão Duque de Caxias** — wrong inauguration year, an entirely invented architectural history for
  a different building (architects, construction material), an invented present-day military command
  structure, none of it in the source.
- **Planetário da Gávea** — invented museum-ranking claim, invented visitor figures, a contradicted
  experiment count, an invented planetarium brand name, and multiple entirely invented
  programs/activities.
- **Sugarloaf Mountain (Pão de Açúcar)** — wrong etymology attributed to a named historian who isn't in
  the source, wrong cable-car history (name, dates, country of manufacture), invented cabin-replacement
  history, invented closure dates, a fabricated UNESCO World Heritage claim.
- **Instituto de Pesquisa e Memória Pretos Novos** — a named vice-roy and causal claim for the cemetery's
  founding, invented burial-count range, invented 2012/2017 excavation details including a named
  skeleton, none of it in the source.
- **Museu da República (Palácio do Catete)** — the entire final third of the narration (founding decree,
  autonomy date, closure/reopening dates, detailed acervo statistics by named sub-collection) absent
  from the source.
- **Centro Cultural Cartola** — an invented second museum, an invented Petrobras-sponsored orchestra, a
  contradicted item count, two misdated institutional recognitions.
- **Maracanã (Estádio Jornalista Mário Filho)** — **a reversed match result** (narration says Rio won
  3-1; the source says São Paulo won), a fabricated attendance record that contradicts the source's own
  stated record-holder match, a fabricated turf composition, invented nicknames.

#### Moderate — a specific invented or contradicted fact in an otherwise accurate narration

- **Biblioteca Nacional do Brasil** — "near nine million pieces" vs. source's "about ten million."
- **Casa das Canoas** — a fabricated direct quotation attributed to Niemeyer; an unsupported claim about
  the structural engineer's role.
- **Fazenda Imperial de Santa Cruz** — wrong founder and date (narration: Jesuits, 1570; source:
  Cristóvão Monteiro, 1556).
- **Instituto Histórico e Geográfico Brasileiro** — invented named members; an invented Jules Verne
  anecdote.
- **Lagoa Rodrigo de Freitas** — an invented, specific legal claim (swimming ban) not in the source.
- **Morro da Babilônia** — invented population/area statistics; invented named organizations/person.
- **Museu de Imagens do Inconsciente** — "350,000 works today" contradicts the source's growth narrative
  (source's current figure is ~400,000, larger than its own 2015 figure — the opposite of what the
  narration implies).
- **Museu Casa do Pontal** — an invented "this is considered the founding year" framing not in the source.
- **Praia do Leblon** — a fabricated attribution to "O Globo"; an invented residential-complex name.
- **Rua do Catete** — a fabricated Machado de Assis residency detail with an invented address; invented
  government-occupancy dates and decree numbers.
- **Universidade Federal do Rio de Janeiro** — invented 2023 enrollment and 2024 budget figures with no
  support in the source (which only gives a 2013 enrollment figure).
- **AquaRio** — a fabricated visitor anecdote: a named child, an invented direct quote, an invented
  "lifetime member" status.
- **Clube Naval** — an invented exact founding date (12 April); the source only gives the year.
- **Fortaleza de São José da Ilha das Cobras** — a misspelled historical name (João Câncio vs. the real
  João Cândido, leader of the Revolta da Chibata).
- **Estação Leopoldina** — one unsupported superlative ("rare" architectural style).
- **Igreja de Nossa Senhora da Pena** — a fabricated "heritage database" claim.
- **Igreja de Nossa Senhora do Carmo** — an unsupported (if historically plausible) causal claim about
  fleeing Napoleon, not stated in this source.

#### Internal contradictions (worth a second look regardless of source quality)

- **Igreja de Nossa Senhora da Lapa dos Mercadores** — the narration states the brotherhood was founded
  in 1747 in one place and ties its founding to 1750 in another.
- **Circo Voador** — states the venue's capacity as both 2,500 and 2,200 in the same narration (both
  numbers individually trace to the source, which is itself inconsistent — the narration reproduces the
  contradiction rather than resolving it).

### A specific, reproducible pattern worth flagging on its own

**National Museum of Brazil** and **Palácio de são cristóvão** — two different places — contain a
**near-identical fabricated passage** describing a technical cause for a fire (an ungrounded air
conditioner, a bare wire touching metal) that appears in neither source. Two independent judge agents
flagged this same passage independently without being told about each other's batch. This looks like a
specific, reproducible hallucination inserted during generation — likely worth a targeted search across
the rest of the corpus for the same boilerplate, not just these two instances.

## The 92 unverifiable places — explicitly NOT clean

### 91 with no matching source found (Wikidata search/distance match failed)

See `places_grounding_for_verification_v1.csv`'s `no_match` rows for the full list (91 places,
`grounding_status = no_match`). These range from well-known landmarks whose Wikidata item didn't
resolve cleanly to niche local businesses/venues that may simply not have a Wikipedia/Wikidata entry at
all.

### 1 with a matched source but no French narration row in the database

- **Cristo Redentor** — matched a real source but has zero rows in `scripts` for `language = 'fr'`,
  so there was nothing to judge.

## Recommendation

**Do not run `cmd/batchreview -dry-run=false` against the full corpus yet.** 28 real, confirmed issues
need a human decision each (rewrite, remove the offending sentence, or accept as-is), and 92 places have
no verification at all and must not be assumed clean by omission. A reasonable path: fix or pull the 9
"Severe" places first (they carry the most invented content, including one outright reversed historical
fact), triage the 19 "Moderate" ones, and decide case-by-case on the 92 unverifiable places — flag each
either for a dedicated grounding attempt (if the place genuinely matters) or for exclusion from this
launch.
