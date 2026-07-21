import json
from pathlib import Path

from sourcing.dedup import deduplicate_places
from sourcing.feiras import fetch_and_parse_feiras_pdf, feiras_to_places
from sourcing.models import Place
from sourcing.overture import query_overture_places
from sourcing.wikidata import query_iphan_heritage_sites

# Santa Teresa + Lapa (min_lon, min_lat, max_lon, max_lat) — à affiner si le
# périmètre réel s'avère trop large ou trop étroit après une première passe.
SANTA_TERESA_LAPA_BBOX = (-43.1950, -22.9250, -43.1750, -22.9050)


def run_pipeline(output_path: Path) -> list[Place]:
    overture_places = query_overture_places(SANTA_TERESA_LAPA_BBOX)
    wikidata_places = query_iphan_heritage_sites()
    feiras_raw = fetch_and_parse_feiras_pdf()
    feiras_places = feiras_to_places(feiras_raw)

    all_places = overture_places + wikidata_places + feiras_places
    deduped = deduplicate_places(all_places)

    output_path.write_text(
        json.dumps([place.__dict__ for place in deduped], ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    return deduped


if __name__ == "__main__":
    result = run_pipeline(Path("places.json"))
    print(f"{len(result)} lieux candidats écrits dans places.json")
