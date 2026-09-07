# Content verification at scale — research findings (parked)

**Status:** research complete, deliberately parked — not scheduled. Revisit when reviewing hundreds of
narrations by hand becomes the actual bottleneck (see `docs/superpowers/specs/2026-08-06-product-prd.md`'s
risk table: the existing anti-hallucination judge already caused a ~50% false-positive rate in one pass
by comparing a narration against the wrong stored source extract).

## The problem

~1028 draft scripts (254 places × up to 4 languages) sit unreviewed. The founder is not a Rio history
expert and can't personally fact-check historical claims — she can confirm a citation matches its source
text, but not judge historical truth on her own. She asked what the actual best practice is for
validating LLM-generated factual content at scale without domain expertise, beyond "another LLM judge
checking the first judge."

## Findings, in priority order

1. **Verifiable exact-quote citations tied to a specific source document** (e.g. Anthropic's Citations
   API — `citations: {enabled: true}` on a document block returns response segments whose `cited_text`
   is *structurally guaranteed* to be a verbatim substring of the specific document passed in, identified
   by `document_index`). This turns verification into "does this exact string exist in the source
   extract?" — mechanical, not historical — and, because the citation is bound to the document actually
   used at generation time, structurally prevents the "judged against the wrong source extract" bug
   already seen in this project: there's no separate retrieval/matching step left for the judge to get
   wrong.
2. **Decompose each narration into atomic claims before verifying**, not whole-paragraph judging
   (Microsoft Research's "Claimify" pipeline: sentence-split → select → disambiguate → decompose; ~99%
   of extracted claims entailed by their source sentence in their evaluation). Catches "two facts
   correct, one fabricated" errors a holistic paragraph judgment misses. Same underlying idea as RAGAS's
   "faithfulness" metric.
3. **Don't over-invest in "a second LLM judge from a different model family."** Counterintuitive
   finding: a 2026 study testing a 9-judge panel across 7 model families found the panel worth only
   ~2 independent votes — judge errors are highly correlated across model families, and the single best
   judge matched or beat the full panel every time (independently reproduced by Apple ML Research too).
   A second judge is not free reliability; the citation-grounding fix (#1) is the higher-leverage spend.
4. **Fix the "wrong extract" bug with explicit provenance IDs, not better matching.** Standard fix in
   RAG failure taxonomies: the source extract must be passed *by reference* (a stored `source_id`)
   through generation and into the judge call, never re-fetched or re-matched by the judge via
   similarity search — that re-matching step is what silently substitutes the wrong extract. The
   Citations API enforces this by construction.
5. **Statistical sampling for the human layer**, MQM/TAUS-style (industry translation-QA practice for
   exactly this problem: non-expert reviewers, large corpora, need a defensible error-rate estimate).
   Stratify the corpus by place category × language, sample ~150-250 of the 1028 with a fixed rubric
   ("is there a citation? does it match the source verbatim? is the claim entailed by it?" — not "is
   this true"), compute a Wilson/Clopper-Pearson confidence interval on the corpus-wide error rate. Gives
   a real, defensible "X% of narrations have zero unsupported claims, ±Y% at 90% confidence" statement
   without reading all 1028 by hand.
6. **Her actual role shifts from "know Rio history" to "verify grounding"** — mechanical, not expert
   knowledge. Where extra assurance is wanted on a sample, cross-check the *grounding source itself*
   against one independent second source (Wikipedia vs. an official tourism/heritage site), still
   without requiring personal historical expertise.

## Gaps / honestly flagged limits

- No public case study was found of a solo founder/small team solving exactly this at this scale — the
  recommendation combines adjacent, well-evidenced practices (RAG citation grounding, translation-QA
  sampling, claim decomposition), not a single ready-made playbook.
- Sample-size math for this specific corpus (1028 scripts, ~254 places, 4 languages) wasn't computed —
  do that with a real Wilson-interval calculator once a target confidence/margin is picked.

## Sources

- Anthropic Citations API: https://claude.com/blog/introducing-citations-api ,
  https://platform.claude.com/docs/en/build-with-claude/citations
- Claimify (Microsoft Research): https://www.microsoft.com/en-us/research/blog/claimify-extracting-high-quality-claims-from-language-model-outputs/ ,
  paper: https://arxiv.org/abs/2502.10855
- "Nine Judges, Two Effective Votes": https://arxiv.org/abs/2605.29800
- RAG failure taxonomy / provenance: https://activewizards.com/blog/the-rag-failure-taxonomy-12-ways-production-retrieval-pipelines-break/ ,
  https://www.openlayer.com/blog/rag-pipeline-evaluation-groundedness-faithfulness
- MQM sampling (translation QA): https://themqm.org/resources/sampling/ , https://www.emergentmind.com/topics/multidimensional-quality-metrics-mqm
- Audit sampling: https://arxiv.org/abs/1802.03778

## Immediate, unrelated workaround already agreed

Rather than solving this now, the founder chose to manually review a small (~10 place) geographically
clustered subset through the existing one-at-a-time `POST /scripts/:id/review` flow, to get one real,
testable itinerary live — see the session this research came from for that decision.
