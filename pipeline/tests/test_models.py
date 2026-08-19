from sourcing.models import Place


def test_place_generates_stable_id_from_source_name_and_coords():
    place = Place(
        name="Escadaria Selarón",
        lat=-22.9147,
        lon=-43.1806,
        category="landmark_and_historical_building",
        source="overture",
    )
    assert place.id == "overture:Escadaria Selarón:-22.91470:-43.18060"


def test_place_accepts_explicit_id():
    place = Place(name="Test", lat=0.0, lon=0.0, category="museum", source="wikidata", id="custom-id")
    assert place.id == "custom-id"


def test_place_wikidata_qid_defaults_to_none():
    place = Place(name="Test", lat=0.0, lon=0.0, category="museum", source="overture")
    assert place.wikidata_qid is None
