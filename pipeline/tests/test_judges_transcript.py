import json

import pytest

from judges.transcript import (
    Mismatch,
    Verdict,
    apply_fixes,
    build_prompt,
    find_mismatches,
    find_mismatches_in_body,
    judge,
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
        ],
    )
    def test_rejects_malformed_or_unusable_responses(self, body):
        with pytest.raises(ValueError):
            parse_verdicts(body, 1)

    @pytest.mark.parametrize("correction", ["null", '""'])
    def test_an_error_without_a_correction_is_allowed_for_words_the_transcript_added(self, correction):
        # nothing to write there: apply_fixes uses the script's words and only for "replace" differences
        body = '[{"index": 0, "verdict": "error", "correction": %s, "reason": "extra word"}]' % correction
        assert parse_verdicts(body, 1)[0].verdict == "error"

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


def _body(*texts):
    return {
        "text": " ".join(t.strip() for t in texts),
        "duration": 10.0,
        "segments": [{"id": i, "start": float(i), "end": float(i + 1), "text": t} for i, t in enumerate(texts)],
    }


def _error(m, correction):
    return (m, Verdict(0, "error", correction, ""))


class TestSegmentAwareAlignment:
    def test_locates_a_mismatch_by_transcript_word_position(self):
        body = _body(" Estamos na Traça 15 de Novembro.")
        [m] = find_mismatches_in_body("Estamos na Praça XV de Novembro.", body)
        assert (m.t_start, m.t_end) == (2, 4)


class TestApplyFixes:
    def test_replaces_only_the_misheard_words_at_their_position(self):
        body = _body(" Estamos na Traça 15 de Novembro.", " Depois seguimos.")
        [m] = find_mismatches_in_body("Estamos na Praça XV de Novembro. Depois seguimos.", body)
        fixed, applied, unapplied = apply_fixes(body, [_error(m, "Praça XV")])
        assert fixed["segments"][0]["text"] == " Estamos na Praça XV de Novembro."
        assert fixed["segments"][1]["text"] == " Depois seguimos."
        assert applied == 1 and unapplied == []

    def test_writes_the_script_s_own_words_whatever_the_model_proposed(self):
        body = _body(" Estamos na Traça 15 de Novembro.")
        [m] = find_mismatches_in_body("Estamos na Praça XV de Novembro.", body)
        fixed, applied, _ = apply_fixes(body, [_error(m, "texto inventado pelo modelo")])
        assert fixed["segments"][0]["text"] == " Estamos na Praça XV de Novembro." and applied == 1

    def test_does_not_touch_the_same_word_elsewhere(self):
        # "O" is a real word in the first segment; only the misheard one in the second is fixed.
        body = _body(" O Circo abre.", " Hoje O evento começa.")
        script = "O Circo abre. Hoje Eu evento começa."
        ms = find_mismatches_in_body(script, body)
        [m] = ms
        fixed, applied, _ = apply_fixes(body, [_error(m, "Eu")])
        assert fixed["segments"][0]["text"] == " O Circo abre."
        assert fixed["segments"][1]["text"] == " Hoje Eu evento começa."

    def test_keeps_surrounding_punctuation_and_spacing(self):
        body = _body(" (Traça 15), cidade.")
        [m] = find_mismatches_in_body("(Praça XV), cidade.", body)
        fixed, _, _ = apply_fixes(body, [_error(m, "Praça XV")])
        assert fixed["segments"][0]["text"] == " (Praça XV), cidade."

    def test_leaves_timing_untouched_and_does_not_mutate_the_input(self):
        body = _body("Traça 15")
        [m] = find_mismatches_in_body("Praça XV", body)
        fixed, _, _ = apply_fixes(body, [_error(m, "Praça XV")])
        assert fixed["segments"][0]["start"] == 0.0 and fixed["duration"] == 10.0
        assert body["segments"][0]["text"] == "Traça 15"

    def test_a_span_crossing_two_segments_is_reported_not_guessed(self):
        body = _body(" na Traça", " 15 de Novembro.")
        [m] = find_mismatches_in_body("na Praça XV de Novembro.", body)
        fixed, applied, unapplied = apply_fixes(body, [_error(m, "Praça XV")])
        assert applied == 0 and len(unapplied) == 1
        assert fixed["segments"][0]["text"] == " na Traça"

    def test_benign_and_missing_and_extra_are_never_applied(self):
        body = _body("um dois cinco")
        [m] = find_mismatches_in_body("um dois três quatro cinco", body)  # missing
        _, applied, unapplied = apply_fixes(body, [_error(m, "três quatro")])
        assert applied == 0 and len(unapplied) == 1
        body2 = _body("a Traça b")
        [m2] = find_mismatches_in_body("a Praça b", body2)
        _, applied2, _ = apply_fixes(body2, [(m2, Verdict(0, "benign", None, ""))])
        assert applied2 == 0

    def test_a_stale_position_is_refused_rather_than_corrupting_text(self):
        body = _body("a Traça b")
        [m] = find_mismatches_in_body("a Praça b", body)
        other = _body("zzz yyy xxx")
        fixed, applied, unapplied = apply_fixes(other, [_error(m, "Praça")])
        assert applied == 0 and len(unapplied) == 1
        assert fixed["segments"][0]["text"] == "zzz yyy xxx"

    def test_updates_the_full_text_when_it_is_the_joined_segments(self):
        body = _body(" Estamos na Traça 15.")
        [m] = find_mismatches_in_body("Estamos na Praça XV.", body)
        fixed, _, _ = apply_fixes(body, [_error(m, "Praça XV")])
        assert fixed["text"] == "Estamos na Praça XV."


class TestJudgeDedup:
    def test_identical_differences_are_sent_to_the_model_once_and_share_the_verdict(self):
        script = "Selarón a Selarón b Selarón"
        heard = "Celarón a Celarón b Celarón"
        seen = []

        def fake(prompt):
            seen.append(prompt.count("differing script words"))
            return json.dumps([{"index": 0, "verdict": "error", "correction": "Selarón", "reason": "name"}])

        out = judge(script, heard, "fr", fake)
        assert seen == [1]
        assert len(out) == 3 and all(v.verdict == "error" for _, v in out)
