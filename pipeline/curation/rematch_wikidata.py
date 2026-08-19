"""Re-match places without a wikidata_qid against Wikidata's general search
(not just the narrow IPHAN-heritage query the pipeline uses), validating
each candidate by geographic proximity to avoid false positives (a common
name match at the wrong location).
"""
import csv
import re
import sys
import time
import unicodedata

import requests

SEARCH_URL = "https://www.wikidata.org/w/api.php"
USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"
MAX_DISTANCE_M = 200.0


def haversine_distance_m(lat1, lon1, lat2, lon2):
    from math import radians, sin, cos, sqrt, atan2
    r = 6371000.0
    phi1, phi2 = radians(lat1), radians(lat2)
    dphi = radians(lat2 - lat1)
    dlambda = radians(lon2 - lon1)
    a = sin(dphi / 2) ** 2 + cos(phi1) * cos(phi2) * sin(dlambda / 2) ** 2
    return 2 * r * atan2(sqrt(a), sqrt(1 - a))


def _get_with_retry(params: dict, max_retries: int = 5) -> dict:
    delay = 2.0
    for attempt in range(max_retries):
        resp = requests.get(SEARCH_URL, params=params, headers={"User-Agent": USER_AGENT}, timeout=15)
        if resp.status_code == 429:
            retry_after = float(resp.headers.get("Retry-After", delay))
            print(f"    [429, retry {attempt+1}/{max_retries} in {retry_after:.0f}s]", file=sys.stderr)
            time.sleep(retry_after)
            delay *= 2
            continue
        resp.raise_for_status()
        return resp.json()
    raise RuntimeError(f"Still rate-limited after {max_retries} retries")


def search_candidates(name: str, limit: int = 3) -> list[str]:
    data = _get_with_retry({
        "action": "wbsearchentities", "search": name, "language": "pt",
        "format": "json", "limit": limit, "type": "item",
    })
    return [r["id"] for r in data.get("search", [])]


def get_coords(qid: str) -> tuple[float, float] | None:
    data = _get_with_retry({"action": "wbgetclaims", "entity": qid, "property": "P625", "format": "json"})
    claims = data.get("claims", {}).get("P625")
    if not claims:
        return None
    value = claims[0]["mainsnak"]["datavalue"]["value"]
    return value["latitude"], value["longitude"]


class SearchFailed(Exception):
    pass


def search_variants(name: str) -> list[str]:
    """Try the name stripped of parenthetical qualifiers first (parentheses
    confuse Wikidata's search — e.g. "Museu de Arte do Rio (MAR)" fails to
    match while "Museu de Arte do Rio" succeeds), then the original name as
    a fallback in case the parenthetical was actually load-bearing."""
    stripped = re.sub(r"\s*\([^)]*\)", "", name).strip()
    variants = [stripped]
    if stripped != name:
        variants.append(name)
    return variants


def rematch_one(name: str, lat: float, lon: float) -> tuple[str, float] | None:
    """Returns (qid, distance_m) on a validated match, None on a genuine
    no-match (all search variants exhausted, nothing within threshold).
    Raises SearchFailed if a search request itself could not complete
    (rate limit exhausted, network error) — caller must NOT treat this as
    "no Wikidata item exists", only as "we could not check"."""
    seen_qids = set()
    for variant in search_variants(name):
        try:
            candidates = search_candidates(variant)
        except (requests.RequestException, RuntimeError) as exc:
            raise SearchFailed(str(exc)) from exc
        for qid in candidates:
            if qid in seen_qids:
                continue
            seen_qids.add(qid)
            time.sleep(0.5)
            try:
                coords = get_coords(qid)
            except (requests.RequestException, RuntimeError) as exc:
                raise SearchFailed(str(exc)) from exc
            if coords is None:
                continue
            dist = haversine_distance_m(lat, lon, coords[0], coords[1])
            if dist <= MAX_DISTANCE_M:
                return qid, dist
    return None


def main(input_path, output_path, limit=None):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    unmatched = [r for r in rows if not r["wikidata_qid"].strip()]
    if limit:
        unmatched = unmatched[:limit]

    print(f"Lieux sans QID à re-matcher : {len(unmatched)}")
    found = 0
    genuinely_not_found = 0
    failed = 0
    for i, r in enumerate(unmatched, 1):
        try:
            result = rematch_one(r["name"], float(r["lat"]), float(r["lon"]))
        except SearchFailed as exc:
            failed += 1
            print(f"  [{i}/{len(unmatched)}] ÉCHEC RECHERCHE (pas une vraie absence) {r['name']!r}: {exc}", file=sys.stderr)
            continue
        time.sleep(0.5)
        if result:
            qid, dist = result
            r["wikidata_qid"] = qid
            r["source"] = r["source"] + "+wikidata_rematch"
            found += 1
            print(f"  [{i}/{len(unmatched)}] MATCH {r['name']!r} -> {qid} ({dist:.0f}m)")
        else:
            genuinely_not_found += 1
            print(f"  [{i}/{len(unmatched)}] pas de correspondance réelle : {r['name']!r}")

    print()
    print(f"Trouvés: {found}/{len(unmatched)}")
    print(f"Vraiment pas de correspondance (recherche aboutie, rien à <200m): {genuinely_not_found}")
    print(f"Échecs de recherche (à réessayer, PAS une absence confirmée): {failed}")

    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=["name", "category", "source", "lat", "lon", "wikidata_qid", "id"])
        writer.writeheader()
        for r in sorted(rows, key=lambda x: (x["category"], x["name"])):
            writer.writerow(r)


if __name__ == "__main__":
    limit = int(sys.argv[3]) if len(sys.argv) > 3 else None
    main(sys.argv[1], sys.argv[2], limit)
