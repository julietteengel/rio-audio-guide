from sourcing.dedup import normalize_name, haversine_distance_m, deduplicate_places
from sourcing.models import Place


def test_normalize_name_strips_accents_and_case():
    assert normalize_name("Escadaria Selarón") == "escadaria selaron"


def test_normalize_name_strips_generic_prefix():
    assert normalize_name("Museu da Chácara do Céu") == "chacara do ceu"


def test_normalize_name_collapses_whitespace():
    assert normalize_name("Igreja   de   Nossa Senhora") == "nossa senhora"


def test_haversine_distance_zero_for_same_point():
    assert haversine_distance_m(-22.9147, -43.1806, -22.9147, -43.1806) == 0.0


def test_haversine_distance_approx_one_km_for_known_delta():
    # 1 degré de latitude vaut environ 111 320 m ; 0.009 degré ~= 1002 m.
    dist = haversine_distance_m(0.0, -43.0, 0.009, -43.0)
    assert 950 < dist < 1050


def test_deduplicate_places_merges_close_same_name():
    places = [
        Place(name="Escadaria Selarón", lat=-22.91470, lon=-43.18060, category="landmark_and_historical_building", source="overture"),
        Place(name="escadaria selaron", lat=-22.91471, lon=-43.18061, category="artwork", source="wikidata"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 1
    assert result[0].source == "overture"


def test_deduplicate_places_keeps_distant_places_with_same_name():
    places = [
        Place(name="Igreja Matriz", lat=-22.9147, lon=-43.1806, category="church_cathedral", source="overture"),
        Place(name="Igreja Matriz", lat=-22.8000, lon=-43.3000, category="church_cathedral", source="overture"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 2


def test_deduplicate_places_merges_by_shared_wikidata_qid_even_if_name_differs():
    places = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture", wikidata_qid="Q1798512"),
        Place(name="National Museum of Brazil", lat=-22.9059, lon=-43.2247, category="museum", source="wikidata", wikidata_qid="Q1798512"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 1
