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


def test_query_overture_places_returns_known_landmark_for_selaron_bbox():
    # Bbox serré autour de l'Escadaria Selarón (frontière Lapa/Santa Teresa).
    # Test d'intégration réel (réseau requis) : vérifié manuellement dans la
    # recherche de conception, cette zone renvoie de manière fiable ce lieu.
    # Adjusted slightly (+/- 0.001°) to match Overture Maps coordinates.
    bbox = (-43.1825, -22.9165, -43.1785, -22.9135)
    places = query_overture_places(bbox)
    names = [p.name for p in places]
    assert any("Selarón" in name or "Selaron" in name for name in names)
