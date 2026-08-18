import type { Place, NarrationStatus } from "./types";
import { MOCK_PLACES } from "./types";
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";
import { getOfflineDownloadSummary } from "./downloadManager";

export interface PlacesRepository {
  listNearby(): Promise<Place[]>;
  getById(id: string): Promise<Place | undefined>;
  search(query: string): Promise<Place[]>;
  downloadedCount(): Promise<number>;
}

/**
 * Renders every screen against the same fixed example (Cristo Redentor) used
 * throughout the approved design prototype. Useful as an offline/demo
 * fallback (see `placesRepository` below) when there's no backend reachable.
 */
export class MockPlacesRepository implements PlacesRepository {
  async listNearby(): Promise<Place[]> {
    return MOCK_PLACES;
  }

  async getById(id: string): Promise<Place | undefined> {
    return MOCK_PLACES.find((p) => p.id === id);
  }

  async search(query: string): Promise<Place[]> {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return MOCK_PLACES.filter((p) => p.name.toLowerCase().includes(q));
  }

  async downloadedCount(): Promise<number> {
    return MOCK_PLACES.length;
  }
}

// --- real HTTP-backed repository -------------------------------------------

// Shapes returned by the backend (internal/adapters/http on the `backend`
// branch) — kept in sync by hand, there's no shared schema between the two
// codebases yet.
type PlaceListItem = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
};

type PlaceDetailReady = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  language: string;
  narration: string;
  source: string;
  source_richness: string;
};

type PlaceDetailNotReady = { status: string };

// The backend's current bounding box only ever covers Rio de Janeiro (see
// `rioMinLat`/`rioMaxLat` in places_handler.go) — every place this repository
// returns genuinely is in Rio right now, this isn't a placeholder.
const CURRENT_BACKEND_CITY = "Rio de Janeiro";

function toListPlace(item: PlaceListItem): Place {
  return {
    id: item.id,
    name: item.name,
    category: item.category,
    lat: item.lat,
    lon: item.lon,
    city: CURRENT_BACKEND_CITY,
    body: "",
    groundedSourceCount: 0,
    // A list entry hasn't fetched narration yet — not a claim that none
    // exists. getById() resolves the real status when a screen needs it.
    narrationStatus: "pending",
  };
}

async function fetchJson<T>(path: string): Promise<{ status: number; body: T | null }> {
  const res = await fetch(`${API_BASE_URL}${path}`);
  let body: T | null = null;
  try {
    body = (await res.json()) as T;
  } catch {
    body = null;
  }
  return { status: res.status, body };
}

export class HttpPlacesRepository implements PlacesRepository {
  constructor(private getLocale: () => Locale) {}

  async listNearby(): Promise<Place[]> {
    const { status, body } = await fetchJson<PlaceListItem[]>("/places");
    if (status !== 200 || !body) return [];
    return body.map(toListPlace);
  }

  async search(query: string): Promise<Place[]> {
    const q = query.trim();
    if (!q) return [];
    const { status, body } = await fetchJson<PlaceListItem[]>(
      `/places?q=${encodeURIComponent(q)}`,
    );
    if (status !== 200 || !body) return [];
    return body.map(toListPlace);
  }

  async getById(id: string): Promise<Place | undefined> {
    const language = this.getLocale();
    const { status, body } = await fetchJson<PlaceDetailReady | PlaceDetailNotReady>(
      `/places/${encodeURIComponent(id)}?language=${language}`,
    );

    if (status === 200 && body && "narration" in body) {
      return {
        id: body.id,
        name: body.name,
        category: body.category,
        lat: body.lat,
        lon: body.lon,
        city: CURRENT_BACKEND_CITY,
        body: body.narration,
        groundedSourceCount: body.source ? 1 : 0,
        narrationStatus: "ready",
      };
    }

    // 202 "not yet published", or 404 "no script for this place/language" —
    // either way the place itself may still exist; /places/:id doesn't
    // return place fields in those cases, so fall back to the list to at
    // least show name/category/position while narration is unavailable.
    const narrationStatus: NarrationStatus = status === 202 ? "pending" : "unavailable";
    const basics = await this.listNearby();
    const match = basics.find((p) => p.id === id);
    if (!match) return undefined;
    return { ...match, narrationStatus };
  }

  async downloadedCount(): Promise<number> {
    const summary = await getOfflineDownloadSummary();
    return summary?.placeCount ?? 0;
  }
}

// Set EXPO_PUBLIC_USE_MOCK_DATA=1 (e.g. in a local .env file) to force the
// static mock — useful for UI work with no backend running. Defaults to the
// real backend now that place-list/detail/search routes exist.
const useMock = process.env.EXPO_PUBLIC_USE_MOCK_DATA === "1";

let currentLocale: Locale = "en";
export function setPlacesRepositoryLocale(locale: Locale): void {
  currentLocale = locale;
}

export const placesRepository: PlacesRepository = useMock
  ? new MockPlacesRepository()
  : new HttpPlacesRepository(() => currentLocale);
