import pytest

from sourcing.overture import filter_by_category, query_overture_places, CATEGORY_ALLOWLIST


def test_filter_by_category_keeps_allowlisted_rows():
    rows = [
        {"name": "Cristo Redentor", "category": "monument"},
        {"name": "SeguidoresPremiun", "category": "business_advertising"},
        {"name": "Pilantras Bar", "category": "bar"},
    ]
    result = filter_by_category(rows)
    assert result == [{"name": "Cristo Redentor", "category": "monument"}]


def test_filter_by_category_excludes_noisy_categories_even_if_present():
    rows = [{"name": "Some Agency", "category": "business_advertising"}]
    assert filter_by_category(rows) == []


def test_category_allowlist_does_not_include_bars_or_restaurants():
    assert "bar" not in CATEGORY_ALLOWLIST
    assert "restaurant" not in CATEGORY_ALLOWLIST
    assert "business_advertising" not in CATEGORY_ALLOWLIST


@pytest.mark.integration
def test_query_overture_places_returns_known_landmark_for_selaron_bbox():
    # Bbox serré autour de l'Escadaria Selarón (frontière Lapa/Santa Teresa).
    # Test d'intégration réel (réseau requis). Les coordonnées ont été obtenues
    # en interrogeant directement le dataset Overture Maps places (release
    # 2026-06-17.0) pour les entrées dont le nom contient "Selar" : elles sont
    # regroupées entre lon [-43.179657, -43.178959] et lat [-22.916449,
    # -22.915533] (ex : "Selarón Apartments", "Scalinata Selarón", "Selaron
    # Steps"). Cette bbox reprend cette enveloppe avec une marge de +/-0.0005°
    # (~55 m) de chaque côté.
    bbox = (-43.180157, -22.916949, -43.178459, -22.915033)
    places = query_overture_places(bbox)
    names = [p.name for p in places]
    assert any("Selarón" in name or "Selaron" in name for name in names)
