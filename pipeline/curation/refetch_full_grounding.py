"""Re-fetch FULL Wikipedia article text (not just the lead/intro) for every
place that already has a matched_qid in places_grounding_for_verification_v1.csv.

Why this exists: enrich_grounding.py's wp_extract() passes exintro=1, which
returns only the lead paragraph(s) -- fine for its original purpose (short
narration generation) but useless for verifying the CURRENT, much richer
narration style against its real source. Comparing rich narration against a
one-sentence stub produced a wave of false "unsupported claim" findings in
the first judge pass. This script reuses the already-resolved matched_qid
(no need to re-run the Wikidata name/distance matching), re-resolves its
sitelink title, and fetches the FULL article extract instead.

Resumable/checkpointed the same way as enrich_grounding.py.
"""
import csv
import sys
import time

sys.path.insert(0, ".")
from curation.enrich_grounding import get_with_retry, WD_API, WP_API_PT, WP_API_EN, USER_AGENT  # noqa: E402

CHECKPOINT_EVERY = 25


def wd_sitelink(qid):
    data = get_with_retry(WD_API, {
        "action": "wbgetentities", "ids": qid, "props": "sitelinks", "format": "json",
    })
    ent = data["entities"][qid]
    return ent.get("sitelinks", {}).get("ptwiki", {}).get("title") or ent.get("sitelinks", {}).get("enwiki", {}).get("title")


def wp_full_extract(title, lang):
    api = WP_API_PT if lang == "pt" else WP_API_EN
    data = get_with_retry(api, {
        "action": "query", "prop": "extracts", "explaintext": 1,
        "redirects": 1, "titles": title, "format": "json",
    })
    page = next(iter(data["query"]["pages"].values()))
    return (page.get("extract") or "").strip()


def main(input_path, output_path):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    for r in rows:
        r.setdefault("full_grounding_status", "")
        r.setdefault("full_grounding_text", "")

    fieldnames = list(rows[0].keys())
    targets = [r for r in rows if r["grounding_status"] == "ok" and not r["full_grounding_status"]]
    print(f"À refetcher en version complète: {len(targets)} (sur {len(rows)} lignes au total)")

    for i, row in enumerate(targets, 1):
        qid = row["matched_qid"]
        try:
            title = wd_sitelink(qid)
            if not title:
                row["full_grounding_status"] = "no_sitelink"
            else:
                lang = "pt"
                text = wp_full_extract(title, "pt")
                if not text:
                    lang = "en"
                    text = wp_full_extract(title, "en")
                if text:
                    row["full_grounding_text"] = text
                    row["full_grounding_status"] = f"ok:{lang}:{len(text)}chars"
                else:
                    row["full_grounding_status"] = "no_text"
        except Exception as exc:  # noqa: BLE001 -- resumable batch job, log and move on
            row["full_grounding_status"] = f"error:{type(exc).__name__}"

        time.sleep(0.3)
        print(f"  [{i}/{len(targets)}] {row['name']!r} -> {row['full_grounding_status']}")
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
    print("Répartition finale:")
    for status, n in Counter(r["full_grounding_status"] for r in rows if r["full_grounding_status"]).most_common():
        print(f"  {status}: {n}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
