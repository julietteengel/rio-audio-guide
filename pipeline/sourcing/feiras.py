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
    test dédié, à vérifier manuellement contre le vrai PDF lors de Task 6."""
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
