from unittest.mock import patch, Mock

import pytest

from sourcing.feiras import parse_feiras_table_rows, geocode_address, feiras_to_places


def test_parse_feiras_table_rows_extracts_known_columns():
    rows = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana", "RA"],
        ["112", "Não", "AV. AUGUSTO SEVERO...", "GLORIA", "Domingo", "04-Botafogo"],
    ]
    result = parse_feiras_table_rows(rows)
    assert result == [{"descricao": "AV. AUGUSTO SEVERO...", "bairro": "GLORIA", "dia_semana": "Domingo"}]


def test_parse_feiras_table_rows_skips_incomplete_rows():
    rows = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana"],
        ["112", "Não"],
    ]
    assert parse_feiras_table_rows(rows) == []


def test_parse_feiras_table_rows_raises_on_unexpected_header():
    rows = [["Foo", "Bar"]]
    with pytest.raises(ValueError, match="Unexpected feiras table header"):
        parse_feiras_table_rows(rows)


def test_geocode_address_returns_lat_lon_from_first_result():
    fake_response = Mock()
    fake_response.json.return_value = [{"lat": "-22.9147", "lon": "-43.1806"}]
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        result = geocode_address("Escadaria Selarón, Rio de Janeiro, Brazil")
    assert result == (-22.9147, -43.1806)


def test_geocode_address_returns_none_when_no_results():
    fake_response = Mock()
    fake_response.json.return_value = []
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        result = geocode_address("Lieu inexistant, Rio de Janeiro, Brazil")
    assert result is None


def test_feiras_to_places_builds_named_place_from_geocoded_feira():
    feiras = [{"descricao": "RUA TERESINA...", "bairro": "SANTA TERESA", "dia_semana": "Sexta-Feira"}]
    fake_response = Mock()
    fake_response.json.return_value = [{"lat": "-22.9350", "lon": "-43.1900"}]
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        places = feiras_to_places(feiras)
    assert len(places) == 1
    assert places[0].name == "Feira de Santa Teresa (Sexta-Feira)"
    assert places[0].category == "recurring_cultural_event"
    assert places[0].source == "feiras_registry"


def test_feiras_to_places_skips_feiras_that_fail_to_geocode():
    feiras = [{"descricao": "RUA X", "bairro": "BAIRRO INEXISTENTE", "dia_semana": "Segunda-Feira"}]
    fake_response = Mock()
    fake_response.json.return_value = []
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        places = feiras_to_places(feiras)
    assert places == []


def test_fetch_and_parse_feiras_pdf_logs_warning_and_continues_on_malformed_page(caplog, monkeypatch):
    import logging
    from unittest.mock import MagicMock
    import sourcing.feiras as feiras_module

    bad_table = [["Título institucional", "", ""]]
    good_table = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana"],
        ["54", "Não", "RUA TEREZINA", "SANTA TERESA", "Sexta-Feira"],
    ]

    fake_page_bad = MagicMock()
    fake_page_bad.extract_table.return_value = bad_table
    fake_page_good = MagicMock()
    fake_page_good.extract_table.return_value = good_table

    fake_pdf = MagicMock()
    fake_pdf.pages = [fake_page_bad, fake_page_good]
    fake_pdf.__enter__.return_value = fake_pdf
    fake_pdf.__exit__.return_value = False

    fake_response = MagicMock()
    fake_response.content = b"fake-pdf-bytes"
    fake_response.raise_for_status = MagicMock()

    monkeypatch.setattr(feiras_module.requests, "get", lambda *a, **kw: fake_response)
    monkeypatch.setattr(feiras_module.pdfplumber, "open", lambda *a, **kw: fake_pdf)

    with caplog.at_level(logging.WARNING):
        result = feiras_module.fetch_and_parse_feiras_pdf()

    assert len(result) == 1
    assert result[0]["bairro"] == "SANTA TERESA"
    assert any("Skipping page" in record.message for record in caplog.records)
