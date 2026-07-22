"""One-off curation pass on a places CSV export: noise filtering + conservative
cross-source near-duplicate merging. Not part of the reusable pipeline (yet) —
run manually, inspect results, decide whether to fold into sourcing/dedup.py.
"""
import csv
import re
import sys
import unicodedata

NOISE_PATTERN = re.compile(
    r"\b("
    r"apartment|apartamento|apartamentos|residencial|residence|residences|"
    r"residential|condom[íi]nio|condominio|rental|suites?|flat|temporada|"
    r"alugue?l|imobili[áa]ria|im[óo]veis|guest\s*house|guesthouse|"
    r"vacation|hospedagem|pousada\s*e\s*apart|portaria"
    r")\b",
    re.IGNORECASE,
)

# Guard list: famous, distinct landmarks that must NEVER be merged into each
# other or dropped. Checked before and after every destructive operation.
GUARD_LIST = [
    "Museu da República", "Museu de Arte do Rio", "Palácio do Catete",
    "Candelária", "Convento do Carmo", "Igreja de Nossa Senhora do Carmo", "Selar",
    "Alfândega do Rio de Janeiro", "Cristo Redentor", "Pão de Açúcar",
    "Theatro Municipal", "Confeitaria Colombo",
    "Copacabana Palace", "Forte de Copacabana",
]


def normalize(name: str) -> str:
    lowered = name.strip().lower()
    no_accents = "".join(c for c in unicodedata.normalize("NFKD", lowered) if not unicodedata.combining(c))
    no_paren = re.sub(r"\([^)]*\)", "", no_accents)
    # strip prefixes BEFORE stripping punctuation, so "ccbb - x" matches "x"
    for prefix in ["ccbb - ", "museu de ", "museu do ", "museu da ", "igreja de ", "igreja do ", "igreja da "]:
        if no_paren.strip().startswith(prefix):
            no_paren = no_paren.strip()[len(prefix):]
            break
    cleaned = re.sub(r"[^a-z0-9 ]", " ", no_paren)
    cleaned = re.sub(r"\s+", " ", cleaned).strip()
    # drop the bare acronym "ccbb" wherever it appears, so "ccbb rio de janeiro"
    # and "centro cultural banco do brasil" converge once the CCBB self-reference
    # is removed from both sides
    cleaned = re.sub(r"\bccbb\b", "", cleaned).strip()
    cleaned = re.sub(r"\s+", " ", cleaned)
    return cleaned


def guard_check(rows, stage):
    names = {r["name"] for r in rows}
    missing = [g for g in GUARD_LIST if not any(g.lower() in n.lower() for n in names)]
    if missing:
        print(f"[GUARD FAILED at {stage}] Missing: {missing}", file=sys.stderr)
        sys.exit(1)
    print(f"[guard ok at {stage}] all {len(GUARD_LIST)} landmarks present, {len(rows)} rows total")


def main(input_path, output_path):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    guard_check(rows, "start")

    # Pass 1: noise filtering (mechanical, safe)
    clean = [r for r in rows if not NOISE_PATTERN.search(r["name"])]
    removed_noise = len(rows) - len(clean)
    guard_check(clean, "after noise filter")

    # Pass 2: conservative cross-source near-duplicate merge.
    # Only merge when normalized names match AND at least one side has a
    # wikidata_qid (never merge two purely-Overture generic entries this way —
    # that's dedup.py's job with its own, already-reviewed distance rules).
    by_norm = {}
    for r in clean:
        key = normalize(r["name"])
        by_norm.setdefault(key, []).append(r)

    merged = []
    merge_log = []
    for key, group in by_norm.items():
        if len(group) == 1:
            merged.append(group[0])
            continue
        with_qid = [r for r in group if r["wikidata_qid"].strip()]
        if with_qid:
            # Prefer the Wikidata-sourced record's coordinates/QID; keep it.
            survivor = with_qid[0]
            merge_log.append((key, [r["name"] for r in group], survivor["name"]))
        else:
            # No QID anchor for this name group — don't merge, keep first seen
            # only if truly identical name; otherwise keep all (safer default).
            exact = {r["name"] for r in group}
            if len(exact) == 1:
                survivor = group[0]
                merge_log.append((key, [r["name"] for r in group], survivor["name"]))
            else:
                merged.extend(group)
                continue
        merged.append(survivor)

    guard_check(merged, "after cross-source merge")

    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=["name", "category", "source", "lat", "lon", "wikidata_qid", "id"])
        writer.writeheader()
        for r in sorted(merged, key=lambda x: (x["category"], x["name"])):
            writer.writerow(r)

    print()
    print(f"Départ: {len(rows)}")
    print(f"Bruit retiré: {removed_noise}")
    print(f"Fusions cross-source (nom normalisé + QID ancre): {len(merge_log)}")
    print(f"Final: {len(merged)}")
    print()
    print("Détail des fusions (pour audit) :")
    for key, names, survivor in merge_log:
        print(f"  {names} -> gardé: {survivor!r}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
