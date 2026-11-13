import json

import pytest

from judges.transcript import (
    Mismatch,
    Verdict,
    apply_corrections,
    build_prompt,
    find_mismatches,
    parse_verdicts,
)


class TestFindMismatches:
    def test_identical_text_has_no_mismatch(self):
        assert find_mismatches("Aqui fica a praça.", "Aqui fica a praça.") == []

    def test_ignores_case_accents_and_punctuation(self):
        script = "Está usted en la Plaza XV, en pleno centro."
        heard = "esta usted en la plaza xv en pleno centro"
        assert find_mismatches(script, heard) == []

    def test_roman_and_arabic_numerals_are_equivalent(self):
        assert find_mismatches("Praça XV de Novembro", "Praça 15 de Novembro") == []
        assert find_mismatches("Luís XIV reinou", "Luís 14 reinou") == []

    def test_catches_the_praca_xv_misheard_as_traca_15(self):
        script = "Aqui fica a Praça XV de Novembro no centro."
        heard = "Aqui fica a Traça 15 de Novembro no centro."
        [m] = find_mismatches(script, heard)
        assert m.kind == "replace"
        assert m.script_span == "Praça XV"
        assert m.transcript_span == "Traça 15"

    def test_catches_words_the_transcript_dropped(self):
        [m] = find_mismatches("um dois três quatro cinco", "um dois cinco")
        assert m.kind == "missing"
        assert m.script_span == "três quatro"
        assert m.transcript_span == ""

    def test_catches_words_the_transcript_added(self):
        [m] = find_mismatches("um dois cinco", "um dois três quatro cinco")
        assert m.kind == "extra"
        assert m.script_span == ""
        assert m.transcript_span == "três quatro"

    def test_context_is_taken_from_both_texts_around_the_span(self):
        script = "a b c d e f g PRACA h i j k l m"
        heard = "a b c d e f g TRACA h i j k l m"
        [m] = find_mismatches(script, heard, context_words=3)
        assert m.script_context == "e f g PRACA h i j"
        assert m.transcript_context == "e f g TRACA h i j"

    def test_context_is_clipped_at_the_ends(self):
        [m] = find_mismatches("PRACA a b", "TRACA a b", context_words=5)
        assert m.script_context == "PRACA a b"


class TestParseVerdicts:
    def test_parses_a_valid_response(self):
        body = json.dumps(
            [
                {"index": 0, "verdict": "error", "correction": "Praça XV", "reason": "misheard"},
                {"index": 1, "verdict": "benign", "correction": None, "reason": "number format"},
            ]
        )
        assert parse_verdicts(body, 2) == [
            Verdict(0, "error", "Praça XV", "misheard"),
            Verdict(1, "benign", None, "number format"),
        ]

    def test_accepts_json_wrapped_in_a_code_fence(self):
        body = '```json\n[{"index": 0, "verdict": "benign", "correction": null, "reason": "x"}]\n```'
        assert parse_verdicts(body, 1)[0].verdict == "benign"

    @pytest.mark.parametrize(
        "body",
        [
            "not json",
            '{"index": 0}',  # not a list
            '[{"index": 0, "verdict": "maybe", "correction": null, "reason": ""}]',  # unknown verdict
            '[{"index": 0, "verdict": "error", "correction": null, "reason": ""}]',  # error without a correction
        ],
    )
    def test_rejects_malformed_or_unusable_responses(self, body):
        with pytest.raises(ValueError):
            parse_verdicts(body, 1)

    def test_rejects_a_response_that_skips_a_mismatch(self):
        body = '[{"index": 0, "verdict": "benign", "correction": null, "reason": ""}]'
        with pytest.raises(ValueError):
            parse_verdicts(body, 2)


class TestBuildPrompt:
    def test_includes_every_mismatch_with_its_index_and_context(self):
        ms = [
            Mismatch("replace", "Praça XV", "Traça 15", "a Praça XV de", "a Traça 15 de"),
            Mismatch("missing", "três", "", "dois três cinco", "dois cinco"),
        ]
        prompt = build_prompt(ms, "pt")
        assert "[0]" in prompt and "[1]" in prompt
        assert "Praça XV" in prompt and "Traça 15" in prompt
        assert "pt" in prompt


class TestApplyCorrections:
    def _body(self, *texts):
        return {
            "duration": 10.0,
            "segments": [{"id": i, "start": float(i), "end": float(i + 1), "text": t} for i, t in enumerate(texts)],
        }

    def test_replaces_the_misheard_text_inside_a_segment(self):
        body = self._body(" Estamos na Traça 15 de Novembro.")
        fixed, unapplied = apply_corrections(body, [("Traça 15", "Praça XV")])
        assert fixed["segments"][0]["text"] == " Estamos na Praça XV de Novembro."
        assert unapplied == []

    def test_leaves_timing_untouched_and_does_not_mutate_the_input(self):
        body = self._body("Traça 15")
        fixed, _ = apply_corrections(body, [("Traça 15", "Praça XV")])
        assert fixed["segments"][0]["start"] == 0.0 and fixed["duration"] == 10.0
        assert body["segments"][0]["text"] == "Traça 15"

    def test_reports_a_correction_it_could_not_place(self):
        body = self._body("nada a ver")
        fixed, unapplied = apply_corrections(body, [("Traça 15", "Praça XV")])
        assert fixed["segments"][0]["text"] == "nada a ver"
        assert unapplied == [("Traça 15", "Praça XV")]
