import type { Place, NarrationStatus, AudioAvailability } from "./types";
import { MOCK_PLACES } from "./types";
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";
import { getOfflineDownloadSummary } from "./downloadManager";
import { getCachedPlace } from "./offlineStore";
import { isOnline } from "../utils/network";

export interface PlacesRepository {
  listNearby(): Promise<Place[]>;
  getById(id: string): Promise<Place | undefined>;
  search(query: string): Promise<Place[]>;
  downloadedCount(): Promise<number>;
  getAudioUrl(placeId: string, language: Locale): Promise<AudioAvailability>;
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

  // No real backend behind the mock repository, so no real audio to point
  // to -- honest "unavailable" rather than pretending.
  async getAudioUrl(): Promise<AudioAvailability> {
    return { state: "unavailable" };
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
  try {
    const res = await fetch(`${API_BASE_URL}${path}`);
    let body: T | null = null;
    try {
      body = (await res.json()) as T;
    } catch {
      body = null;
    }
    return { status: res.status, body };
  } catch {
    return { status: 0, body: null };
  }
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

  // Online: unchanged network call. Offline: falls back to the cached place
  // (narration text + fields) saved by downloadManager.downloadCity, rather
  // than returning undefined -- a device with no signal must still be able
  // to open a place it already downloaded.
  async getById(id: string): Promise<Place | undefined> {
    if (await isOnline()) {
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

      if (status !== 0) {
        // 202 "not yet published", or 404 "no script for this place/language" --
        // either way the place itself may still exist; /places/:id doesn't
        // return place fields in those cases, so fall back to the list to at
        // least show name/category/position while narration is unavailable.
        const narrationStatus: NarrationStatus = status === 202 ? "pending" : "unavailable";
        const basics = await this.listNearby();
        const match = basics.find((p) => p.id === id);
        if (match) return { ...match, narrationStatus };
      }
      // status === 0 (the request itself failed despite isOnline() saying
      // yes) falls through to the offline cache below, same as genuinely
      // offline.
    }

    const cached = await getCachedPlace(id);
    if (!cached) return undefined;
    return {
      id: cached.id,
      name: cached.name,
      category: cached.category,
      lat: cached.lat,
      lon: cached.lon,
      city: CURRENT_BACKEND_CITY,
      body: cached.body,
      groundedSourceCount: cached.body ? 1 : 0,
      narrationStatus: "ready",
    };
  }

  async downloadedCount(): Promise<number> {
    const summary = await getOfflineDownloadSummary();
    return summary?.placeCount ?? 0;
  }

  // Online: unchanged streamed URL, real presigned S3 URL. Offline: the
  // locally cached audio file (downloadManager.downloadCity already wrote it
  // and recorded its URI in offlineStore) if this place was downloaded, else
  // "unavailable" -- never claims a place is playable offline when it wasn't
  // actually downloaded.
  async getAudioUrl(placeId: string, language: Locale): Promise<AudioAvailability> {
    if (await isOnline()) {
      const { status, body } = await fetchJson<{ url?: string; timestamps_url?: string }>(
        `/places/${encodeURIComponent(placeId)}/audio?language=${language}`,
      );
      if (status === 200 && body?.url) {
        return { state: "ready", url: body.url, timestampsUrl: body.timestamps_url };
      }
      if (status === 202) {
        return { state: "pending" };
      }
      if (status !== 0) {
        return { state: "unavailable" };
      }
      // status === 0: fall through to the offline cache rather than
      // reporting "unavailable" for a place that IS downloaded.
    }

    const cached = await getCachedPlace(placeId);
    if (cached?.audioLocalUri) {
      return { state: "ready", url: cached.audioLocalUri };
    }
    return { state: "unavailable" };
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
