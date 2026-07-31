"""Grounding enrichment, v2: one Wikipedia API call per place instead of up
to ~10 Wikidata calls. Uses generator=geosearch combined with prop=extracts
to fetch nearby articles AND their intro text in a single request, anchored
on the place's own coordinates (which we already trust) rather than fuzzy
name matching against Wikidata.

Resumable: skips rows that already have grounding_status set.
"""
import csv
import math
import sys
import time

import requests

USER_AGENT = "rio-audio-guide-sourcing/1.0"
WP_API_PT = "https://pt.wikipedia.org/w/api.php"
WP_API_EN = "https://en.wikipedia.org/w/api.php"
GEOSEARCH_RADIUS_M = 150
MAX_ACCEPT_DISTANCE_M = 250  # reject matches further than this even if word-hint matched
CHECKPOINT_EVERY = 25


def haversine_distance_m(lat1, lon1, lat2, lon2):
    R = 6371000
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp = math.radians(lat2 - lat1)
    dl = math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * R * math.asin(math.sqrt(a))


def get_with_retry(url, params, max_retries=5):
    delay = 2.0
    for attempt in range(max_retries):
        resp = requests.get(url, params=params, headers={"User-Agent": USER_AGENT}, timeout=20)
        if resp.status_code == 429 or "too many requests" in resp.text.lower()[:200]:
            retry_after = resp.headers.get("Retry-After")
            wait = float(retry_after) if retry_after else delay
            print(f"    [throttled, retry {attempt+1}/{max_retries} in {wait:.0f}s]", file=sys.stderr)
            time.sleep(wait)
            delay *= 2
            continue
        resp.raise_for_status()
        return resp.json()
    raise RuntimeError(f"Still throttled after {max_retries} retries: {url}")


def geosearch_with_extract(lat, lon, name_hint, lang="pt"):
    """One call: find nearby Wikipedia articles + their intro extracts.
    Returns (title, extract, distance_m) for the best match, or None."""
    api = WP_API_PT if lang == "pt" else WP_API_EN
    data = get_with_retry(api, {
        "action": "query",
        "generator": "geosearch",
        "ggscoord": f"{lat}|{lon}",
        "ggsradius": GEOSEARCH_RADIUS_M,
        "ggslimit": 5,
        "prop": "extracts|coordinates",
        "exintro": 1,
        "explaintext": 1,
        "format": "json",
    })
    pages = data.get("query", {}).get("pages")
    if not pages:
        return None

    # NOTE: generator=geosearch does NOT populate a "dist" field on
    # prop=coordinates (that only exists with list=geosearch). Compute the
    # real distance ourselves from the returned lat/lon against our own
    # coordinates -- otherwise every candidate falls back to a fake default
    # and effectively disables proximity-based ranking.
    candidates = []
    for page in pages.values():
        title = page.get("title", "")
        extract = (page.get("extract") or "").strip()
        coords_list = page.get("coordinates") or [{}]
        c = coords_list[0]
        if "lat" in c and "lon" in c:
            dist = haversine_distance_m(lat, lon, c["lat"], c["lon"])
        else:
            dist = float("inf")
        candidates.append((title, extract, dist))

    candidates = [c for c in candidates if c[2] <= MAX_ACCEPT_DISTANCE_M]

    # Only accept a candidate whose title shares a real word with our name
    # hint. A nearby-but-unrelated building (a different museum, a church
    # next door) is not grounding for THIS place -- it's grounding for a
    # different one, and using it would misattribute facts. No accepted
    # candidate is a valid, honest outcome (grounding_status stays no_match),
    # not a bug to work around.
    STOPWORDS = {"museu", "museum", "casa", "centro", "instituto", "espaço",
                 "espaco", "cultural", "de", "da", "do", "dos", "das", "e"}
    hint_words = set(w.lower() for w in name_hint.split() if len(w) > 3) - STOPWORDS
    if not hint_words:
        return None
    for title, extract, dist in sorted(candidates, key=lambda c: c[2]):
        title_words = set(w.lower() for w in title.split()) - STOPWORDS
        if extract and hint_words & title_words:
            return title, extract, dist
    return None


def process_row(row):
    lat, lon, name = float(row["lat"]), float(row["lon"]), row["name"]
    try:
        result = geosearch_with_extract(lat, lon, name, lang="pt")
        if not result:
            result = geosearch_with_extract(lat, lon, name, lang="en")
    except (requests.RequestException, RuntimeError) as exc:
        row["grounding_status"] = f"error:{type(exc).__name__}"
        return

    if not result:
        row["grounding_status"] = "no_match"
        return

    title, extract, dist = result
    row["matched_wikipedia_title"] = title
    row["grounding_text"] = extract
    row["grounding_distance_m"] = f"{dist:.0f}"
    row["grounding_status"] = "ok"


def main(input_path, output_path, limit=None):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    for r in rows:
        for col in ("matched_wikipedia_title", "grounding_text", "grounding_distance_m", "grounding_status"):
            r.setdefault(col, "")

    fieldnames = list(rows[0].keys())
    targets = [r for r in rows if not r["grounding_status"]]
    if limit:
        targets = targets[:limit]

    print(f"À traiter: {len(targets)} (sur {len(rows)} au total)")

    for i, row in enumerate(targets, 1):
        process_row(row)
        time.sleep(0.5)
        print(f"  [{i}/{len(targets)}] {row['name']!r} -> {row['grounding_status']}"
              + (f" ({row['matched_wikipedia_title']}, {row['grounding_distance_m']}m)" if row["grounding_status"] == "ok" else ""))
        if i % CHECKPOINT_EVERY == 0:
            with open(output_path, "w", newline="", encoding="utf-8") as f:
                writer = csv.DictWriter(f, fieldnames=fieldnames)
                writer.writeheader()
                writer.writerows(rows)
            print(f"    [checkpoint {i}/{len(targets)}]")

    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)

    from collections import Counter
    print()
    print("Répartition finale :")
    for status, n in Counter(r["grounding_status"] for r in rows if r["grounding_status"]).most_common():
        print(f"  {status}: {n}")


if __name__ == "__main__":
    limit = int(sys.argv[3]) if len(sys.argv) > 3 else None
    main(sys.argv[1], sys.argv[2], limit)
