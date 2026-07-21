import json
from pathlib import Path

from sourcing.dedup import deduplicate_places
from sourcing.feiras import fetch_and_parse_feiras_pdf, feiras_to_places
from sourcing.models import Place
from sourcing.overture import query_overture_places
from sourcing.wikidata import query_iphan_heritage_sites

# Municipalité de Rio de Janeiro entière (min_lon, min_lat, max_lon, max_lat).
# Source : limites administratives officielles OSM/Nominatim (relation 2697338),
# vérifiées en direct. Anciennement restreint à Santa Teresa + Lapa seuls ;
# élargi à la ville entière sur décision explicite (couverture complète dès le
# MVP, pas juste 2 quartiers).
RIO_DE_JANEIRO_BBOX = (-43.7962520, -23.0827051, -43.0990811, -22.7460878)


def _within_bbox(place: Place, bbox: tuple[float, float, float, float]) -> bool:
    min_lon, min_lat, max_lon, max_lat = bbox
    return min_lon <= place.lon <= max_lon and min_lat <= place.lat <= max_lat


def run_pipeline(output_path: Path) -> list[Place]:
    overture_places = query_overture_places(RIO_DE_JANEIRO_BBOX)
    wikidata_places = query_iphan_heritage_sites()
    feiras_raw = fetch_and_parse_feiras_pdf()
    feiras_places = feiras_to_places(feiras_raw)

    all_places = overture_places + wikidata_places + feiras_places
    in_scope_places = [place for place in all_places if _within_bbox(place, RIO_DE_JANEIRO_BBOX)]
    deduped = deduplicate_places(in_scope_places)

    output_path.write_text(
        json.dumps([place.__dict__ for place in deduped], ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    return deduped


if __name__ == "__main__":
    result = run_pipeline(Path("places.json"))
    print(f"{len(result)} lieux candidats écrits dans places.json")
