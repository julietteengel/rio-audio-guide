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
