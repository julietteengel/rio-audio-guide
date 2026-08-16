import duckdb

from sourcing.models import Place

CATEGORY_ALLOWLIST = {
    "landmark_and_historical_building",
    "monument",
    "museum",
    "history_museum",
    "art_museum",
    "modern_art_museum",
    "topic_concert_venue",
    "cultural_center",
    "art_gallery",
    "music_venue",
    "theatre",
    "venue_and_event_space",
    "performing_arts",
    "science_museum",
    "contemporary_art_museum",
    "design_museum",
    "childrens_museum",
    "civilization_museum",
    "community_museum",
    "state_museum",
    "sports_museum",
    "cartooning_museum",
    "aviation_museum",
    "costume_museum",
    "church_cathedral",
}

# Explicitly excluded, not forgotten: "beach", "botanical_garden", "national_park".
# Scope decision: culturel strict — no nature/landscape categories, even popular
# ones. landmark_and_historical_building still needs its own cultural-vs-natural
# triage downstream (it mixes built heritage with hills/rocks/waterfalls/viewpoints
# under the same Overture category).
#
# "church_cathedral" added deliberately narrow: Overture also has
# "religious_organization", "evangelical_church", "pentecostal_church",
# "baptist_church", "catholic_church" etc. in this bbox, at far higher volume
# (tens of thousands combined) and overwhelmingly ordinary neighborhood
# congregations with no heritage/touristic relevance. church_cathedral alone
# still needs its own cultural-vs-noise triage downstream, same as
# landmark_and_historical_building.

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
