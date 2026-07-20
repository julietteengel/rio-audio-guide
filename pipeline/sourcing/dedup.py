import re
import unicodedata
from math import radians, sin, cos, sqrt, atan2

from sourcing.models import Place

GENERIC_PREFIXES = [
    "museu de ", "museu do ", "museu da ",
    "igreja de ", "igreja do ", "igreja da ",
    "praça ", "parque ",
]


def normalize_name(name: str) -> str:
    lowered = name.strip().lower()
    without_accents = "".join(
        c for c in unicodedata.normalize("NFKD", lowered) if not unicodedata.combining(c)
    )
    collapsed = re.sub(r"\s+", " ", without_accents).strip()
    for prefix in GENERIC_PREFIXES:
        if collapsed.startswith(prefix):
            collapsed = collapsed[len(prefix):]
            break
    return collapsed.strip()


def haversine_distance_m(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    r = 6371000.0
    phi1, phi2 = radians(lat1), radians(lat2)
    dphi = radians(lat2 - lat1)
    dlambda = radians(lon2 - lon1)
    a = sin(dphi / 2) ** 2 + cos(phi1) * cos(phi2) * sin(dlambda / 2) ** 2
    return 2 * r * atan2(sqrt(a), sqrt(1 - a))


def deduplicate_places(places: list[Place], max_distance_m: float = 100.0) -> list[Place]:
    kept: list[Place] = []
    for place in places:
        is_duplicate = False
        for existing in kept:
            same_qid = place.wikidata_qid and place.wikidata_qid == existing.wikidata_qid
            same_name_and_close = (
                normalize_name(place.name) == normalize_name(existing.name)
                and haversine_distance_m(place.lat, place.lon, existing.lat, existing.lon) <= max_distance_m
            )
            if same_qid or same_name_and_close:
                is_duplicate = True
                break
        if not is_duplicate:
            kept.append(place)
    return kept
