# -*- coding: utf-8 -*-
"""Fetch Wikipedia intro extracts in batches of 50 titles per request
(instead of one request per place) for places matched via the Wikidata
SPARQL bulk query. Uses a compliant User-Agent per Wikimedia's 2026 API
usage policy (meaningful UA + contact info) to avoid the restrictive
anonymous-traffic rate tier.
"""
import json
import pickle
import sys
import time
import urllib.parse

import requests

USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"


def fetch_batch(titles, lang, max_retries=6):
    api = f"https://{lang}.wikipedia.org/w/api.php"
    delay = 5.0
    for attempt in range(max_retries):
        resp = requests.get(api, params={
            "action": "query",
            "prop": "extracts",
            "exintro": 1,
            "explaintext": 1,
            "exlimit": "max",  # without this, TextExtracts only computes the extract for the FIRST title in the batch
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


def main():
    with open("/Users/julietteengel/.claude/jobs/b341cee9/tmp/wikidata_matches.pkl", "rb") as f:
        matches = pickle.load(f)

    def get_title_lang(wp):
        for lang, key in [("pt", "artPT"), ("fr", "artFR"), ("en", "artEN"), ("es", "artES")]:
            url = wp.get(key)
            if url:
                title = urllib.parse.unquote(url.rsplit("/", 1)[-1]).replace("_", " ")
                return lang, title
        return None, None

    by_lang = {}
    for r, wp, d in matches:
        lang, title = get_title_lang(wp)
        by_lang.setdefault(lang, []).append({"place_name": r["name"], "title": title, "qid": wp["qid"], "dist": d})

    try:
        with open("curation/wikidata_bulk_extracts.json", encoding="utf-8") as f:
            all_results = json.load(f)
        print(f"Resuming: {len(all_results)} already fetched", file=sys.stderr)
    except FileNotFoundError:
        all_results = {}  # place_name -> {title, lang, extract}

    for lang, items in by_lang.items():
        items = [it for it in items if it["place_name"] not in all_results]
        titles = [it["title"] for it in items]
        BATCH = 20  # TextExtracts' exlimit caps at 20 even with exlimit=max
        for i in range(0, len(titles), BATCH):
            batch = titles[i:i+BATCH]
            print(f"[{lang}] batch {i//BATCH+1}/{(len(titles)-1)//BATCH+1} ({len(batch)} titles)", file=sys.stderr)
            extracts, normalized, redirects = fetch_batch(batch, lang)
            # resolve title -> extract, following normalization/redirects
            for it in items[i:i+BATCH]:
                t = it["title"]
                t2 = normalized.get(t, t)
                t3 = redirects.get(t2, t2)
                extract = extracts.get(t3) or extracts.get(t2) or extracts.get(t)
                if extract:
                    all_results[it["place_name"]] = {"title": t, "lang": lang, "extract": extract, "qid": it["qid"], "dist_m": it["dist"]}
            with open("curation/wikidata_bulk_extracts.json", "w", encoding="utf-8") as f:
                json.dump(all_results, f, ensure_ascii=False, indent=2)
            time.sleep(2)

    with open("curation/wikidata_bulk_extracts.json", "w", encoding="utf-8") as f:
        json.dump(all_results, f, ensure_ascii=False, indent=2)

    print(f"Total extracts fetched: {len(all_results)} / {len(matches)}", file=sys.stderr)


if __name__ == "__main__":
    main()
