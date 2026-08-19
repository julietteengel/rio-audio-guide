from unittest.mock import patch, Mock

from sourcing.wikidata import parse_wkt_point, query_iphan_heritage_sites


def test_parse_wkt_point_extracts_lat_lon():
    lat, lon = parse_wkt_point("Point(-43.2246 -22.9058)")
    assert lat == -22.9058
    assert lon == -43.2246


def test_query_iphan_heritage_sites_parses_sparql_response():
    fake_response = Mock()
    fake_response.json.return_value = {
        "results": {
            "bindings": [
                {
                    "item": {"value": "http://www.wikidata.org/entity/Q1798512"},
                    "itemLabel": {"value": "Museu Nacional"},
                    "coord": {"value": "Point(-43.2246 -22.9058)"},
                }
            ]
        }
    }
    fake_response.raise_for_status = Mock()
    with patch("sourcing.wikidata.requests.get", return_value=fake_response) as mock_get:
        places = query_iphan_heritage_sites()

    assert len(places) == 1
    assert places[0].name == "Museu Nacional"
    assert places[0].wikidata_qid == "Q1798512"
    assert places[0].lat == -22.9058
    assert places[0].lon == -43.2246
    assert places[0].source == "wikidata"
    mock_get.assert_called_once()
