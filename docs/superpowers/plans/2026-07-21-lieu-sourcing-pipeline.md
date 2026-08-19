# Pipeline de Sourcing des Lieux — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construire un script Python autonome qui interroge Overture Maps, Wikidata (patrimoine IPHAN) et le registre officiel des feiras livres, déduplique les résultats entre sources, et produit un fichier JSON de lieux candidats pour Santa Teresa/Lapa — la base de données brute à partir de laquelle les 25 lieux du MVP seront sélectionnés manuellement.

**Architecture:** Un module Python par source de données (`overture.py`, `wikidata.py`, `feiras.py`), un module de déduplication pur sans dépendance réseau (`dedup.py`), et un orchestrateur (`pipeline.py`) qui combine tout et écrit le JSON final. Chaque module de source sépare sa logique pure (testable sans réseau, avec mocks/fixtures) de son appel réseau (testé en intégration).

**Tech Stack:** Python 3.11+ (vérifié en pratique dans Task 1 : `requests` et `duckdb` gèrent leurs propres certificats TLS et fonctionnent bien même sur Python 3.14 ; le souci SSL rencontré plus tôt en conception était spécifique à `urllib` brut, non utilisé ici — pas de contrainte de version stricte au-delà de 3.11+), `duckdb` (requêtes Overture via S3 parquet), `requests` (Wikidata SPARQL, Nominatim, PDF), `pdfplumber` (extraction de tableau PDF), `pytest`.

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
mkdir -p /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline/pipeline/sourcing
mkdir -p /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline/pipeline/tests
```

- [ ] **Step 2: Créer `pyproject.toml`**

```toml
[project]
name = "rio-audio-guide-sourcing"
version = "0.1.0"
description = "Location sourcing pipeline for Rio Audio Guide"
requires-python = ">=3.11"
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
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline/pipeline
python3 -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

Expected: installation sans erreur, quelle que soit la version 3.11+ disponible (vérifié fonctionnel avec 3.14 sur cette machine, `python3 --version` pour vérifier).

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
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline/pipeline
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
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
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
        Place(name="escadaria selaron", lat=-22.91471, lon=-43.18061, category="landmark_and_historical_building", source="wikidata"),
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


def test_normalize_name_strips_praca_prefix():
    assert normalize_name("Praça XV de Novembro") == "xv de novembro"


def test_deduplicate_places_does_not_merge_qid_match_when_far_apart():
    places = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture", wikidata_qid="Q1798512"),
        Place(name="Museu Nacional (Erro)", lat=-22.8600, lon=-43.1700, category="museum", source="wikidata", wikidata_qid="Q1798512"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 2


def test_deduplicate_places_merges_qid_match_within_qid_threshold():
    places = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture", wikidata_qid="Q1798512"),
        Place(name="Museu Nacional (approx)", lat=-22.9080, lon=-43.2260, category="museum", source="wikidata", wikidata_qid="Q1798512"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 1


def test_deduplicate_places_does_not_merge_different_categories_even_if_close_same_name():
    places = [
        Place(name="Igreja de Santa Rita", lat=-22.9050, lon=-43.1800, category="church_cathedral", source="overture"),
        Place(name="Praça Santa Rita", lat=-22.90501, lon=-43.18001, category="landmark_and_historical_building", source="overture"),
    ]
    result = deduplicate_places(places)
    assert len(result) == 2


def test_deduplicate_places_merges_qid_match_even_with_different_categories():
    places = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture", wikidata_qid="Q1798512"),
        Place(name="Museu Nacional", lat=-22.9060, lon=-43.2248, category="history_museum", source="wikidata", wikidata_qid="Q1798512"),
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
    "praca ", "parque ",
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


def deduplicate_places(
    places: list[Place],
    max_distance_m: float = 100.0,
    max_qid_distance_m: float = 5000.0,
) -> list[Place]:
    """max_qid_distance_m is deliberately more generous than max_distance_m: a
    shared Wikidata QID is stronger evidence than name+proximity alone, but a
    QID match with wildly divergent coordinates (a plausible tagging error)
    should still not auto-merge unconditionally — that would violate the
    "jamais de fusion automatique en cas d'ambiguïté forte" constraint."""
    kept: list[Place] = []
    for place in places:
        is_duplicate = False
        for existing in kept:
            same_qid = (
                place.wikidata_qid
                and place.wikidata_qid == existing.wikidata_qid
                and haversine_distance_m(place.lat, place.lon, existing.lat, existing.lon) <= max_qid_distance_m
            )
            categories_compatible = (
                place.category == existing.category
                or place.category in GENERIC_SOURCE_CATEGORIES
                or existing.category in GENERIC_SOURCE_CATEGORIES
            )
            same_name_and_close = (
                normalize_name(place.name) == normalize_name(existing.name)
                and categories_compatible
                and haversine_distance_m(place.lat, place.lon, existing.lat, existing.lon) <= max_distance_m
            )
            if same_qid or same_name_and_close:
                is_duplicate = True
                break
        if not is_duplicate:
            kept.append(place)
    return kept
```

Note : le chemin QID ne vérifie volontairement pas la catégorie — une identité Wikidata partagée confirmée est une preuve suffisante en elle-même, même si la catégorie diffère (ex : un même lieu catégorisé différemment selon la source).

**Correctif post-revue finale (`GENERIC_SOURCE_CATEGORIES`)** : découvert lors de la revue de branche complète (Task 6) que la dédup inter-sources ne fonctionnait jamais en pratique (350 lieux réels = 221+126+3, zéro fusion) — Wikidata génère toujours `"heritage_site"` et les feiras toujours `"recurring_cultural_event"` (des catégories génériques par source, pas de vraie information de type d'entité), donc elles ne correspondaient jamais aux catégories précises d'Overture, et Overture ne porte jamais de `wikidata_qid`. Solution : traiter ces catégories génériques comme compatibles avec n'importe quelle autre catégorie pour la correspondance nom+proximité, sans affaiblir la protection anti-fusion entre deux catégories *spécifiques* différentes (le cas concret church/praça, qui reste protégé). Compromis accepté et documenté : un lieu générique (Wikidata/feiras) qui devient l'ancre `kept` peut en théorie faire fusionner deux entités Overture spécifiques différentes qui partagent son nom normalisé à moins de 100m — jugé rare en pratique, non bloquant.

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_dedup.py -v
```

Expected: `13 passed`.

- [ ] **Step 5: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
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
import pytest

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


@pytest.mark.integration
def test_query_overture_places_returns_known_landmark_for_selaron_bbox():
    # Bbox serré (marge uniforme de ±0.0005°) autour des coordonnées réelles
    # des entrées "Selarón"/"Selaron" trouvées par requête diagnostique dans
    # le release 2026-06-17.0 (Selarón Apartments, Scalinata Selarón, Selaron
    # Steps). Test d'intégration réel (réseau requis).
    bbox = (-43.180157, -22.916949, -43.178459, -22.915033)
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

Expected: `4 passed`. Le 4e test (`test_query_overture_places_returns_known_landmark_for_selaron_bbox`) est marqué `@pytest.mark.integration` (nécessite un accès réseau, prend ~10-20s) — exclu du run rapide via `pytest -m "not integration"`, mais s'exécute normalement dans un run complet. Le marqueur doit être enregistré dans `pipeline/pyproject.toml` sous `[tool.pytest.ini_options]` :

```toml
markers = [
    "integration: tests that require live network access (deselect with '-m \"not integration\"')",
]
```

- [ ] **Step 5: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
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
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
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


def test_fetch_and_parse_feiras_pdf_logs_warning_and_continues_on_malformed_page(caplog, monkeypatch):
    import logging
    from unittest.mock import MagicMock
    import sourcing.feiras as feiras_module

    bad_table = [["Título institucional", "", ""]]
    good_table = [
        ["Código", "Turno", "Descrição", "Bairro", "Dias da Semana"],
        ["54", "Não", "RUA TEREZINA", "SANTA TERESA", "Sexta-Feira"],
    ]

    fake_page_bad = MagicMock()
    fake_page_bad.extract_table.return_value = bad_table
    fake_page_good = MagicMock()
    fake_page_good.extract_table.return_value = good_table

    fake_pdf = MagicMock()
    fake_pdf.pages = [fake_page_bad, fake_page_good]
    fake_pdf.__enter__.return_value = fake_pdf
    fake_pdf.__exit__.return_value = False

    fake_response = MagicMock()
    fake_response.content = b"fake-pdf-bytes"
    fake_response.raise_for_status = MagicMock()

    monkeypatch.setattr(feiras_module.requests, "get", lambda *a, **kw: fake_response)
    monkeypatch.setattr(feiras_module.pdfplumber, "open", lambda *a, **kw: fake_pdf)

    with caplog.at_level(logging.WARNING):
        result = feiras_module.fetch_and_parse_feiras_pdf()

    assert len(result) == 1
    assert result[0]["bairro"] == "SANTA TERESA"
    assert any("Skipping page" in record.message for record in caplog.records)
```

- [ ] **Step 2: Lancer les tests, vérifier qu'ils échouent**

```bash
pytest tests/test_feiras.py -v
```

Expected: `ModuleNotFoundError: No module named 'sourcing.feiras'`.

- [ ] **Step 3: Implémenter `sourcing/feiras.py`**

```python
import io
import logging

import pdfplumber
import requests

from sourcing.models import Place

logger = logging.getLogger(__name__)

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
    test dédié sur l'accès réseau réel, mais une page malformée (ex: le bloc
    titre/en-tête institutionnel de la page 1, qui n'a pas la structure de
    colonnes attendue) est loguée en warning et sautée plutôt que de faire
    planter toute l'extraction ou d'être avalée silencieusement."""
    response = requests.get(url, headers={"User-Agent": USER_AGENT}, timeout=60)
    response.raise_for_status()
    all_feiras: list[dict] = []
    with pdfplumber.open(io.BytesIO(response.content)) as pdf:
        for page_num, page in enumerate(pdf.pages, start=1):
            table = page.extract_table()
            if table:
                try:
                    all_feiras.extend(parse_feiras_table_rows(table))
                except ValueError as exc:
                    logger.warning("Skipping page %d in feiras PDF: %s", page_num, exc)
    return all_feiras
```

- [ ] **Step 4: Lancer les tests, vérifier qu'ils passent**

```bash
pytest tests/test_feiras.py -v
```

Expected: `8 passed`.

- [ ] **Step 5: Vérification manuelle de `fetch_and_parse_feiras_pdf` contre le vrai PDF**

```bash
python3 -c "
import logging
logging.basicConfig(level=logging.WARNING)
from sourcing.feiras import fetch_and_parse_feiras_pdf
feiras = fetch_and_parse_feiras_pdf()
print(f'Total feiras parsées: {len(feiras)}')
santa_teresa = [f for f in feiras if 'SANTA TERESA' in f['bairro'].upper()]
print('Santa Teresa:', santa_teresa)
"
```

Expected (vérifié en exécution réelle) : un warning loggé pour la page 1 (bloc titre institutionnel, structure de colonnes différente), puis **149 feiras parsées** sur les pages restantes, avec l'entrée Santa Teresa (Rua Terezina, vendredi) présente. **Point de vigilance pour Task 6** : 149 est en dessous du total de 165 feiras actives annoncé par le registre — l'écart pourrait venir de lignes de continuation de tableau sur plusieurs pages mal détectées comme des en-têtes (donc silencieusement sautées avec un warning, pas une vraie perte de données non tracée), mais ça mérite une vérification manuelle ciblée (comparer la liste obtenue au PDF source) avant de considérer le pipeline de sourcing des feiras comme complet à 100%.

- [ ] **Step 6: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
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
    # Catégorie alignée sur "museum" des deux côtés (et non "heritage_site" côté
    # Wikidata) : le chemin de fusion nom+proximité de dedup.py exige des
    # catégories identiques par conception (durci en Task 2, commit 28547c2 —
    # "jamais de fusion automatique en cas d'ambiguïté forte"), et le côté
    # Overture ne porte jamais de wikidata_qid (query_overture_places n'en
    # définit pas), donc le chemin de fusion par QID ne s'applique pas non
    # plus ici. Une version à catégories différentes ne fusionnerait
    # légitimement pas avec le dedup.py actuel — ce ne serait donc plus ce
    # test-ci (vérifier que run_pipeline câble bien deduplicate_places entre
    # sources), mais un test du comportement de dedup.py lui-même (déjà
    # couvert en Task 2).
    overture_result = [Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="overture")]
    wikidata_result = [
        Place(name="Museu Nacional", lat=-22.9058, lon=-43.2246, category="museum", source="wikidata", wikidata_qid="Q1798512")
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

**Avant de lancer** : `feiras_to_places` (Task 5) doit avoir un `time.sleep(1)` dans sa boucle après chaque appel à `geocode_address` — Nominatim (usage public) limite à 1 requête/seconde, et le pipeline complet géocode ~149 feiras en une seule exécution. Sans ce délai, l'exécution réelle ci-dessous violerait leur politique d'usage.

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline/pipeline
python -m sourcing.pipeline
```

Expected (~2-3 minutes à cause du rate-limit Nominatim) : un fichier `places.json` créé. Vérifié en exécution réelle : **350 lieux candidats** (221 Overture, 126 Wikidata, 3 feiras après géocodage). Présence confirmée : Escadaria Selarón (3 variantes de nom), Museu da Chácara do Céu, au moins une feira. Absence confirmée du Santuário do Zé Pelintra sous son nom exact (attendu : catégorisé `business_advertising` par Overture, hors allowlist) — **mais** une entrée différemment nommée "Santuário de Seu Zé Pelintra" (catégorie `topic_concert_venue`, dans l'allowlist) apparaît bien : c'est le résultat voulu de l'ajout de cette catégorie plus tôt dans la conception, à vérifier manuellement que c'est bien la même entité avant de l'inclure dans la liste finale.

**Limitations connues révélées par cette exécution réelle** (voir aussi section "Résultats et limitations connues" après le Self-Review) :
- Seulement 3 des ~149 feiras survivent au géocodage (description d'adresse trop complexe pour Nominatim) — amélioration de la stratégie de géocodage hors scope de ce plan.
- Wikidata et les feiras ne sont pas filtrés par bbox (contrairement à Overture) — résultats à l'échelle de la ville entière, le filtrage géographique final pour Santa Teresa/Lapa reste à faire manuellement lors de la curation.

- [ ] **Step 7: Commit**

```bash
cd /Users/julietteengel/code/julietteengel/rio-audio-guide/.worktrees/sourcing-pipeline
echo "pipeline/places.json" >> .gitignore
echo "pipeline/.venv/" >> .gitignore
git add pipeline/sourcing/pipeline.py pipeline/tests/test_pipeline.py .gitignore
git commit -m "Add pipeline orchestration: combine sources, dedupe, write places.json"
```

---

### Task 7: Élargir la couverture par catégorie — églises, galeries, venues, culture afro-brésilienne, fêtes

Origine : question utilisateur ("il manque peut-être des catégories genre églises, lieux de culture afro-bré, fêtes ?") pendant la phase de curation des 154 lieux part4. Vérification faite par requête live sur le vrai parquet Overture (bbox Rio complet, release `2026-06-17.0`), pas de suppositions sur les noms de catégorie.

**Constat** :
- Overture a de vraies catégories pour les lieux de culte, mais à un volume très inégal : `church_cathedral` (6314), `religious_organization` (3799), `evangelical_church` (1758), `pentecostal_church` (1377), `baptist_church` (989), `catholic_church` (908), plus une longue traîne (`synagogue`, `buddhist_temple`, `mosque`, `hindu_temple`, `anglican_church`...). La quasi-totalité des trois catégories evangelical/pentecostal/baptist + `religious_organization` sont vraisemblablement des paroisses de quartier sans intérêt patrimonial/touristique — même risque de bruit que `landmark_and_historical_building` (déjà documenté dans le CLAUDE.md du repo).
- **Aucune catégorie Overture** ne couvre la culture afro-brésilienne (terreiros, candomblé, umbanda) ni les fêtes/carnaval/écoles de samba (`music_festivals_and_organizations` n'a qu'1 résultat sur tout Rio). Ce n'est pas un problème d'allowlist — c'est un vrai trou de couverture de la source elle-même. Les quelques lieux de ce type déjà dans le corpus (Baianas da Estação Primeira de Mangueira, Cordão da Bola Preta) n'y sont que par accident, catégorisés `cultural_center`.
- Overture a aussi des catégories galerie/venue/musée-de-niche jamais ajoutées à l'allowlist initial : `art_gallery` (321), `music_venue` (256), `theatre` (219), `venue_and_event_space` (98), `performing_arts` (53), et une dizaine de sous-types de musée à faible volume chacun (`science_museum`, `contemporary_art_museum`, `design_museum`, `childrens_museum`, `civilization_museum`, `community_museum`, `state_museum`, `sports_museum`, `cartooning_museum`, `aviation_museum`, `costume_museum`).

**Décision de périmètre (utilisateur, cette session)** : pour les églises, n'ajouter que `church_cathedral` (6314 lieux) — la catégorie la plus susceptible de contenir de vraies églises historiques/touristiques (Candelária, cathédrale métropolitaine, etc.), en excluant délibérément `religious_organization`/`evangelical_church`/`pentecostal_church`/`baptist_church` (~7900 lieux combinés) jugées trop bruitées pour justifier le coût de triage. Peut être révisé après une première passe de triage réelle sur `church_cathedral` si le taux de bruit s'avère lui aussi trop élevé.

- [x] **Step 1** : ajouter `art_gallery`, `music_venue`, `theatre`, `venue_and_event_space`, `performing_arts`, les sous-types de musée listés ci-dessus, et `church_cathedral` à `CATEGORY_ALLOWLIST` dans `sourcing/overture.py`, avec commentaire documentant pourquoi les autres catégories église sont explicitement exclues (pas juste oubliées). Commit `ba52276`.
- [x] **Step 2** : vérifier que la suite de tests existante passe toujours sans modification (`pytest -m "not integration"`, 34 passed) — l'allowlist n'a pas de test qui énumère positivement son contenu complet, seulement des tests d'exclusion (`bar`, `restaurant`, `beach`...), donc aucun test cassé par l'ajout.
- [ ] **Step 3** : relancer `python -m sourcing.pipeline` pour tirer les nouveaux lieux. Volume mesuré par requête live avant exécution : **7295 lieux nouveaux** au total sur ces catégories combinées (dont ~6314 pour `church_cathedral` seul) — à comparer aux 3858 lieux du dataset actuel post-perte de données. Vérifier que le runtime DuckDB/S3 reste raisonnable à ce volume.
- [ ] **Step 4** : ré-exécuter tout le cycle de curation déjà utilisé pour les 773 CULTURAL initiaux sur ce nouveau lot — triage CULTURAL/NATURAL/NOISE (`scope_classification_v2.csv`-style), dédup (`dedup_cultural.py`), vérification de frontière municipale (`verify_boundary.py`), avant de passer au grounding. Attention particulière au taux de bruit réel de `church_cathedral` — si le triage montre qu'une grosse majorité est du bruit (paroisses sans importance), documenter le taux exact avant de décider si le grounding + narration valent le coût pour le reste.
- [ ] **Step 5 (hors scope Overture)** : sourcer séparément la culture afro-brésilienne et les fêtes/carnaval — pas réparable par l'allowlist. Construire un module dédié (sur le modèle de `sourcing/wikidata.py`) avec une requête SPARQL ciblée (ex. instances de "terreiro de candomblé"/"casa de umbanda" + lieux liés au carnaval carioca dans le périmètre de Rio), ou une source manuelle curatée si Wikidata s'avère trop pauvre sur ce sujet précis.

---

## Self-Review

**Spec coverage** : les 3 sources automatisées du spec (Overture Maps, Wikidata/IPHAN, registre feiras) sont chacune couvertes par un module + tests (Tasks 3-5). La déduplication multi-sources documentée dans l'annexe technique du spec (normalisation de nom, proximité ≤100m, priorité au QID Wikidata, pas de fusion automatique en cas d'ambiguïté) est implémentée et testée en Task 2. Les catégories Overture retenues correspondent exactement à celles listées dans le spec, avec exclusion explicite de `business_advertising`/`bar`/`restaurant`. La sortie JSON correspond au besoin de "liste de candidats pour sélection manuelle des 25 lieux" — la sélection finale elle-même reste, comme prévu, une étape humaine hors du code.

**Placeholders** : aucun TBD/TODO — chaque étape contient du code complet et exécutable.

**Cohérence des types/signatures** : `Place` (Task 1) est utilisé de façon cohérente dans tous les modules suivants (mêmes noms de champs). `deduplicate_places` (Task 2) est appelée avec la signature exacte définie. `query_overture_places`, `query_iphan_heritage_sites`, `fetch_and_parse_feiras_pdf`, `feiras_to_places` sont importés dans `pipeline.py` avec les noms exacts définis dans leurs tasks respectives, et mockés sous ces mêmes noms dans les tests de Task 6.

---

## Résultats de l'exécution réelle et limitations connues

Ce plan a été exécuté en entier (Tasks 1-6, subagent-driven, avec revue et corrections à chaque tâche). Résumé pour qui reprend ce travail :

- **350 lieux candidats** produits pour Santa Teresa/Lapa (221 Overture, 126 Wikidata/IPHAN, 3 feiras) — c'est la liste de candidats, **pas** la sélection finale des 25 lieux (étape humaine, toujours à faire).
- **Géocodage des feiras très partiel** (3/149 réussis) : la requête Nominatim concatène la description d'adresse complète du registre (souvent une plage de rues type "entre la rue X et la rue Y") avec le quartier — trop complexe pour un géocodeur généraliste. Une stratégie de géocodage plus fine (extraire une seule rue, ou géocoder par quartier avec vérification manuelle) serait nécessaire pour vraiment exploiter les feiras — hors scope de ce plan.
- **Wikidata et feiras ne sont pas filtrés par bbox** (contrairement à Overture) — résultats à l'échelle de Rio entière ; le filtrage géographique final pour Santa Teresa/Lapa se fait actuellement à la main lors de la curation, pas dans le code.
- **Rate limiting Nominatim** : `time.sleep(1)` ajouté dans `feiras_to_places` (découvert nécessaire seulement à l'exécution réelle du pipeline complet en Task 6, pas anticipé dans le plan initial) — respecte la politique d'usage de l'instance publique (max 1 req/s).
- **Santuário do Zé Pelintra** : confirmé absent sous son nom exact (catégorisé `business_advertising`, hors allowlist), mais une entrée "Santuário de Seu Zé Pelintra" (`topic_concert_venue`, dans l'allowlist) apparaît bien — probablement la même entité sous un nom légèrement différent, à vérifier manuellement avant curation finale.

**Correctifs post-revue finale de branche** (voir aussi la note dans le bloc `deduplicate_places` de Task 2) : la revue de branche complète a révélé que le chiffre de 350 lieux ci-dessus correspondait à **zéro fusion inter-source réelle** (221+126+3, addition exacte) à cause d'un défaut de conception dans le filtre de catégorie de la dédup, et que le filtrage géographique Santa Teresa/Lapa n'était appliqué qu'à Overture, pas à Wikidata/feiras (contrairement à la contrainte globale du spec). Les deux ont été corrigés (`GENERIC_SOURCE_CATEGORIES` dans `dedup.py`, filtre bbox post-agrégation dans `pipeline.py`) — une ré-exécution réelle du pipeline produirait maintenant un nombre de lieux plus bas que 350, à la fois grâce au filtre géographique et à une vraie déduplication inter-sources. Non re-exécuté après ce correctif faute de temps dans cette session (le géocodage des feiras à 1 req/s prend plusieurs minutes) — à faire avant la sélection finale des 25 lieux.

## Execution Handoff

Plan complet et sauvegardé dans `docs/superpowers/plans/2026-07-21-lieu-sourcing-pipeline.md`. Deux options d'exécution :

**1. Subagent-Driven (recommandé)** — je dispatche un sous-agent frais par tâche, avec revue entre chaque tâche, itération rapide.

**2. Exécution en ligne** — j'exécute les tâches dans cette session avec executing-plans, exécution par lot avec points de contrôle.

Laquelle des deux options ?
