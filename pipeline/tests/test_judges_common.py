from judges.common import coverage, quote_in, sentence_coverage


class TestQuoteIn:
    def test_finds_an_exact_passage(self):
        assert quote_in("fundado em 1808", "O jardim foi fundado em 1808 por D. João.")

    def test_ignores_case_accents_whitespace_and_curly_quotes(self):
        assert quote_in("O  JARDIM foi fundado", "o jardim\nfoi fundado em 1808")
        assert quote_in("Palacio de Sao Cristovao", "o Palácio de São Cristóvão abriga")
        assert quote_in("l’Escadaria", "C'est l'Escadaria Selarón")

    def test_rejects_a_passage_that_is_not_there(self):
        assert not quote_in("fundado em 1810", "O jardim foi fundado em 1808.")

    def test_rejects_an_empty_quote(self):
        assert not quote_in("", "texto")
        assert not quote_in("   ", "texto")


class TestCoverage:
    def test_is_the_share_of_the_text_the_quotes_cover(self):
        text = "um dois tres quatro cinco seis sete oito nove dez"
        assert coverage(["um dois tres quatro cinco"], text) == 0.5

    def test_overlapping_quotes_are_not_counted_twice(self):
        text = "um dois tres quatro cinco seis sete oito nove dez"
        assert coverage(["um dois tres quatro", "tres quatro cinco"], text) == 0.5

    def test_quotes_not_found_cover_nothing(self):
        assert coverage(["inventado"], "um dois tres") == 0.0

    def test_empty_text_is_fully_covered(self):
        assert coverage([], "") == 1.0


class TestSentenceCoverage:
    TEXT = "O jardim abriu em 1808. Tem muitas palmeiras. Fica no Rio. É lindo."

    def test_is_the_share_of_sentences_holding_at_least_one_quote(self):
        # a short fragment of two of the four sentences is enough to count each one
        assert sentence_coverage(["abriu em 1808", "no Rio"], self.TEXT) == 0.5

    def test_several_quotes_in_one_sentence_count_it_once(self):
        assert sentence_coverage(["o jardim", "1808"], self.TEXT) == 0.25

    def test_nothing_found_covers_nothing(self):
        assert sentence_coverage(["inventado"], self.TEXT) == 0.0

    def test_empty_text_is_fully_covered(self):
        assert sentence_coverage([], "") == 1.0
