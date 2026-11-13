import json

import pytest

from judges.grounding import ClaimVerdict, build_prompt, judge_grounding, parse_claims, summarize

SCRIPT = "O jardim foi fundado em 1808. Ele tem 8.000 espécies. Fica no Rio."
SOURCE = "O Jardim Botânico foi fundado em 1808 por D. João. Fica na Zona Sul do Rio de Janeiro."


def reply(*items):
    return json.dumps(list(items))


def claim(text, status, quote=None, reason=""):
    return {"claim": text, "status": status, "quote": quote, "reason": reason}


class TestParseClaims:
    def test_a_supported_claim_with_a_real_citation_is_verified(self):
        body = reply(claim("O jardim foi fundado em 1808.", "supported", "foi fundado em 1808"))
        [c] = parse_claims(body, SCRIPT, SOURCE)
        assert c.status == "supported" and c.citation_verified

    def test_a_supported_claim_whose_citation_is_not_in_the_source_is_downgraded(self):
        body = reply(claim("Ele tem 8.000 espécies.", "supported", "possui 8.000 espécies"))
        [c] = parse_claims(body, SCRIPT, SOURCE)
        assert c.status == "unsupported" and not c.citation_verified
        assert "citation" in c.reason.lower()

    def test_a_supported_claim_without_any_citation_is_downgraded(self):
        [c] = parse_claims(reply(claim("Fica no Rio.", "supported", None)), SCRIPT, SOURCE)
        assert c.status == "unsupported"

    def test_unsupported_and_contradicted_are_kept_as_they_are(self):
        body = reply(
            claim("Ele tem 8.000 espécies.", "unsupported"),
            claim("O jardim foi fundado em 1808.", "contradicted", "foi fundado em 1808", "x"),
        )
        a, b = parse_claims(body, SCRIPT, SOURCE)
        assert a.status == "unsupported" and b.status == "contradicted"

    def test_a_paraphrased_claim_is_kept_but_marked_as_not_located_in_the_narration(self):
        [c] = parse_claims(reply(claim("Texto que não está no script.", "unsupported")), SCRIPT, SOURCE)
        assert c.status == "unsupported" and not c.located

    def test_a_claim_copied_from_the_narration_is_located(self):
        [c] = parse_claims(reply(claim("Fica no Rio.", "unsupported")), SCRIPT, SOURCE)
        assert c.located

    @pytest.mark.parametrize("body", ["not json", '{"a": 1}', reply({"claim": "Fica no Rio.", "status": "maybe"})])
    def test_malformed_replies_raise(self, body):
        with pytest.raises(ValueError):
            parse_claims(body, SCRIPT, SOURCE)


class TestSummarize:
    def _c(self, status):
        return ClaimVerdict("x", status, None, "", status == "supported")

    def test_clean_only_when_every_claim_is_supported_and_coverage_is_high(self):
        s = summarize([self._c("supported")] * 3, coverage_ratio=0.9)
        assert s["clean"] and s["supported"] == 3

    def test_one_unsupported_claim_makes_it_not_clean(self):
        assert not summarize([self._c("supported"), self._c("unsupported")], 0.9)["clean"]

    def test_low_coverage_is_never_clean_because_the_judge_skipped_content(self):
        s = summarize([self._c("supported")], coverage_ratio=0.3)
        assert not s["clean"] and s["incomplete"]


class TestBuildPrompt:
    def test_contains_both_texts(self):
        p = build_prompt(SCRIPT, SOURCE, "pt")
        assert SCRIPT in p and SOURCE in p


class TestJudgeGrounding:
    def test_end_to_end_with_a_fake_model(self):
        body = reply(
            claim("O jardim foi fundado em 1808.", "supported", "foi fundado em 1808"),
            claim("Ele tem 8.000 espécies.", "unsupported"),
            claim("Fica no Rio.", "supported", "Fica na Zona Sul do Rio de Janeiro"),
        )
        claims, summary = judge_grounding(SCRIPT, SOURCE, "pt", lambda prompt: body)
        assert [c.status for c in claims] == ["supported", "unsupported", "supported"]
        assert summary["unsupported"] == 1 and not summary["clean"]


class TestChunking:
    LONG = " ".join(f"Frase numero {i} fala do jardim." for i in range(40))

    def test_a_long_narration_is_judged_in_blocks_each_against_the_full_source(self):
        prompts = []

        def fake(prompt):
            prompts.append(prompt)
            # every sentence of this block, as supported claims quoting the source
            block = prompt.split("NARRATION:\n")[1].split("\n\nSOURCE:")[0]
            return reply(*[claim(s.strip() + ".", "supported", "foi fundado em 1808") for s in block.split(".") if s.strip()])

        claims, summary = judge_grounding(self.LONG, SOURCE, "pt", fake, chunk_chars=300)
        assert len(prompts) > 1
        assert all(SOURCE in p for p in prompts)
        assert len(claims) == 40 and summary["supported"] == 40 and summary["clean"]

    def test_a_short_narration_is_one_call(self):
        calls = []
        judge_grounding(SCRIPT, SOURCE, "pt", lambda p: calls.append(1) or reply(claim("Fica no Rio.", "unsupported")))
        assert len(calls) == 1


class TestRetryAndCoverage:
    def test_a_malformed_block_is_retried_once(self):
        replies = iter(["not json", reply(claim("Fica no Rio.", "unsupported"))])
        claims, _ = judge_grounding(SCRIPT, SOURCE, "pt", lambda p: next(replies))
        assert len(claims) == 1

    def test_two_malformed_replies_in_a_row_raise(self):
        with pytest.raises(ValueError):
            judge_grounding(SCRIPT, SOURCE, "pt", lambda p: "not json")

    def test_paraphrased_claims_do_not_count_toward_coverage(self):
        body = reply(claim("Texto parafraseado.", "supported", "foi fundado em 1808"))
        _, summary = judge_grounding(SCRIPT, SOURCE, "pt", lambda p: body)
        assert summary["coverage"] == 0.0 and summary["incomplete"]
