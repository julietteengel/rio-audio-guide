"""Enrichment pass: for each place, fetch a grounding text (Wikipedia intro)
to later feed an LLM for narration generation. Two paths per row:
  1. Has wikidata_qid -> fetch its pt-wiki sitelink -> extract intro
  2. No qid -> search Wikidata by name, validate candidate by <=200m distance
     (same principle as sourcing/dedup.py's QID-match sanity check), then
     fetch its sitelink -> extract intro

Resumable: writes a checkpoint every CHECKPOINT_EVERY rows; re-running skips
rows that already have a grounding_status set.

Rate-limit handling reuses the retry-with-backoff approach already validated
earlier in this session (a naive fixed sleep() got 429'd by Wikidata).
"""
import csv
import math
import sys
import time

import requests

USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"
WD_API = "https://www.wikidata.org/w/api.php"
WP_API_PT = "https://pt.wikipedia.org/w/api.php"
WP_API_EN = "https://en.wikipedia.org/w/api.php"
MAX_DIST_M = 200.0
CHECKPOINT_EVERY = 25


def haversine_m(lat1, lon1, lat2, lon2):
    r = 6371000.0
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp, dl = math.radians(lat2 - lat1), math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * r * math.asin(math.sqrt(a))


def get_with_retry(url, params, max_retries=5):
    delay = 2.0
    for attempt in range(max_retries):
        resp = requests.get(url, params=params, headers={"User-Agent": USER_AGENT}, timeout=20)
        if resp.status_code == 429:
            retry_after = float(resp.headers.get("Retry-After", delay))
            print(f"    [429, retry {attempt+1}/{max_retries} in {retry_after:.0f}s]", file=sys.stderr)
            time.sleep(retry_after)
            delay *= 2
            continue
        resp.raise_for_status()
        return resp.json()
    raise RuntimeError(f"Rate-limited after {max_retries} retries: {url}")


def wd_entity_sitelink_and_coord(qid):
    data = get_with_retry(WD_API, {
        "action": "wbgetentities", "ids": qid, "props": "sitelinks|claims", "format": "json",
    })
    ent = data["entities"][qid]
    title = ent.get("sitelinks", {}).get("ptwiki", {}).get("title")
    coord = None
    try:
        v = ent["claims"]["P625"][0]["mainsnak"]["datavalue"]["value"]
        coord = (v["latitude"], v["longitude"])
    except (KeyError, IndexError):
        pass
    return title, coord


def wd_search(name):
    data = get_with_retry(WD_API, {
        "action": "wbsearchentities", "search": name, "language": "pt", "format": "json", "limit": 5, "type": "item",
    })
    return [c["id"] for c in data.get("search", [])]


def wp_extract(title, lang="pt"):
    api = WP_API_PT if lang == "pt" else WP_API_EN
    data = get_with_retry(api, {
        "action": "query", "prop": "extracts", "exintro": 1, "explaintext": 1,
        "redirects": 1, "titles": title, "format": "json",
    })
    page = next(iter(data["query"]["pages"].values()))
    return (page.get("extract") or "").strip()


def rematch_by_name(name, lat, lon):
    stripped = name.split(" (")[0].strip()
    variants = [stripped] if stripped == name else [stripped, name]
    seen = set()
    for variant in variants:
        for qid in wd_search(variant):
            if qid in seen:
                continue
            seen.add(qid)
            time.sleep(0.3)
            title, coord = wd_entity_sitelink_and_coord(qid)
            if coord and haversine_m(lat, lon, coord[0], coord[1]) <= MAX_DIST_M:
                return qid, title
    return None, None


def process_row(row):
    """Mutates row in place, setting matched_qid/grounding_text/grounding_status."""
    qid = row.get("wikidata_qid", "").strip()
    try:
        if qid:
            title, _ = wd_entity_sitelink_and_coord(qid)
        else:
            qid, title = rematch_by_name(row["name"], float(row["lat"]), float(row["lon"]))
    except (requests.RequestException, RuntimeError) as exc:
        row["grounding_status"] = f"error:{type(exc).__name__}"
        return

    if not title:
        row["grounding_status"] = "no_match"
        return

    try:
        text = wp_extract(title, lang="pt")
        if not text:
            text = wp_extract(title, lang="en")
    except (requests.RequestException, RuntimeError) as exc:
        row["grounding_status"] = f"error:{type(exc).__name__}"
        return

    row["matched_qid"] = qid or ""
    if text:
        row["grounding_text"] = text
        row["grounding_status"] = "ok"
    else:
        row["grounding_status"] = "no_text"


def main(input_path, output_path, limit=None):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    for r in rows:
        for col in ("matched_qid", "grounding_text", "grounding_status"):
            r.setdefault(col, "")

    fieldnames = list(rows[0].keys())
    targets = [r for r in rows if not r["grounding_status"]]
    if limit:
        targets = targets[:limit]

    print(f"À traiter: {len(targets)} (sur {len(rows)} au total, {len(rows) - len(targets)} déjà faits)")

    for i, row in enumerate(targets, 1):
        process_row(row)
        time.sleep(0.3)
        print(f"  [{i}/{len(targets)}] {row['name']!r} -> {row['grounding_status']}")
        if i % CHECKPOINT_EVERY == 0:
            with open(output_path, "w", newline="", encoding="utf-8") as f:
                writer = csv.DictWriter(f, fieldnames=fieldnames)
                writer.writeheader()
                writer.writerows(rows)
            print(f"    [checkpoint écrit à {i}/{len(targets)}]")

    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)

    from collections import Counter
    print()
    print("Répartition finale des statuts :")
    for status, n in Counter(r["grounding_status"] for r in rows if r["grounding_status"]).most_common():
        print(f"  {status}: {n}")


if __name__ == "__main__":
    limit = int(sys.argv[3]) if len(sys.argv) > 3 else None
    main(sys.argv[1], sys.argv[2], limit)
