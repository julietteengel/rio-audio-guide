import requests

from sourcing.models import Place

SPARQL_ENDPOINT = "https://query.wikidata.org/sparql"
RIO_DE_JANEIRO_QID = "Q8678"
USER_AGENT = "rio-audio-guide-sourcing/1.0"

IPHAN_HERITAGE_QUERY = f"""
SELECT ?item ?itemLabel ?coord WHERE {{
  ?item wdt:P1435 ?heritage .
  ?item wdt:P625 ?coord .
  ?item wdt:P131* wd:{RIO_DE_JANEIRO_QID} .
  ?heritage rdfs:label ?hlabel .
  FILTER(LANG(?hlabel) = "pt" && CONTAINS(?hlabel, "IPHAN"))
  SERVICE wikibase:label {{ bd:serviceParam wikibase:language "pt,en". }}
}}
"""


def parse_wkt_point(wkt: str) -> tuple[float, float]:
    """Parse 'Point(lon lat)' (format WKT de Wikidata) en (lat, lon)."""
    inner = wkt.strip().removeprefix("Point(").removesuffix(")")
    lon_str, lat_str = inner.split(" ")
    return float(lat_str), float(lon_str)


def query_iphan_heritage_sites() -> list[Place]:
    response = requests.get(
        SPARQL_ENDPOINT,
        params={"query": IPHAN_HERITAGE_QUERY},
        headers={"Accept": "application/sparql-results+json", "User-Agent": USER_AGENT},
        timeout=30,
    )
    response.raise_for_status()
    rows = response.json()["results"]["bindings"]
    places = []
    for row in rows:
        qid = row["item"]["value"].rsplit("/", 1)[-1]
        name = row["itemLabel"]["value"]
        lat, lon = parse_wkt_point(row["coord"]["value"])
        places.append(
            Place(name=name, lat=lat, lon=lon, category="heritage_site", source="wikidata", wikidata_qid=qid)
        )
    return places
