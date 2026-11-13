import json

import pytest

from judges.translation import Issue, build_prompt, judge_translation, parse_issues, precheck

FR = "Le jardin a été fondé en 1808. Il compte 8000 espèces. Il est situé à Rio."
EN = "The garden was founded in 1808. It holds 8000 species. It is in Rio."


def issue(kind, tr=None, fr=None, reason=""):
    return {"type": kind, "translation_quote": tr, "fr_quote": fr, "reason": reason}


class TestPrecheck:
    def test_matching_numbers_and_length_are_not_suspicious(self):
        p = precheck(FR, EN)
        assert p.numbers_added == [] and p.numbers_dropped == [] and not p.suspicious

    def test_a_number_only_in_the_translation_is_added(self):
        p = precheck(FR, EN + " It opened to 300000 visitors.")
        assert p.numbers_added == ["300000"] and p.suspicious

    def test_a_number_only_in_the_french_is_dropped(self):
        p = precheck(FR, "The garden was founded in 1808. It is in Rio.")
        assert p.numbers_dropped == ["8000"] and p.suspicious

    def test_number_formatting_is_not_a_difference(self):
        assert precheck("Il y a 85,4 millions.", "There are 85.4 million.").numbers_added == []
        assert precheck("Il y a 8 000 espèces.", "There are 8,000 species.").numbers_dropped == []

    def test_a_translation_far_longer_than_the_french_is_suspicious(self):
        p = precheck(FR, EN * 4)
        assert p.length_ratio > 3 and p.suspicious

    def test_a_translation_far_shorter_than_the_french_is_suspicious(self):
        assert precheck(FR * 4, "The garden.").suspicious


class TestParseIssues:
    def test_a_dropped_issue_quotes_the_french(self):
        [i] = parse_issues(json.dumps([issue("dropped", None, "Il compte 8000 espèces.", "r")]), FR, EN)
        assert i.kind == "dropped" and i.quote_verified

    def test_an_added_issue_quotes_the_translation(self):
        [i] = parse_issues(json.dumps([issue("added", "It holds 8000 species.", None, "r")]), FR, EN)
        assert i.kind == "added" and i.quote_verified

    def test_an_altered_issue_needs_both_quotes(self):
        ok = issue("altered", "founded in 1808", "fondé en 1808", "r")
        [i] = parse_issues(json.dumps([ok]), FR, EN)
        assert i.quote_verified

    def test_a_quote_that_is_not_in_the_text_is_kept_but_marked_unverified(self):
        [i] = parse_issues(json.dumps([issue("added", "It has a zoo.", None, "r")]), FR, EN)
        assert not i.quote_verified

    def test_no_issues_is_a_valid_answer(self):
        assert parse_issues("[]", FR, EN) == []

    @pytest.mark.parametrize(
        "body",
        ["nope", '{"a": 1}', json.dumps([issue("weird")]), json.dumps([issue("added", None, None)])],
    )
    def test_malformed_replies_raise(self, body):
        with pytest.raises(ValueError):
            parse_issues(body, FR, EN)


class TestJudgeTranslation:
    def test_identical_meaning_makes_no_model_call_when_the_precheck_is_clean_and_skip_is_on(self):
        def boom(prompt):
            raise AssertionError("must not call the model")

        issues, p = judge_translation(FR, EN, "en", boom, skip_clean=True)
        assert issues == [] and not p.suspicious

    def test_runs_the_model_by_default(self):
        issues, _ = judge_translation(FR, EN, "en", lambda prompt: "[]")
        assert issues == []

    def test_prompt_names_the_language_and_includes_both_texts(self):
        prompt = build_prompt(FR, EN, "en")
        assert FR in prompt and EN in prompt and "en" in prompt
