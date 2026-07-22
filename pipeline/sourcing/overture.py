import duckdb

from sourcing.models import Place

CATEGORY_ALLOWLIST = {
    "landmark_and_historical_building",
    "monument",
    "museum",
    "history_museum",
    "art_museum",
    "modern_art_museum",
    "botanical_garden",
    "national_park",
    "topic_concert_venue",
    "cultural_center",
    # Ajouté après audit manuel : "beach" avait été identifié dès les tout
    # premiers tests de cette recherche (777 entrées dans la bbox Rio) mais
    # jamais reporté dans l'allowlist finale — Copacabana, Ipanema, Flamengo
    # et Botafogo, parmi les lieux les plus incontournables de Rio, étaient
    # de ce fait totalement absents du pipeline.
    "beach",
}

DEFAULT_RELEASE = "2026-06-17.0"


def filter_by_category(rows: list[dict], allowlist: set[str] = CATEGORY_ALLOWLIST) -> list[dict]:
    return [row for row in rows if row.get("category") in allowlist]


def query_overture_places(
    bbox: tuple[float, float, float, float],
    release: str = DEFAULT_RELEASE,
) -> list[Place]:
    """bbox = (min_lon, min_lat, max_lon, max_lat)."""
    min_lon, min_lat, max_lon, max_lat = bbox
    con = duckdb.connect()
    con.execute("INSTALL spatial; INSTALL httpfs; LOAD spatial; LOAD httpfs;")
    con.execute("SET s3_region='us-west-2';")
    query = f"""
        SELECT names.primary AS name, categories.primary AS category,
               bbox.xmin AS lon, bbox.ymin AS lat
        FROM read_parquet('s3://overturemaps-us-west-2/release/{release}/theme=places/type=place/*.parquet')
        WHERE bbox.xmin BETWEEN {min_lon} AND {max_lon}
          AND bbox.ymin BETWEEN {min_lat} AND {max_lat}
    """
    rows = con.execute(query).fetchall()
    columns = [desc[0] for desc in con.description]
    dict_rows = [dict(zip(columns, row)) for row in rows]
    filtered = filter_by_category(dict_rows)
    return [
        Place(name=row["name"], lat=row["lat"], lon=row["lon"], category=row["category"], source="overture")
        for row in filtered
        if row["name"]
    ]
