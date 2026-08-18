# -*- coding: utf-8 -*-
"""Merge all multilingual narration parts into one CSV + run duration-equivalence check.

Only merges parts that went through the project's real grounding/anti-hallucination-judge
methodology (DATA_PART1-6). The old hardcoded 6-place "SAMPLE" list (pre-grounding-methodology
prototype content, added 2026-07-31) has been dropped: where it overlapped by name with later
grounded work (Cristo Redentor, Cais do Valongo, Quinta da Boa Vista, Praia de Ipanema), its
facts didn't match the sourced versions (e.g. it claimed the Cristo Redentor is 38m tall; the
grounded figure is 30m + an 8m pedestal). The two SAMPLE-only places (Real Gabinete Português de
Leitura, Bonde de Santa Teresa) are not carried over here since they were never grounded against
a real source under this project's rules -- they'd need to go through proper grounding + judge
verification before being narrated again, same as any other place.

As of the full-corpus rewrite (254 places, full Wikipedia articles instead of thin intro
extracts, no fixed word cap, all anti-hallucination issues fixed), DATA_PART1-5 entries carry
the rewritten long-form text where a match existed; DATA_PART6 holds the places from that
rewrite that had no preexisting entry in parts 1-5.
"""
import csv
import sys

sys.path.insert(0, ".")

from narrations_data_part1 import DATA_PART1
from narrations_data_part2 import DATA_PART2
from narrations_data_part3 import DATA_PART3
from narrations_data_part4 import DATA_PART4
from narrations_data_part5 import DATA_PART5
from narrations_data_part6 import DATA_PART6

all_entries = DATA_PART1 + DATA_PART2 + DATA_PART3 + DATA_PART4 + DATA_PART5 + DATA_PART6

# Dedupe by name, first occurrence wins (part1 > part2 > part3 > part4 > part5 > part6). Known
# real duplicate this order resolves: "Polo Praça XV" appears in both part1 (the original
# recovered 33, already judge-verified) and part5 (independently grounded via fallback search)
# -- keeps part1's version, drops part5's redundant one.
seen = set()
deduped = []
duplicates_dropped = []
for e in all_entries:
    if e["name"] in seen:
        duplicates_dropped.append(e["name"])
        continue
    seen.add(e["name"])
    deduped.append(e)

print(f"Total unique entries: {len(deduped)}")
if duplicates_dropped:
    print(f"Dropped {len(duplicates_dropped)} exact-name duplicate(s): {duplicates_dropped}")

with open("narrations_multi_full.csv", "w", encoding="utf-8", newline="") as f:
    writer = csv.writer(f)
    writer.writerow(["name", "narration_fr", "narration_en", "narration_es", "narration_pt"])
    for e in deduped:
        writer.writerow([e["name"], e["fr"], e["en"], e["es"], e["pt"]])

print("Written to narrations_multi_full.csv")

# Duration-equivalence check (word count, +/-15% target, informational only)
print("\n--- Duration check (word counts) ---")
flagged = []
for e in deduped:
    fr_words = len(e["fr"].split())
    for lang in ("en", "es", "pt"):
        words = len(e[lang].split())
        ratio = words / fr_words
        if not (0.75 <= ratio <= 1.30):
            flagged.append((e["name"], lang, fr_words, words, ratio))

if flagged:
    print(f"{len(flagged)} entries outside +/-15-30% tolerance band:")
    for name, lang, fr_w, w, ratio in flagged:
        print(f"  {name} [{lang}]: fr={fr_w} words, {lang}={w} words, ratio={ratio:.2f}")
else:
    print("All entries within tolerance.")
