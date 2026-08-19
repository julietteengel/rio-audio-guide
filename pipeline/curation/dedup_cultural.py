"""One-off curation pass: run the pipeline's dedup.py (QID or ≤100m same-
category proximity) against the CULTURAL subset of scope_classification_v2.csv,
producing the base dataset for grounding/narration. Not part of the reusable
pipeline (yet) — run manually, inspect results.
"""
import csv
import sys

from sourcing.models import Place
from sourcing.dedup import deduplicate_places


def main(places_path, scope_path, output_path):
    places_by_id = {}
    with open(places_path, encoding="utf-8") as f:
        for row in csv.DictReader(f):
            places_by_id[row["id"]] = row

    cultural_ids = []
    reasons = {}
    with open(scope_path, encoding="utf-8") as f:
        for row in csv.DictReader(f):
            if row["scope"] == "CULTURAL":
                cultural_ids.append(row["id"])
                reasons[row["id"]] = row["reason"]

    place_objs = [
        Place(
            name=places_by_id[pid]["name"],
            lat=float(places_by_id[pid]["lat"]),
            lon=float(places_by_id[pid]["lon"]),
            category=places_by_id[pid]["category"],
            source=places_by_id[pid]["source"],
            wikidata_qid=places_by_id[pid]["wikidata_qid"] or None,
            id=pid,
        )
        for pid in cultural_ids
    ]

    deduped = deduplicate_places(place_objs)
    kept_ids = {p.id for p in deduped}
    dropped = [p for p in place_objs if p.id not in kept_ids]

    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(
            f, fieldnames=["id", "name", "category", "source", "lat", "lon", "wikidata_qid", "reason"]
        )
        writer.writeheader()
        for p in sorted(deduped, key=lambda x: (x.category, x.name)):
            writer.writerow(
                {
                    "id": p.id,
                    "name": p.name,
                    "category": p.category,
                    "source": p.source,
                    "lat": p.lat,
                    "lon": p.lon,
                    "wikidata_qid": p.wikidata_qid or "",
                    "reason": reasons.get(p.id, ""),
                }
            )

    print(f"CULTURAL candidates: {len(place_objs)}")
    print(f"after dedup.py (QID or <=100m same-category proximity): {len(deduped)}")
    print(f"removed: {len(dropped)}")
    for p in dropped:
        print(f"  dropped: {p.name!r} ({p.category}, {p.source})")

    # Known unresolved cases (checked manually, see docs/superpowers/plans/
    # content-pipeline.plan.md): same-name groups too far apart or with
    # mismatched categories for dedup.py's conservative rules to touch —
    # Centro Cultural Banco do Brasil (x3), Museu da Imagem e do Som (x3),
    # Museu da Imagem e do Som (MIS) (x2), Cidade das Artes (x2, 191m apart).
    # Left as separate candidates deliberately: no wikidata_qid anchor on any
    # of them, and this project's dedup rule intentionally declines to merge
    # on name+proximity alone when the signal is ambiguous. Real resolution
    # needs per-record grounding (which one has a genuine web source at that
    # exact coordinate), not a guess here.


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3])
