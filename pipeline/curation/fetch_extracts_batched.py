# -*- coding: utf-8 -*-
"""Fetch Wikipedia intro extracts in batches of 20 titles per request
(TextExtracts' real cap even with exlimit=max) for places matched via
bulk_wikidata_match.py's SPARQL bulk query. Uses a compliant User-Agent per
Wikimedia's 2026 API usage policy (meaningful UA + contact info) to avoid
the restrictive anonymous-traffic rate tier.

Resumable: skips place ids already present in the output file.
"""
import json
import sys
import time

import requests

USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"
BATCH = 20  # TextExtracts' exlimit caps at 20 even with exlimit=max


def fetch_batch(titles, lang, max_retries=6):
    api = f"https://{lang}.wikipedia.org/w/api.php"
    delay = 5.0
    for attempt in range(max_retries):
        resp = requests.get(api, params={
            "action": "query",
            "prop": "extracts",
            "exintro": 1,
            "explaintext": 1,
            "exlimit": "max",
            "redirects": 1,
            "titles": "|".join(titles),
            "format": "json",
        }, headers={"User-Agent": USER_AGENT}, timeout=30)
        if resp.status_code == 429:
            retry_after = resp.headers.get("Retry-After")
            wait = float(retry_after) if retry_after else delay
            print(f"    [429, retry {attempt+1}/{max_retries} in {wait:.0f}s]", file=sys.stderr)
            time.sleep(wait)
            delay *= 2
            continue
        resp.raise_for_status()
        break
    else:
        raise RuntimeError(f"Still throttled after {max_retries} retries")
    data = resp.json()
    pages = data.get("query", {}).get("pages", {})
    normalized = {n["from"]: n["to"] for n in data.get("query", {}).get("normalized", [])}
    redirects = {r["from"]: r["to"] for r in data.get("query", {}).get("redirects", [])}
    result = {}
    for page in pages.values():
        title = page.get("title", "")
        extract = (page.get("extract") or "").strip()
        result[title] = extract
    return result, normalized, redirects


def main(matches_path, output_path):
    with open(matches_path, encoding="utf-8") as f:
        matches = json.load(f)  # place_id -> {qid, wikidata_label, lang, title, dist_m}

    try:
        with open(output_path, encoding="utf-8") as f:
            all_results = json.load(f)
        print(f"Resuming: {len(all_results)} already fetched", file=sys.stderr)
    except FileNotFoundError:
        all_results = {}  # place_id -> {title, lang, extract, qid, dist_m}

    by_lang = {}
    for place_id, m in matches.items():
        if place_id in all_results:
            continue
        by_lang.setdefault(m["lang"], []).append((place_id, m))

    for lang, items in by_lang.items():
        for i in range(0, len(items), BATCH):
            batch = items[i:i + BATCH]
            titles = [m["title"] for _, m in batch]
            print(f"[{lang}] batch {i // BATCH + 1}/{(len(items) - 1) // BATCH + 1} ({len(batch)} titles)", file=sys.stderr)
            extracts, normalized, redirects = fetch_batch(titles, lang)
            for place_id, m in batch:
                t = m["title"]
                t2 = normalized.get(t, t)
                t3 = redirects.get(t2, t2)
                extract = extracts.get(t3) or extracts.get(t2) or extracts.get(t)
                if extract:
                    all_results[place_id] = {
                        "title": t, "lang": lang, "extract": extract,
                        "qid": m["qid"], "dist_m": m["dist_m"],
                    }
            with open(output_path, "w", encoding="utf-8") as f:
                json.dump(all_results, f, ensure_ascii=False, indent=2)
            time.sleep(2)

    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(all_results, f, ensure_ascii=False, indent=2)

    print(f"Total extracts fetched: {len(all_results)} / {len(matches)}", file=sys.stderr)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
