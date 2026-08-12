"""Grounding, bulk approach: one SPARQL query (wikibase:around, 30km radius
centered on Rio) fetches every Wikidata item near the city with a Wikipedia
article in pt/en/fr/es, in a single request -- instead of one Wikipedia API
call per place (which gets throttled into Wikimedia's anti-scraping tier
even with a compliant User-Agent, once request volume climbs). A local
spatial join (haversine <=100m) against our places CSV does the matching.

Proximity alone isn't enough validation (a random <100m item could be an
unrelated street/neighborhood entity) -- candidates also need a real word
overlap between the place name and the Wikidata item label, same principle
used in enrich_grounding_geosearch.py.

Writes curation/wikidata_matches.json: place id -> {qid, label, lang,
title, dist_m}. Not resumable (single bulk query, re-run is cheap).
"""
import json
import sys
import unicodedata
import urllib.parse

import requests

SPARQL_ENDPOINT = "https://query.wikidata.org/sparql"
USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"
RIO_CENTER_LON, RIO_CENTER_LAT = -43.1729, -22.9068
RADIUS_KM = "30"
MAX_DISTANCE_M = 100.0

QUERY = f"""
SELECT ?item ?itemLabel ?coord ?artPT ?artEN ?artFR ?artES WHERE {{
  SERVICE wikibase:around {{
    ?item wdt:P625 ?coord .
    bd:serviceParam wikibase:center "Point({RIO_CENTER_LON} {RIO_CENTER_LAT})"^^geo:wktLiteral .
    bd:serviceParam wikibase:radius "{RADIUS_KM}" .
  }}
  OPTIONAL {{ ?artPT schema:about ?item ; schema:isPartOf <https://pt.wikipedia.org/> . }}
  OPTIONAL {{ ?artEN schema:about ?item ; schema:isPartOf <https://en.wikipedia.org/> . }}
  OPTIONAL {{ ?artFR schema:about ?item ; schema:isPartOf <https://fr.wikipedia.org/> . }}
  OPTIONAL {{ ?artES schema:about ?item ; schema:isPartOf <https://es.wikipedia.org/> . }}
  FILTER(BOUND(?artPT) || BOUND(?artEN) || BOUND(?artFR) || BOUND(?artES))
  SERVICE wikibase:label {{ bd:serviceParam wikibase:language "pt,en,fr,es". }}
}}
"""

STOPWORDS = {"museu", "museum", "casa", "centro", "instituto", "espaco",
             "cultural", "de", "da", "do", "dos", "das", "e", "praca", "parque",
             "janeiro", "rio", "nacional", "catete"}

NON_MAINSPACE_PREFIXES = {
    "wikipedia", "wikipédia", "wikipédia discussão", "wikipedia talk",
    "portal", "portal discussão", "portal talk",
    "categoria", "category", "categoria discussão", "category talk",
    "ficheiro", "file", "arquivo",
    "predefinição", "template", "modelo",
    "ajuda", "help", "wikcionário", "wiktionary",
    "usuário", "usuária", "user", "utilisateur",
    "anexo", "anexo discussão",
}


def haversine_m(lat1, lon1, lat2, lon2):
    from math import radians, sin, cos, sqrt, atan2
    r = 6371000.0
    p1, p2 = radians(lat1), radians(lat2)
    dp, dl = radians(lat2 - lat1), radians(lon2 - lon1)
    a = sin(dp / 2) ** 2 + cos(p1) * cos(p2) * sin(dl / 2) ** 2
    return 2 * r * atan2(sqrt(a), sqrt(1 - a))


def normalize_words(text):
    lowered = text.strip().lower()
    no_accents = "".join(c for c in unicodedata.normalize("NFKD", lowered) if not unicodedata.combining(c))
    return {w for w in no_accents.split() if len(w) > 3} - STOPWORDS


def parse_wkt_point(wkt):
    inner = wkt.strip().removeprefix("Point(").removesuffix(")")
    lon_str, lat_str = inner.split(" ")
    return float(lat_str), float(lon_str)


def fetch_wikidata_items():
    resp = requests.get(
        SPARQL_ENDPOINT,
        params={"query": QUERY},
        headers={"Accept": "application/sparql-results+json", "User-Agent": USER_AGENT},
        timeout=90,
    )
    resp.raise_for_status()
    bindings = resp.json()["results"]["bindings"]

    items = []
    for b in bindings:
        qid = b["item"]["value"].rsplit("/", 1)[-1]
        label = b.get("itemLabel", {}).get("value", "")
        lat, lon = parse_wkt_point(b["coord"]["value"])
        articles = {}
        for lang, key in [("pt", "artPT"), ("en", "artEN"), ("fr", "artFR"), ("es", "artES")]:
            if key in b:
                url = b[key]["value"]
                # Split only on the "/wiki/" marker, not on the last "/" --
                # titles can contain their own slashes (e.g. project pages
                # like "Wikipédia:GLAM/Instituto Pretos Novos"), and rsplit
                # on "/" alone truncates those to a nonexistent page title.
                title = urllib.parse.unquote(url.split("/wiki/", 1)[-1].replace("_", " "))
                # Reject non-mainspace pages: project/portal/category pages
                # aren't real encyclopedia content and can't ground a
                # narration (mainspace titles can legitimately contain a
                # colon, e.g. film titles, so check against known namespace
                # prefixes rather than "any colon").
                prefix = title.split(":", 1)[0].lower()
                if prefix in NON_MAINSPACE_PREFIXES:
                    continue
                articles[lang] = title
        if not articles:
            continue  # every sitelink was non-mainspace -- no real article left to ground on
        items.append({"qid": qid, "label": label, "lat": lat, "lon": lon, "articles": articles})
    return items


def nearby_candidates(place_lat, place_lon, wd_items):
    candidates = []
    for item in wd_items:
        dist = haversine_m(place_lat, place_lon, item["lat"], item["lon"])
        if dist <= MAX_DISTANCE_M:
            candidates.append((dist, item))
    return sorted(candidates, key=lambda c: c[0])


def best_match(place_name, candidates):
    hint_words = normalize_words(place_name)
    if not hint_words:
        return None
    for dist, item in candidates:
        label_words = normalize_words(item["label"])
        if hint_words & label_words:
            return dist, item
    return None


def main(places_path, output_path):
    import csv
    with open(places_path, encoding="utf-8") as f:
        places = list(csv.DictReader(f))

    print(f"Fetching Wikidata items within {RADIUS_KM}km of Rio center...")
    wd_items = fetch_wikidata_items()
    print(f"Wikidata items with a pt/en/fr/es article: {len(wd_items)}")

    matches = {}
    within_100m = 0
    for place in places:
        candidates = nearby_candidates(float(place["lat"]), float(place["lon"]), wd_items)
        if candidates:
            within_100m += 1
        result = best_match(place["name"], candidates)
        if result:
            dist, item = result
            lang = next((lg for lg in ("pt", "en", "fr", "es") if lg in item["articles"]))
            matches[place["id"]] = {
                "qid": item["qid"],
                "wikidata_label": item["label"],
                "lang": lang,
                "title": item["articles"][lang],
                "dist_m": round(dist, 1),
            }

    print(f"Places with >=1 Wikidata item within {MAX_DISTANCE_M:.0f}m: {within_100m}")
    print(f"Places matched after name-overlap validation: {len(matches)}")

    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(matches, f, ensure_ascii=False, indent=2)
    print(f"Wrote {output_path}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
