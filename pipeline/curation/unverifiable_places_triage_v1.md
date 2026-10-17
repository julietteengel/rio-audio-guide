# Triage of the 92 originally-unverifiable places — 2026-10-17

Continues `anti_hallucination_verification_report_v1.md`, which left 92 places (91 with no
matched grounding source + Cristo Redentor, which has no FR narration at all) explicitly
**not verified** — not clean, just unchecked.

## Step 1 — free proximity + name-match reuse (28 places)

A naive ≤150m-distance pre-check against the 161 already-grounded places found 40 candidate
"duplicates." Requiring the candidate's own name tokens to actually appear in the proposed
neighbor's `grounding_text` (to reject false positives like "Theatro Municipal" near "Clube
Naval," or "Confeitaria Colombo" near a nearby church) left **28 confirmed reuses** (status
`ok_reused` in `places_grounding_for_verification_v1.csv`) and rejected 14 as genuinely
distinct places, sent to step 2 below. 1 place (Cristo Redentor) has a source but no FR
narration — excluded from this pipeline (narration problem, not a grounding problem).

That left **63 places** with no source at all.

## Step 2 — real web search on the 63 (this pass)

8 parallel agents, one per 8-place batch, searched the open web (not just Wikidata) for each
place, cross-checking coordinates against the real-world location before accepting a source.
Results, now merged into `places_grounding_for_verification_v1.csv` (`grounding_status`
column) with the source URL in `websearch_source` and caveats in `websearch_note`:

| Outcome | Count | `grounding_status` |
|---|---|---|
| Found — solid source | 54 | `ok_websearch` |
| Found — thin source (flag before narrating) | 4 | `ok_websearch_thin` |
| Likely duplicate / mislocated row | 3 | `duplicate_mislocated` |
| No credible source found | 2 | `no_match_confirmed` |
| **Total** | **63** | |

### Found — thin sources (usable, but narrate conservatively)

These clear the "real, checkable facts" bar but have noticeably less material than a full
Wikipedia article — don't let narration invent texture beyond what's here:

- **Centro Cultural Itaporã** — address/mission facts only (official site blocked automated
  fetch, snippets only).
- **Centro Cultural Sindicato dos Rodoviários do Município do Rio de Janeiro** — a municipal
  heritage-registry (IRPH) listing only: address + protection decree, no building history
  found anywhere.
- **Museu de Bangu** (real name: Grêmio Literário José Mauro de Vasconcelos) — official
  national museum registry entry, mostly logistical facts (opened 1996, address, phone);
  secondary tourism source adds founding date/namesake but is lower-confidence.
- **Teatro do Liceu** (Teatro do Liceu de Artes e Ofícios) — seat-count and inauguration-year
  figures conflict across the two sources found; real but imprecise.

### Likely duplicate / mislocated rows — do NOT auto-merge, human decision needed

Three "no_match" rows turned out, on investigation, to almost certainly be duplicate database
rows for landmarks **already grounded elsewhere** in the 161 `ok` set — not genuinely distinct
places. In all three cases the stored coordinates are significantly off from the real
landmark's location, and no alternative real institution was found near the stored
coordinates either:

- **Museu Nacional** (id `38f41131…`, coords -23.0198,-43.4648) — ~13km from the real Museu
  Nacional at Quinta da Boa Vista. No other "Museu Nacional" found nearby.
- **Pedra da Gávea - Rio de Janeiro** (id `5cde6392…`, coords -22.9064,-43.6093) — far from the
  real Pedra da Gávea monolith (-22.9965,-43.2819). No other named rock landmark found nearby.
- **Quinta da Boa Vista - Museu Nacional** (id `4e5278bc…`, coords -22.9264,-43.2158) — ~2.3–2.5km
  from the real museum/park. Same landmark as the row above, by name.

**Recommendation:** treat all three as duplicate rows of the single already-grounded "Museu
Nacional" / "Pedra da Gávea" entries. Before deleting or merging them in the database, a human
should sanity-check *why* three separate rows ended up with three different wrong coordinates
for the same two landmarks — this looks like a recurring geocoding failure mode in the
sourcing pipeline (per `dedup.py`'s own documented caution against false merges), not
one-off noise.

### A fourth likely duplicate, caught in this pass but not flagged by the search agents

- **Jardim e Morro do Valongo** (id `d04e0ffb…`) and **Mirante do Valongo** (id `2b976018…`)
  were searched independently (different batches) and both resolved to essentially the same
  physical site — the **Jardim Suspenso do Valongo** on Morro da Conceição. Both are currently
  marked `ok_websearch` with real sources (not forced to `duplicate_mislocated`, since each
  batch's search was credible on its own), but a human should check whether these are in fact
  one place double-counted in the database, same as the Museu Nacional / Pedra da Gávea cases
  above, before generating narration for both.

### No credible source found (2) — genuinely exclude from narration

- **Baianas da Estação Primeira de Mangueira** — extensive search found only coverage of the
  "Ala das Baianas" as a *parade wing/tradition* of the Estação Primeira de Mangueira samba
  school, not a visitable place with checkable facts. Looks like a POI mislabeled from an
  event/caption rather than a real landmark.
- **Comunidade do Vidigal** — a rich Wikipedia article exists for the famous Vidigal favela,
  but its real coordinates are ~13km from this row's coordinates (wrong side of the city,
  Centro/Glória/Catete area vs. the South Zone beachfront). Using the Vidigal article here
  would risk grounding narration in the wrong real-world place. No source was found for
  whatever this row's coordinates actually point to.

**Per the project's hard rule, these 2 get no narration.**

## Net result

| | Before this pass | After this pass |
|---|---|---|
| Grounded, ready for a judge pass | 161 (`ok`) + 28 (`ok_reused`) = 189 | +54 (`ok_websearch`) +4 (`ok_websearch_thin`) = **247** |
| Flagged duplicate, needs human merge decision | 0 | 3 confirmed (+2 suspected: the Valongo pair) |
| Confirmed no source — exclude from narration | 0 | 2 (+ Cristo Redentor, excluded for a different reason: no FR narration row) |
| Total places in DB | 252 | 252 |

**247 of 252 places now have a real grounding source on file.** None of the 58 newly-sourced
places (54 `ok_websearch` + 4 `ok_websearch_thin`) have ever been run through the
anti-hallucination judge — their FR narration must be judged against this newly-attached
`grounding_text` before being trusted, exactly like the original 161+28 were.

## Recommended next steps

1. Human review of the 3 (or 5, including the Valongo pair) likely-duplicate rows — decide
   whether to merge/delete them in the database before they reach narration.
2. Run a fresh judge pass (same methodology as the main verification report) on the FR
   narration for all 58 newly-sourced places.
3. Confirm the 2 no-source places get no narration going forward (and that Cristo Redentor's
   missing FR narration is addressed separately).
4. Fold this into the still-open "19 Moderate findings" triage and the planned EN/ES/PT
   translation-fidelity judge — both from the original verification report, still pending.
