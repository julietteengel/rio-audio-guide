# Pipeline de Sourcing des Lieux — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construire un script Python autonome qui interroge Overture Maps, Wikidata (patrimoine IPHAN) et le registre officiel des feiras livres, déduplique les résultats entre sources, et produit un fichier JSON de lieux candidats pour Santa Teresa/Lapa — la base de données brute à partir de laquelle les 25 lieux du MVP seront sélectionnés manuellement.

**Architecture:** Un module Python par source de données (`overture.py`, `wikidata.py`, `feiras.py`), un module de déduplication pur sans dépendance réseau (`dedup.py`), et un orchestrateur (`pipeline.py`) qui combine tout et écrit le JSON final. Chaque module de source sépare sa logique pure (testable sans réseau, avec mocks/fixtures) de son appel réseau (testé en intégration).

**Tech Stack:** Python 3.11+ (éviter 3.14, incompatibilités SSL constatées avec certaines libs sur macOS), `duckdb` (requêtes Overture via S3 parquet), `requests` (Wikidata SPARQL, Nominatim, PDF), `pdfplumber` (extraction de tableau PDF), `pytest`.

## Global Constraints

- Zone géographique v1 : Santa Teresa + Lapa uniquement (bbox à définir précisément en Task 6).
- Catégories Overture retenues : `landmark_and_historical_building`, `monument`, `museum`, `history_museum`, `art_museum`, `modern_art_museum`, `botanical_garden`, `national_park`, `topic_concert_venue`, `cultural_center` — jamais `bar`, `restaurant`, `business_advertising` en bloc (spec : filtrage par catégorie insuffisant seul, ces catégories sont trop bruitées).
- Déduplication : normalisation de nom + proximité géographique (≤100m) + QID Wikidata prioritaire si disponible ; jamais de fusion automatique en cas d'ambiguïté forte.
- Sélection finale des 25 lieux : étape humaine, hors scope de ce pipeline (le pipeline produit des candidats, pas la liste finale).
- Sources IRPH/Riotur/MuseusBr : vérification croisée manuelle (pas d'API propre trouvée) — hors scope de ce pipeline, gérées séparément par la fondatrice.
- Ne jamais scraper d'application concurrente commerciale ; le scraping de portails publics gouvernementaux (feiras, MuseusBr) est légitime.

---

## File Structure

```
pipeline/
  pyproject.toml
  sourcing/
    __init__.py
    models.py       # Place dataclass
    dedup.py         # normalize_name, haversine_distance_m, deduplicate_places
    overture.py      # requête Overture Maps via DuckDB
    wikidata.py      # requête SPARQL Wikidata (patrimoine IPHAN)
    feiras.py        # parsing PDF feiras + géocodage Nominatim
    pipeline.py      # orchestration + écriture JSON
  tests/
    test_models.py
    test_dedup.py
    test_overture.py
    test_wikidata.py
    test_feiras.py
    test_pipeline.py
```

---

### Task 1: Scaffolding du projet et modèle `Place`

**Files:**
- Create: `pipeline/pyproject.toml`
- Create: `pipeline/sourcing/__init__.py`
- Create: `pipeline/sourcing/models.py`
- Test: `pipeline/tests/test_models.py`

**Interfaces:**
- Produces: `Place` dataclass (champs : `name: str`, `lat: float`, `lon: float`, `category: str`, `source: str`, `wikidata_qid: str | None = None`, `id: str` généré automatiquement si non fourni).

- [ ] **Step 1: Créer la structure du projet**

```bash
mkdir -p /Users/julietteengel/code/julietteengel/rio-audio-guide/pipeline/sourcing
mkdir -p /Users/julietteengel/code/julietteengel/rio-audio-guide/pipeline/tests
```

- [ ] **Step 2: Créer `pyproject.toml`**

```toml
[project]
name = "rio-audio-guide-sourcing"
version = "0.1.0"
description = "Location sourcing pipeline for Rio Audio Guide"
requires-python = ">=3.11,<3.13"
dependencies = [
    "duckdb>=1.5.0",
    "requests>=2.32.0",
    "pdfplumber>=0.11.0",
]

[project.optional-dependencies]
dev = ["pytest>=8.0.0"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

- [ ] **Step 3: Créer un venv et installer les dépendances**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/pipeline
python3.12 -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

Expected: installation sans erreur. Si `python3.12` n'est pas disponible, utiliser la version 3.11/3.12 la plus proche installée (`python3 --version` pour vérifier).

- [ ] **Step 4: Écrire le test qui échoue**

Créer `pipeline/tests/test_models.py` :

```python
from sourcing.models import Place


def test_place_generates_stable_id_from_source_name_and_coords():
    place = Place(
        name="Escadaria Selarón",
        lat=-22.9147,
        lon=-43.1806,
        category="landmark_and_historical_building",
        source="overture",
    )
    assert place.id == "overture:Escadaria Selarón:-22.91470:-43.18060"


def test_place_accepts_explicit_id():
    place = Place(name="Test", lat=0.0, lon=0.0, category="museum", source="wikidata", id="custom-id")
    assert place.id == "custom-id"


def test_place_wikidata_qid_defaults_to_none():
    place = Place(name="Test", lat=0.0, lon=0.0, category="museum", source="overture")
    assert place.wikidata_qid is None
```

- [ ] **Step 5: Lancer le test, vérifier qu'il échoue**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/pipeline
pytest tests/test_models.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing'` (ou `models`).

- [ ] **Step 6: Créer `sourcing/__init__.py` (vide) et implémenter `sourcing/models.py`**

`pipeline/sourcing/__init__.py` : fichier vide.

`pipeline/sourcing/models.py` :

```python
from dataclasses import dataclass, field


@dataclass
class Place:
    name: str
    lat: float
    lon: float
    category: str
    source: str
    wikidata_qid: str | None = None
    id: str = field(default="")

    def __post_init__(self):
        if not self.id:
            self.id = f"{self.source}:{self.name}:{self.lat:.5f}:{self.lon:.5f}"
```

- [ ] **Step 7: Lancer le test, vérifier qu'il passe**

```bash
pytest tests/test_models.py -v
```

Expected: `3 passed`.

- [ ] **Step 8: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
git add pipeline/pyproject.toml pipeline/sourcing/__init__.py pipeline/sourcing/models.py pipeline/tests/test_models.py
git commit -m "Add sourcing pipeline scaffolding and Place model"
```

---

### Task 2: Normalisation de noms et déduplication (logique pure, sans réseau)

**Files:**
- Create: `pipeline/sourcing/dedup.py`
- Test: `pipeline/tests/test_dedup.py`

**Interfaces:**
- Consumes: `Place` (Task 1).
- Produces: `normalize_name(name: str) -> str`, `haversine_distance_m(lat1, lon1, lat2, lon2) -> float`, `deduplicate_places(places: list[Place], max_distance_m: float = 100.0) -> list[Place]`.

- [ ] **Step 1: Écrire les tests qui échouent**

Créer `pipeline/tests/test_dedup.py` :

```python
from sourcing.dedup import normalize_name, haversine_distance_m, deduplicate_places
from sourcing.models import Place


def test_normalize_name_strips_accents_and_case():
    assert normalize_name("Escadaria Selarón") == "escadaria selaron"


def test_normalize_name_strips_generic_prefix():
    assert normalize_name("Museu da Chácara do Céu") == "chacara do ceu"


def test_normalize_name_collapses_whitespace():
    assert normalize_name("Igreja   de   Nossa Senhora") == "nossa senhora"


def test_haversine_distance_zero_for_same_point():
    assert haversine_distance_m(-22.9147, -43.1806, -22.9147, -43.1806) == 0.0


def test_haversine_distance_approx_one_km_for_known_delta():
    # 1 degré de latitude vaut environ 111 320 m ; 0.009 degré ~= 1002 m.
    dist = haversine_distance_m(0.0, -43.0, 0.009, -43.0)
    assert 950 < dist < 1050


def test_deduplicate_places_merges_close_same_name():
    places = [
        Place(name="Escadaria Selarón", lat=-22.91470, lon=-43.18060, category="landmark_and_historical_building", source="overture"),
        Place(name="escadaria selaron", lat=-22.91471, lon=-43.18061, category="artwork", source="wikidata"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 1
    assert result[0].source == "overture"


def test_deduplicate_places_keeps_distant_places_with_same_name():
    places = [
        Place(name="Igreja Matriz", lat=-22.9147, lon=-43.1806, category="church_cathedral", source="overture"),
        Place(name="Igreja Matriz", lat=-22.8000, lon=-43.3000, category="church_cathedral", source="overture"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 2


def test_deduplicate_places_merges_by_shared_wikidata_qid_even_if_name_differs():
    places = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture", wikidata_qid="Q1798512"),
        Place(name="National Museum of Brazil", lat=-22.9059, lon=-43.2247, category="museum", source="wikidata", wikidata_qid="Q1798512"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 1
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_dedup.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.dedup'`.

- [ ] **Step 3: Implémenter `sourcing/dedup.py`**

```python
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
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_dedup.py -v
```

Expected: `7 passed`.

- [ ] **Step 5: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
git add pipeline/sourcing/dedup.py pipeline/tests/test_dedup.py
git commit -m "Add name normalization and cross-source deduplication logic"
```

---

### Task 3: Requête Overture Maps

**Files:**
- Create: `pipeline/sourcing/overture.py`
- Test: `pipeline/tests/test_overture.py`

**Interfaces:**
- Consumes: `Place` (Task 1).
- Produces: `CATEGORY_ALLOWLIST: set[str]`, `filter_by_category(rows: list[dict], allowlist: set[str] = CATEGORY_ALLOWLIST) -> list[dict]`, `query_overture_places(bbox: tuple[float, float, float, float], release: str = DEFAULT_RELEASE) -> list[Place]` (bbox = `(min_lon, min_lat, max_lon, max_lat)`).

- [ ] **Step 1: Écrire les tests qui échouent**

Créer `pipeline/tests/test_overture.py` :

```python
from sourcing.overture import filter_by_category, query_overture_places, CATEGORY_ALLOWLIST


def test_filter_by_category_keeps_allowlisted_rows():
    rows = [
        {"name": "Cristo Redentor", "category": "monument"},
        {"name": "SeguidoresPremiun", "category": "business_advertising"},
        {"name": "Pilantras Bar", "category": "bar"},
    ]
    result = filter_by_category(rows)
    assert result == [{"name": "Cristo Redentor", "category": "monument"}]


def test_filter_by_category_excludes_noisy_categories_even_if_present():
    rows = [{"name": "Some Agency", "category": "business_advertising"}]
    assert filter_by_category(rows) == []


def test_category_allowlist_does_not_include_bars_or_restaurants():
    assert "bar" not in CATEGORY_ALLOWLIST
    assert "restaurant" not in CATEGORY_ALLOWLIST
    assert "business_advertising" not in CATEGORY_ALLOWLIST


def test_query_overture_places_returns_known_landmark_for_selaron_bbox():
    # Bbox serré autour de l'Escadaria Selarón (frontière Lapa/Santa Teresa).
    # Test d'intégration réel (réseau requis) : vérifié manuellement dans la
    # recherche de conception, cette zone renvoie de manière fiable ce lieu.
    bbox = (-43.1815, -22.9155, -43.1795, -22.9140)
    places = query_overture_places(bbox)
    names = [p.name for p in places]
    assert any("Selarón" in name or "Selaron" in name for name in names)
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_overture.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.overture'`.

- [ ] **Step 3: Implémenter `sourcing/overture.py`**

```python
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
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_overture.py -v
```

Expected: `4 passed`. Le 4e test (`test_query_overture_places_returns_known_landmark_for_selaron_bbox`) nécessite un accès réseau et prend ~10-20s (requête sur le parquet public S3, comme vérifié en conception) — si le CI n'a pas d'accès réseau, marquer ce test `@pytest.mark.integration` et l'exclure du run rapide, mais le garder pour l'exécution locale.

- [ ] **Step 5: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
git add pipeline/sourcing/overture.py pipeline/tests/test_overture.py
git commit -m "Add Overture Maps query module with tourism category allowlist"
```

---

### Task 4: Requête Wikidata (patrimoine IPHAN)

**Files:**
- Create: `pipeline/sourcing/wikidata.py`
- Test: `pipeline/tests/test_wikidata.py`

**Interfaces:**
- Consumes: `Place` (Task 1).
- Produces: `parse_wkt_point(wkt: str) -> tuple[float, float]`, `query_iphan_heritage_sites() -> list[Place]`.

- [ ] **Step 1: Écrire les tests qui échouent**

Créer `pipeline/tests/test_wikidata.py` :

```python
from unittest.mock import patch, Mock

from sourcing.wikidata import parse_wkt_point, query_iphan_heritage_sites


def test_parse_wkt_point_extracts_lat_lon():
    lat, lon = parse_wkt_point("Point(-43.2246 -22.9058)")
    assert lat == -22.9058
    assert lon == -43.2246


def test_query_iphan_heritage_sites_parses_sparql_response():
    fake_response = Mock()
    fake_response.json.return_value = {
        "results": {
            "bindings": [
                {
                    "item": {"value": "http://www.wikidata.org/entity/Q1798512"},
                    "itemLabel": {"value": "Museu Nacional"},
                    "coord": {"value": "Point(-43.2246 -22.9058)"},
                }
            ]
        }
    }
    fake_response.raise_for_status = Mock()
    with patch("sourcing.wikidata.requests.get", return_value=fake_response) as mock_get:
        places = query_iphan_heritage_sites()

    assert len(places) == 1
    assert places[0].name == "Museu Nacional"
    assert places[0].wikidata_qid == "Q1798512"
    assert places[0].lat == -22.9058
    assert places[0].lon == -43.2246
    assert places[0].source == "wikidata"
    mock_get.assert_called_once()
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_wikidata.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.wikidata'`.

- [ ] **Step 3: Implémenter `sourcing/wikidata.py`**

```python
import requests

from sourcing.models import Place

SPARQL_ENDPOINT = "https://query.wikidata.org/sparql"
RIO_DE_JANEIRO_QID = "Q8678"
USER_AGENT = "rio-audio-guide-sourcing/1.0"

IPHAN_HERITAGE_QUERY = f"""
SELECT ?item ?itemLabel ?coord WHERE {{
  ?item wdt:P1435 ?heritage .
  ?item wdt:P625 ?coord .
  ?item wdt:P131* wd:{RIO_DE_JANEIRO_QID} .
  ?heritage rdfs:label ?hlabel .
  FILTER(LANG(?hlabel) = "pt" && CONTAINS(?hlabel, "IPHAN"))
  SERVICE wikibase:label {{ bd:serviceParam wikibase:language "pt,en". }}
}}
"""


def parse_wkt_point(wkt: str) -> tuple[float, float]:
    """Parse 'Point(lon lat)' (format WKT de Wikidata) en (lat, lon)."""
    inner = wkt.strip().removeprefix("Point(").removesuffix(")")
    lon_str, lat_str = inner.split(" ")
    return float(lat_str), float(lon_str)


def query_iphan_heritage_sites() -> list[Place]:
    response = requests.get(
        SPARQL_ENDPOINT,
        params={"query": IPHAN_HERITAGE_QUERY},
        headers={"Accept": "application/sparql-results+json", "User-Agent": USER_AGENT},
        timeout=30,
    )
    response.raise_for_status()
    rows = response.json()["results"]["bindings"]
    places = []
    for row in rows:
        qid = row["item"]["value"].rsplit("/", 1)[-1]
        name = row["itemLabel"]["value"]
        lat, lon = parse_wkt_point(row["coord"]["value"])
        places.append(
            Place(name=name, lat=lat, lon=lon, category="heritage_site", source="wikidata", wikidata_qid=qid)
        )
    return places
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_wikidata.py -v
```

Expected: `2 passed`.

- [ ] **Step 5: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
git add pipeline/sourcing/wikidata.py pipeline/tests/test_wikidata.py
git commit -m "Add Wikidata SPARQL query for IPHAN-listed heritage sites"
```

---

### Task 5: Feiras livres — parsing PDF + géocodage

**Files:**
- Create: `pipeline/sourcing/feiras.py`
- Test: `pipeline/tests/test_feiras.py`

**Interfaces:**
- Consumes: `Place` (Task 1).
- Produces: `parse_feiras_table_rows(rows: list[list[str]]) -> list[dict]`, `geocode_address(query: str) -> tuple[float, float] | None`, `feiras_to_places(feiras: list[dict]) -> list[Place]`, `fetch_and_parse_feiras_pdf(url: str = FEIRAS_PDF_URL) -> list[dict]`.

- [ ] **Step 1: Écrire les tests qui échouent**

Créer `pipeline/tests/test_feiras.py` :

```python
from unittest.mock import patch, Mock

import pytest

from sourcing.feiras import parse_feiras_table_rows, geocode_address, feiras_to_places


def test_parse_feiras_table_rows_extracts_known_columns():
    rows = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana", "RA"],
        ["112", "Não", "AV. AUGUSTO SEVERO...", "GLORIA", "Domingo", "04-Botafogo"],
    ]
    result = parse_feiras_table_rows(rows)
    assert result == [{"descricao": "AV. AUGUSTO SEVERO...", "bairro": "GLORIA", "dia_semana": "Domingo"}]


def test_parse_feiras_table_rows_skips_incomplete_rows():
    rows = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana"],
        ["112", "Não"],
    ]
    assert parse_feiras_table_rows(rows) == []


def test_parse_feiras_table_rows_raises_on_unexpected_header():
    rows = [["Foo", "Bar"]]
    with pytest.raises(ValueError, match="Unexpected feiras table header"):
        parse_feiras_table_rows(rows)


def test_geocode_address_returns_lat_lon_from_first_result():
    fake_response = Mock()
    fake_response.json.return_value = [{"lat": "-22.9147", "lon": "-43.1806"}]
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        result = geocode_address("Escadaria Selarón, Rio de Janeiro, Brazil")
    assert result == (-22.9147, -43.1806)


def test_geocode_address_returns_none_when_no_results():
    fake_response = Mock()
    fake_response.json.return_value = []
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        result = geocode_address("Lieu inexistant, Rio de Janeiro, Brazil")
    assert result is None


def test_feiras_to_places_builds_named_place_from_geocoded_feira():
    feiras = [{"descricao": "RUA TERESINA...", "bairro": "SANTA TERESA", "dia_semana": "Sexta-Feira"}]
    fake_response = Mock()
    fake_response.json.return_value = [{"lat": "-22.9350", "lon": "-43.1900"}]
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        places = feiras_to_places(feiras)
    assert len(places) == 1
    assert places[0].name == "Feira de Santa Teresa (Sexta-Feira)"
    assert places[0].category == "recurring_cultural_event"
    assert places[0].source == "feiras_registry"


def test_feiras_to_places_skips_feiras_that_fail_to_geocode():
    feiras = [{"descricao": "RUA X", "bairro": "BAIRRO INEXISTENTE", "dia_semana": "Segunda-Feira"}]
    fake_response = Mock()
    fake_response.json.return_value = []
    fake_response.raise_for_status = Mock()
    with patch("sourcing.feiras.requests.get", return_value=fake_response):
        places = feiras_to_places(feiras)
    assert places == []
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_feiras.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.feiras'`.

- [ ] **Step 3: Implémenter `sourcing/feiras.py`**

```python
import io

import requests

from sourcing.models import Place

FEIRAS_PDF_URL = (
    "https://ordempublica.prefeitura.rio/wp-content/uploads/sites/30/2024/10/"
    "Relacao-feira-livre-atualizada.pdf"
)
NOMINATIM_URL = "https://nominatim.openstreetmap.org/search"
USER_AGENT = "rio-audio-guide-sourcing/1.0"


def parse_feiras_table_rows(rows: list[list[str]]) -> list[dict]:
    """rows: lignes de tableau telles qu'extraites par un parseur PDF, en-tête inclus.
    Colonnes attendues : Código, Turno, Descrição, Bairro, Dias da Semana, ...
    """
    if not rows:
        return []
    header = [cell.strip().lower() if cell else "" for cell in rows[0]]
    try:
        idx_descricao = header.index("descrição")
        idx_bairro = header.index("bairro")
        idx_dia = header.index("dias da semana")
    except ValueError as exc:
        raise ValueError(f"Unexpected feiras table header: {header}") from exc

    feiras = []
    for row in rows[1:]:
        if len(row) <= max(idx_descricao, idx_bairro, idx_dia):
            continue
        descricao = (row[idx_descricao] or "").strip()
        bairro = (row[idx_bairro] or "").strip()
        dia = (row[idx_dia] or "").strip()
        if descricao and bairro:
            feiras.append({"descricao": descricao, "bairro": bairro, "dia_semana": dia})
    return feiras


def geocode_address(query: str) -> tuple[float, float] | None:
    response = requests.get(
        NOMINATIM_URL,
        params={"q": query, "format": "json", "limit": 1, "countrycodes": "br"},
        headers={"User-Agent": USER_AGENT},
        timeout=15,
    )
    response.raise_for_status()
    results = response.json()
    if not results:
        return None
    return float(results[0]["lat"]), float(results[0]["lon"])


def feiras_to_places(feiras: list[dict]) -> list[Place]:
    places = []
    for feira in feiras:
        query = f"{feira['descricao']}, {feira['bairro']}, Rio de Janeiro, Brazil"
        coords = geocode_address(query)
        if coords is None:
            continue
        lat, lon = coords
        name = f"Feira de {feira['bairro'].title()} ({feira['dia_semana']})"
        places.append(
            Place(name=name, lat=lat, lon=lon, category="recurring_cultural_event", source="feiras_registry")
        )
    return places


def fetch_and_parse_feiras_pdf(url: str = FEIRAS_PDF_URL) -> list[dict]:
    """Wrapper d'I/O fin autour de parse_feiras_table_rows (déjà testé) : pas de
    test dédié, à vérifier manuellement contre le vrai PDF lors de Task 6."""
    import pdfplumber

    response = requests.get(url, headers={"User-Agent": USER_AGENT}, timeout=60)
    response.raise_for_status()
    all_feiras: list[dict] = []
    with pdfplumber.open(io.BytesIO(response.content)) as pdf:
        for page in pdf.pages:
            table = page.extract_table()
            if table:
                all_feiras.extend(parse_feiras_table_rows(table))
    return all_feiras
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_feiras.py -v
```

Expected: `6 passed`.

- [ ] **Step 5: Vérification manuelle de `fetch_and_parse_feiras_pdf` contre le vrai PDF**

```bash
python3 -c "
from sourcing.feiras import fetch_and_parse_feiras_pdf
feiras = fetch_and_parse_feiras_pdf()
print(f'Total feiras parsées: {len(feiras)}')
santa_teresa = [f for f in feiras if 'SANTA TERESA' in f['bairro'].upper()]
print('Santa Teresa:', santa_teresa)
"
```

Expected: un nombre de feiras proche de 165 (total actif connu), avec au moins une entrée pour Santa Teresa (Rua Terezina, vendredi — confirmé en conception). Si le nombre est très inférieur, l'extraction de tableau du PDF a probablement raté des pages — inspecter `pdf.pages` individuellement.

- [ ] **Step 6: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
git add pipeline/sourcing/feiras.py pipeline/tests/test_feiras.py
git commit -m "Add feiras livres PDF parsing and Nominatim geocoding"
```

---

### Task 6: Orchestration du pipeline complet

**Files:**
- Create: `pipeline/sourcing/pipeline.py`
- Test: `pipeline/tests/test_pipeline.py`

**Interfaces:**
- Consumes: `Place` (Task 1), `deduplicate_places` (Task 2), `query_overture_places` (Task 3), `query_iphan_heritage_sites` (Task 4), `fetch_and_parse_feiras_pdf` + `feiras_to_places` (Task 5).
- Produces: `run_pipeline(output_path: Path) -> list[Place]`, `SANTA_TERESA_LAPA_BBOX: tuple[float, float, float, float]`.

- [ ] **Step 1: Écrire les tests qui échouent**

Créer `pipeline/tests/test_pipeline.py` :

```python
import json
from unittest.mock import patch

from sourcing.models import Place
from sourcing.pipeline import run_pipeline


def test_run_pipeline_combines_and_writes_output(tmp_path):
    overture_result = [
        Place(name="Escadaria Selarón", lat=-22.9147, lon=-43.1806, category="landmark_and_historical_building", source="overture")
    ]
    wikidata_result = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="heritage_site", source="wikidata", wikidata_qid="Q1798512")
    ]
    feiras_places_result = [
        Place(name="Feira de Santa Teresa (Sexta-Feira)", lat=-22.9350, lon=-43.1900, category="recurring_cultural_event", source="feiras_registry")
    ]

    output_file = tmp_path / "places.json"

    with patch("sourcing.pipeline.query_overture_places", return_value=overture_result), \
         patch("sourcing.pipeline.query_iphan_heritage_sites", return_value=wikidata_result), \
         patch("sourcing.pipeline.fetch_and_parse_feiras_pdf", return_value=[{"descricao": "x", "bairro": "y", "dia_semana": "z"}]), \
         patch("sourcing.pipeline.feiras_to_places", return_value=feiras_places_result):
        result = run_pipeline(output_file)

    assert len(result) == 3
    saved = json.loads(output_file.read_text(encoding="utf-8"))
    assert len(saved) == 3
    names = {place["name"] for place in saved}
    assert names == {"Escadaria Selarón", "Museu Nacional", "Feira de Santa Teresa (Sexta-Feira)"}


def test_run_pipeline_deduplicates_across_sources(tmp_path):
    overture_result = [Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture")]
    wikidata_result = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="heritage_site", source="wikidata", wikidata_qid="Q1798512")
    ]

    output_file = tmp_path / "places.json"

    with patch("sourcing.pipeline.query_overture_places", return_value=overture_result), \
         patch("sourcing.pipeline.query_iphan_heritage_sites", return_value=wikidata_result), \
         patch("sourcing.pipeline.fetch_and_parse_feiras_pdf", return_value=[]), \
         patch("sourcing.pipeline.feiras_to_places", return_value=[]):
        result = run_pipeline(output_file)

    assert len(result) == 1
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_pipeline.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.pipeline'`.

- [ ] **Step 3: Implémenter `sourcing/pipeline.py`**

```python
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
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_pipeline.py -v
```

Expected: `2 passed`.

- [ ] **Step 5: Lancer la suite complète**

```bash
pytest -v
```

Expected: tous les tests passent (le test d'intégration Overture de Task 3 inclus, réseau requis).

- [ ] **Step 6: Exécution réelle du pipeline complet**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/pipeline
python -m sourcing.pipeline
```

Expected: un fichier `places.json` créé avec une liste de lieux candidats pour Santa Teresa/Lapa. Vérifier manuellement : présence de l'Escadaria Selarón, du Museu da Chácara do Céu, d'au moins une feira — et absence du Santuário do Zé Pelintra (attendu : Overture le catégorise en `business_advertising`, hors allowlist — confirme qu'il faudra l'ajouter manuellement à la liste finale, comme documenté dans le spec).

- [ ] **Step 7: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide
echo "pipeline/places.json" >> .gitignore
echo "pipeline/.venv/" >> .gitignore
git add pipeline/sourcing/pipeline.py pipeline/tests/test_pipeline.py .gitignore
git commit -m "Add pipeline orchestration: combine sources, dedupe, write places.json"
```

---

## Self-Review

**Spec coverage** : les 3 sources automatisées du spec (Overture Maps, Wikidata/IPHAN, registre feiras) sont chacune couvertes par un module + tests (Tasks 3-5). La déduplication multi-sources documentée dans l'annexe technique du spec (normalisation de nom, proximité ≤100m, priorité au QID Wikidata, pas de fusion automatique en cas d'ambiguïté) est implémentée et testée en Task 2. Les catégories Overture retenues correspondent exactement à celles listées dans le spec, avec exclusion explicite de `business_advertising`/`bar`/`restaurant`. La sortie JSON correspond au besoin de "liste de candidats pour sélection manuelle des 25 lieux" — la sélection finale elle-même reste, comme prévu, une étape humaine hors du code.

**Placeholders** : aucun TBD/TODO — chaque étape contient du code complet et exécutable.

**Cohérence des types/signatures** : `Place` (Task 1) est utilisé de façon cohérente dans tous les modules suivants (mêmes noms de champs). `deduplicate_places` (Task 2) est appelée avec la signature exacte définie. `query_overture_places`, `query_iphan_heritage_sites`, `fetch_and_parse_feiras_pdf`, `feiras_to_places` sont importés dans `pipeline.py` avec les noms exacts définis dans leurs tasks respectives, et mockés sous ces mêmes noms dans les tests de Task 6.

---

## Execution Handoff

Plan complet et sauvegardé dans `docs/superpowers/plans/2026-07-21-lieu-sourcing-pipeline.md`. Deux options d'exécution :

**1. Subagent-Driven (recommandé)** — je dispatche un sous-agent frais par tâche, avec revue entre chaque tâche, itération rapide.

**2. Exécution en ligne** — j'exécute les tâches dans cette session avec executing-plans, exécution par lot avec points de contrôle.

Laquelle des deux options ?
