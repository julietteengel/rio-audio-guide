import json
from unittest.mock import patch

from sourcing.models import Place
from sourcing.pipeline import run_pipeline


def test_run_pipeline_combines_and_writes_output(tmp_path):
    overture_result = [
        Place(name="Escadaria Selarón", lat=-22.9147, lon=-43.1806, category="landmark_and_historical_building", source="overture")
    ]
    wikidata_result = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.1850, category="heritage_site", source="wikidata", wikidata_qid="Q1798512")
    ]
    feiras_places_result = [
        Place(name="Feira de Santa Teresa (Sexta-Feira)", lat=-22.9150, lon=-43.1900, category="recurring_cultural_event", source="feiras_registry")
    ]

    output_file = tmp_path / "places.json"

    with patch("sourcing.pipeline.query_overture_places", return_value=overture_result), \
         patch("sourcing.pipeline.query_iphan_heritage_sites", return_value=wikidata_result), \
         patch("sourcing.pipeline.fetch_and_parse_feiras_pdf", return_value=[{"descricao": "x", "bairro": "y", "dia_semana": "z"}]), \
         patch("sourcing.pipeline.feiras_to_places", return_value=feiras_places_result):
        result = run_pipeline(output_file)

    assert len(result) == 3
    saved = json.loads(output_file.read_text(encoding="utf-8"))
    assert len(saved) == 3
    names = {place["name"] for place in saved}
    assert names == {"Escadaria Selarón", "Museu Nacional", "Feira de Santa Teresa (Sexta-Feira)"}


def test_run_pipeline_deduplicates_across_sources(tmp_path):
    # NOTE: category aligned to "museum" on both sides (brief originally specified
    # "heritage_site" for the Wikidata entry). dedup.py's name+proximity merge path
    # requires matching categories by design (commit 28547c2, "never auto-merge on
    # strong ambiguity" per spec) and the Overture side here carries no wikidata_qid
    # (query_overture_places never sets one), so the QID-match path doesn't apply
    # either. A mismatched-category version of this test would correctly NOT dedupe
    # under the current, intentionally-hardened dedup.py — so it wouldn't exercise
    # what this test is meant to check (that run_pipeline wires deduplicate_places
    # in correctly across sources).
    overture_result = [Place(name="Museu Nacional", lat=-22.9058, lon=-43.1850, category="museum", source="overture")]
    wikidata_result = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.1850, category="museum", source="wikidata", wikidata_qid="Q1798512")
    ]

    output_file = tmp_path / "places.json"

    with patch("sourcing.pipeline.query_overture_places", return_value=overture_result), \
         patch("sourcing.pipeline.query_iphan_heritage_sites", return_value=wikidata_result), \
         patch("sourcing.pipeline.fetch_and_parse_feiras_pdf", return_value=[]), \
         patch("sourcing.pipeline.feiras_to_places", return_value=[]):
        result = run_pipeline(output_file)

    assert len(result) == 1


def test_run_pipeline_filters_out_of_bbox_places(tmp_path):
    # RIO_DE_JANEIRO_BBOX = (-43.7962520, -23.0827051, -43.0990811, -22.7460878).
    # "In Bbox" est dans Rio (Santa Teresa) ; "Far Away" est à São Paulo, bien
    # en dehors de la municipalité de Rio.
    overture_result = [Place(name="In Bbox", lat=-22.92, lon=-43.18, category="museum", source="overture")]
    wikidata_result = [Place(name="Far Away", lat=-23.5505, lon=-46.6333, category="heritage_site", source="wikidata")]

    output_file = tmp_path / "places.json"

    with patch("sourcing.pipeline.query_overture_places", return_value=overture_result), \
         patch("sourcing.pipeline.query_iphan_heritage_sites", return_value=wikidata_result), \
         patch("sourcing.pipeline.fetch_and_parse_feiras_pdf", return_value=[]), \
         patch("sourcing.pipeline.feiras_to_places", return_value=[]):
        result = run_pipeline(output_file)

    assert len(result) == 1
    assert result[0].name == "In Bbox"
