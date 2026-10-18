# Severe findings — fixes applied (2026-10-17)

Corrects the 9 "Severe" findings from `anti_hallucination_verification_report_v1.md`. Each fix is
surgical: only the specific flagged claims were removed or corrected (using the real grounding
source where it had an equivalent fact), everything else in the narration — style, structure, and
every unflagged fact — was left untouched and independently re-verified against the full grounding
article before being kept.

**Applied directly to the `scripts` table (French rows only) in the local dev database** — not yet
re-verified by a fresh judge pass, and not yet migrated to any real production database, since none
exists yet pending the Terraform deployment in `backend/deploy/terraform/`.

## National Museum of Brazil
Removed two invented naturalist names (kept the four confirmed ones: Spix, Martius, Saint-Hilaire,
Langsdorff, plus Hartt), an invented per-object cost statistic, a fabricated World Bank loan episode,
a fabricated technical fire cause, and a fabricated 2019 excavation claim. Replaced an invented donor
list with the real reconstruction funding figures (R$85.4M available from 2019, incl. R$55M from the
2019 Union budget; R$50M more from the federal government in 2023–2024) and corrected the reopening
year to 2026.

## Palácio de são cristóvão
Corrected the merchant's description (removed "slave trader"), the renovation year (1819 → 1816), and
the damage percentage (92.5% → "more than 90%", matching the source). Removed invented room names/
technique for the painter Bragaldi, an ungrounded causal clause about Princess Isabel, invented 1891/
1892 founding dates for the Museu Nacional's tenancy, an invented Petrópolis throne-room detail, an
invented Pedro II donation list, and the same fabricated fire-cause passage as the National Museum
entry. Replaced the invented "survived meteorite + new roof" ending with the source's real fact: only
the façade survived.

## Panteão Duque de Caxias
Removed an entire invented section about a different building (a "Palácio Duque de Caxias" with
named architects and a 1941 inauguration) and an invented present-day military command structure.
Rebuilt the narration around what the source actually supports: the real August 1949 ceremonies
transferring the Duke and Duchess's remains, with their real participants, dates, and details.

## Planetário da Gávea
Removed an invented "second museum of its kind" ranking claim, an invented visitor-count figure, an
invented planetarium brand name, four entirely invented public programs, and invented observation
hours. Corrected the experiment count (56 → 60, matching the source).

## Sugarloaf Mountain (Pão de Açúcar)
Corrected the etymology (removed an invented historian citation and a Goa-import claim; replaced with
the source's real sugar-cane-cultivation origin). Removed an invented engineer name, company name, and
German-manufacturing claim around the cable car's 1908 conception. Removed an invented cabin-
replacement history and invented 2020 closure dates. **Correction to the correction**: the 2012 UNESCO
World Heritage mention, originally flagged as unsupported by the first judge pass, was independently
re-checked against the real source and found to be genuinely present ("tornou-se parte de um
Patrimônio Mundial declarado pela UNESCO em 2012") — restored rather than removed.

## Instituto de Pesquisa e Memória Pretos Novos
The most heavily corrected entry: removed a named vice-roy and his invented causal role, invented
burial-count estimates, an invented prior-burial-site claim, an invented 1830-closure rationale,
invented 2012 construction details, an entire invented 2017 excavation episode (including a named
skeleton), and invented archive/library statistics. The corrected narration is substantially shorter,
built only around what the source actually supports: the Institute's founding, the real 1996
discovery, and its library/gallery.

## Museu da República (Palácio do Catete)
Removed the entire unsupported final section: an invented 1960 founding decree and named director, an
invented 1983/1984/1989/2005 institutional timeline, and a long invented block of acervo statistics by
named sub-collection. The narration now ends naturally where the real source's coverage ends.

## Centro Cultural Cartola
Removed an invented rival museum, an invented Petrobras-sponsored orchestra, and an invented 2003
adaptation date. Corrected the item count (45,000 → "more than 50,000") and the 2009 institutional
recognition (replaced a misdated/misattributed claim with the real 2009 "Pontão de Cultura" status),
and corrected the Museu do Samba renaming to its real date (August 2015, not "since 2013").

## Maracanã (Estádio Jornalista Mário Filho)
**The single most important correction in this entire pass**: the 1950 Rio-São Paulo friendly's result
was stated backwards (narration said Rio won 3-1; the source says São Paulo won 3-1) — corrected.
Removed a fabricated "biggest attendance ever" claim for the 1950 final (kept the two real,
correctly-sourced attendance records: December 1963 and August 1969). Removed an invented hybrid-turf
claim and three invented nicknames (kept the one real nickname, "Maraca").
