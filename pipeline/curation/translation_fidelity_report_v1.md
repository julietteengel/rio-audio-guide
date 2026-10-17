# Translation-fidelity judge — EN/ES/PT vs FR — 2026-10-17

## Why this exists

The anti-hallucination pass (`anti_hallucination_verification_report_v1.md`) only checked FR
narration against each place's real grounding source. EN/ES/PT were never independently
checked — and per the project's own methodology (agreed this session), they shouldn't be
re-grounded against Wikipedia a second time either (that would double the "wrong source" risk).
Instead, EN/ES/PT are judged **against the already-validated FR narration as ground truth**:
any fact added, dropped, or altered in translation is a translation-fidelity bug, independent
of whether FR itself is accurate.

## Methodology

All 251 places with an FR script were exported (`export_translations.py`), split into 8
batches of ~32, and judged in parallel: for each place, EN vs FR, ES vs FR, PT vs FR
independently, flagging added/dropped/altered names, dates, numbers, and specific claims.
Style, phrasing, and idiomatic translation choices were explicitly not flagged.

## Headline finding: the 9 "Severe" FR fixes never propagated to translations

All 9 places whose FR narration was corrected in `severe_findings_fixes_v1.md` (the severe
anti-hallucination fixes applied earlier this session) **still have their old, pre-fix
hallucinated content in EN/ES/PT**, confirmed independently by the judge for every one:

| Place | What's still wrong in EN/ES/PT |
|---|---|
| National Museum of Brazil | Invented naturalists, World Bank loan episode, fire-cause fabrication, entire invented post-fire-recovery paragraph (real 85.4M-reais funding replaced by fabricated donations/digs) |
| Palácio de São Cristóvão | "Slave merchant" claim, wrong renovation year, invented room/painter detail, invented 1891-92 history, invented donation list, fire-cause fabrication, invented reopening |
| Panteão Duque de Caxias | Describes an almost entirely different subject (a statue + an adjacent palace) instead of the real 1949 remains-transfer ceremony — near-zero factual overlap with FR |
| Planetário da Gávea | Invented ranking claim, invented visitor count, invented brand name, wrong exhibit count, entire invented programming paragraph |
| Sugarloaf Mountain (Pão de Açúcar) | Fabricated historian/Goa-import etymology, invented engineer/company/German-manufacturing claim, invented cabin-replacement history and closure dates |
| Instituto de Pesquisa e Memória Pretos Novos | ~3x longer than FR; adds house numbers, burial-count statistics, an entire invented 2017 excavation with a named skeleton, invented archive stats |
| Museu da República (Palácio do Catete) | Two entire extra paragraphs appended after FR's real ending — invented decrees, dates, and detailed acervo statistics |
| Centro Cultural Cartola | Wrong adaptation date, wrong 2009 designation, wrong renaming date, invented disambiguation sentence, wrong item count, entire invented orchestra-program paragraph |
| Maracanã (Estádio Jornalista Mário Filho) | **Reversed match result** (says Rio won 3-1; real result is São Paulo 3-1), fabricated "Maracanaço" attendance paragraph, fabricated pitch composition, invented nicknames |

**Root cause (inferred, not yet independently confirmed):** EN/ES/PT for these 9 places were
almost certainly generated from the same pre-fix/hallucinated source material as the original
FR, and were never regenerated after FR was corrected. The identical wording across all three
target languages for each divergence (noted independently by multiple judge batches) supports
a shared upstream source rather than three independent per-language errors.

**Status:** corrected EN/ES/PT text for all 9 places is being generated now (faithful
translations of the corrected FR, mirroring exactly what `severe_findings_fixes_v1.md` already
did for FR) and will be applied to the `scripts` table the same way the FR fixes were applied.

## Other findings — minor translation drift (not part of the 9 severe places)

26 further places had at least one smaller divergence — a dropped qualifier, an added
descriptor, an altered number, or (in 5 cases) an identically-shared addition across all three
languages suggesting a shared upstream embellishment rather than independent translator error:

| Place | Issue |
|---|---|
| Aqueduto da Carioca | EN: height altered (17.6m → "17.5m") |
| Cidade do Samba RJ | EN/PT: "mecânica" (mechanics) workshop mistranslated as "tire repair"; PT also adds unsupported "ABNT standards" |
| Colégio Pedro II | EN/PT: generic "jurist" upgraded to an invented specific credential ("constitutional law scholar"/"professor") |
| Copacabana Palace | EN: adds an unstated actor ("the family hired..."); PT: "résidences" overstated as "mansões" |
| Cordão da Bola Preta | EN/ES/PT (shared): adds a "Cabrochas" character not in FR; reverses "searching for" to "reunion with"; "grand opening" rendered as "lavish send-off" (reversed sense) |
| D.R.I EAV Parque Lage | EN: "TV series" mistranslated as "a novel"; PT: adds "empresários," calls Snoop Dogg "o cantor" (wrong role) |
| Estação Ferroviária Leopoldina - Barão de Mauá | EN/ES/PT (shared): adds an ownership clause belonging to a different, sibling place's FR |
| Favela da Rocinha | EN/ES/PT (shared): adds an unstated title for Rogério Avelino |
| Fondation Moreira Sales | EN/ES/PT (shared): adds "for download," not supported by FR's "makes available" |
| Forte de Copacabana | PT: "boutique de souvenirs" mistranslated as "loja de conveniência" (convenience store) |
| Igreja de Nossa Senhora da Lapa dos Mercadores | EN/ES/PT (shared): adds "Second" to "Revolt of the Armada" (historically accurate, but not in FR) |
| Instituto De Pesquisas Jardim Botânico / Jardim Botânico do Rio de Janeiro | EN/ES/PT: adds "incapacitated" for Queen Maria I; replaces FR's Brazilian-biome list with "South and Central America"; PT additionally invents a source-garden name |
| Morro da Babilônia | EN/PT: adds a specific network name ("Rede Globo"/"TV Globo") not in FR |
| Museu Casa de Benjamin Constant | EN/ES/PT (shared): drops the specific "Article 8" reference |
| Museu Casa do Pontal | PT: drops explanatory glosses for "vaqueiros" and "cangaceiros" |
| Museu da Imagem e do Som (MIS) | EN: alters a Lacerda quote ("man" → "people"); PT: fabricates a precise figure ("304,845" vs FR's open "more than 304,000") |
| Museu de Arte Contemporânea de Niterói (MAC) | EN/ES/PT: each adds a different unsupported descriptor to "music workshops" |
| Museu do Amanhã | ES/PT: adds "renowned" for Frank Stella |
| Museu Histórico Nacional | PT: alters two area figures; swaps "Ministry of War" for "Army" |
| Museu Nacional | ES/PT: adds "postgraduate" to a generic "programs" claim |
| Praia de Copacabana | EN/ES/PT (shared): adds "third world tour" ordinal not in FR |
| Santa Casa de Misericórdia do Rio de Janeiro | EN/ES/PT: adds "public"; PT additionally drops "lay" |
| Sambódromo da Marquês de Sapucaí | PT: adds "CD e DVD" where FR just says "album" |
| Sambódromo Setor 10-11-12-13 | PT: same "CD e DVD" addition |
| Universidade Federal do Rio de Janeiro | EN: adds "top 100" to a generic "global ranking" claim |

## Gaps (not translation-fidelity issues — missing content)

The following places have a **null/missing** translation for at least one language — this is
a content gap, not a drift bug, and needs new narration generated, not a text correction:

- Centro Cultural Municipal Parque das Ruínas — PT missing
- Chácara do Céu — PT missing
- Circo Voador — PT missing
- Convento de Santa Teresa — PT missing
- Arcos Da Lapa - Vida Boemia Do Rio De Janeiro — PT missing
- Catedral Metropolitana de São Sebastião — PT missing
- Fundição Progresso — PT missing
- Igreja de Nossa Senhora do Carmo da Lapa do Desterro — PT missing
- Escadaria Selarón — PT missing (expected/intentional per the place's own data)
- Instituto Histórico e Geográfico Brasileiro — PT missing

## Net result

- **216 of 251 places (86%)** had fully clean EN/ES/PT translations relative to FR.
- **9 places** (the severe anti-hallucination fixes) need their EN/ES/PT regenerated to match
  the corrected FR — in progress.
- **26 places** have minor translation drift — targeted find/replace corrections, applied
  directly to the `scripts` table alongside the 9 severe fixes.
- **10 places** have a missing PT (or in one case the gap is expected) — flagged separately,
  needs new narration generation, out of scope for this fidelity-correction pass.

## Recommended next steps

1. Apply the 9 severe-place EN/ES/PT corrections (generated to match the already-fixed FR).
2. Apply the 26 minor-drift corrections.
3. Generate the 9 (or 10) missing PT narrations as a separate task.
4. Re-run this judge once more after both fix passes, scoped only to the corrected places, to
   confirm no new divergence was introduced.
