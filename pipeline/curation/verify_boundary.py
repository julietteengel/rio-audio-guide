"""Municipal-boundary check on the CULTURAL set's own stored coordinates
(not just on matched grounding sources -- a place's own coordinates can
already be in the wrong municipality, independent of whether it later
grounds to a real source). Overture's category tagging and our cultural
triage both judge content, not geography, so places physically in Niterói,
São João de Meriti, Nova Iguaçu, Duque de Caxias etc. can slip through even
when everything else about them is correct.

Reverse-geocodes each place via Nominatim (1 req/s, per usage policy).
Resumable: skips rows that already have boundary_municipality set.
"""
import csv
import sys
import time

import requests

USER_AGENT = "rio-audio-guide-content-pipeline/1.0 (non-commercial research project; contact via project repo)"
NOMINATIM_REVERSE_URL = "https://nominatim.openstreetmap.org/reverse"
RIO_MUNICIPALITY_NAMES = {"rio de janeiro"}
CHECKPOINT_EVERY = 25


def get_with_retry(url, params, max_retries=5):
    delay = 2.0
    for attempt in range(max_retries):
        resp = requests.get(url, params=params, headers={"User-Agent": USER_AGENT}, timeout=20)
        if resp.status_code == 429:
            retry_after = resp.headers.get("Retry-After")
            wait = float(retry_after) if retry_after else delay
            print(f"    [429, retry {attempt+1}/{max_retries} in {wait:.0f}s]", file=sys.stderr)
            time.sleep(wait)
            delay *= 2
            continue
        resp.raise_for_status()
        return resp.json()
    raise RuntimeError(f"Still rate-limited after {max_retries} retries: {url}")


def municipality_for(lat, lon):
    # zoom=10 is too coarse for some coastal/island points (e.g. Ilha do
    # Fundão / Cidade Universitária) -- Nominatim falls back to a state-level
    # polygon and returns no city/town/municipality at all. Retry at a finer
    # zoom before treating "no municipality" as a real signal.
    for zoom in (10, 16):
        data = get_with_retry(NOMINATIM_REVERSE_URL, {
            "lat": lat, "lon": lon, "format": "json", "zoom": zoom, "addressdetails": 1,
        })
        address = data.get("address", {})
        municipality = address.get("city") or address.get("town") or address.get("municipality")
        if municipality:
            return municipality
        time.sleep(1)
    return None


def main(input_path, output_path):
    with open(input_path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))

    for r in rows:
        r.setdefault("boundary_municipality", "")
        r.setdefault("boundary_status", "")

    fieldnames = list(rows[0].keys())
    targets = [r for r in rows if not r["boundary_status"]]
    print(f"À vérifier: {len(targets)} (sur {len(rows)} au total)")

    for i, row in enumerate(targets, 1):
        try:
            municipality = municipality_for(float(row["lat"]), float(row["lon"]))
        except (requests.RequestException, RuntimeError) as exc:
            row["boundary_status"] = f"error:{type(exc).__name__}"
            time.sleep(1)
            continue

        row["boundary_municipality"] = municipality or ""
        if municipality and municipality.strip().lower() in RIO_MUNICIPALITY_NAMES:
            row["boundary_status"] = "ok"
        else:
            row["boundary_status"] = "outside_boundary"

        print(f"  [{i}/{len(targets)}] {row['name']!r} -> {row['boundary_status']} ({municipality})")
        time.sleep(1)  # Nominatim public-instance usage policy: max 1 req/s.

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
    for status, n in Counter(r["boundary_status"] for r in rows if r["boundary_status"]).most_common():
        print(f"  {status}: {n}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
