import AsyncStorage from "@react-native-async-storage/async-storage";
import type { Locale } from "../i18n/dictionary";
import type { Place } from "./types";
import { API_BASE_URL } from "../config";

export type DownloadFileRef = {
  placeId: string;
  kind: "metadata" | "audio";
  locale?: Locale;
  path: string;
};

/**
 * Pure function: given the places belonging to a city and the single language the
 * user wants offline, compute exactly which files a "download this city" action
 * needs to fetch. Deliberately fetches only `locale`, never all four, per the
 * design spec (offline packs are per-language, not per-language-times-four).
 */
export function planCityDownload(
  places: Place[],
  city: string,
  locale: Locale,
): DownloadFileRef[] {
  const cityPlaces = places.filter((p) => p.city === city);

  const files: DownloadFileRef[] = [];
  for (const place of cityPlaces) {
    files.push({ placeId: place.id, kind: "metadata", path: `${place.id}/metadata.json` });
    files.push({
      placeId: place.id,
      kind: "audio",
      locale,
      path: `${place.id}/audio-${locale}.mp3`,
    });
  }
  return files;
}

export function estimateDownloadSizeBytes(files: DownloadFileRef[]): number {
  const METADATA_BYTES = 4_000;
  const AUDIO_BYTES_PER_FILE = 1_800_000;
  return files.reduce(
    (total, f) => total + (f.kind === "audio" ? AUDIO_BYTES_PER_FILE : METADATA_BYTES),
    0,
  );
}

// --- real backend wiring -----------------------------------------------

type ManifestPlace = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  narration: string;
  source: string;
  source_richness: string;
  audio_url: string;
};

type ManifestResponse = {
  city: string;
  language: string;
  places: ManifestPlace[];
};

// The only city the backend's manifest route currently serves (see
// rioCitySlug in manifest_handler.go).
export const RIO_CITY_SLUG = "rio";

/**
 * Calls GET /cities/:city/manifest?language=xx and maps the result to
 * `Place[]` so it can go straight through `planCityDownload`/
 * `estimateDownloadSizeBytes` like any other place list. Every place the
 * manifest returns already has a published narration and a ready audio
 * file — that's the whole point of the route (see manifest_handler.go) — so
 * they're all mapped as narrationStatus "ready".
 *
 * Returns an empty array (not an error) on any non-200 response or network
 * failure — a city with nothing published yet, or a briefly unreachable
 * backend, should read as "nothing downloadable right now", not crash the
 * onboarding flow.
 */
export async function fetchCityManifest(
  citySlug: string,
  language: Locale,
): Promise<Place[]> {
  try {
    const res = await fetch(
      `${API_BASE_URL}/cities/${encodeURIComponent(citySlug)}/manifest?language=${language}`,
    );
    if (res.status !== 200) return [];
    const body = (await res.json()) as ManifestResponse;
    return body.places.map((p) => ({
      id: p.id,
      name: p.name,
      category: p.category,
      lat: p.lat,
      lon: p.lon,
      city: "Rio de Janeiro",
      body: p.narration,
      groundedSourceCount: p.source ? 1 : 0,
      narrationStatus: "ready" as const,
    }));
  } catch {
    return [];
  }
}

export type OfflineDownloadSummary = {
  city: string;
  language: Locale;
  placeCount: number;
  approxSizeBytes: number;
  downloadedAt: string;
};

const STORAGE_KEY = "memoria-carioca:offline-download";

/**
 * Fetches the real manifest, computes its size the same way the rest of the
 * app estimates download size, and persists a summary on-device — this is
 * what makes "42 lieux · 184 Mo" (and Settings' delete button, eventually) a
 * real number instead of copy hardcoded into the dictionary.
 *
 * Note: this downloads *metadata* (which places, their narration text, an
 * estimated size) — it does not yet fetch and store the actual audio bytes
 * for offline playback. See the mobile app spec's known-gaps section.
 */
export async function downloadCity(
  citySlug: string,
  cityDisplayName: string,
  language: Locale,
): Promise<OfflineDownloadSummary> {
  const places = await fetchCityManifest(citySlug, language);
  const files = planCityDownload(places, cityDisplayName, language);
  const summary: OfflineDownloadSummary = {
    city: cityDisplayName,
    language,
    placeCount: places.length,
    approxSizeBytes: estimateDownloadSizeBytes(files),
    downloadedAt: new Date().toISOString(),
  };
  await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(summary));
  return summary;
}

export async function getOfflineDownloadSummary(): Promise<OfflineDownloadSummary | null> {
  const raw = await AsyncStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as OfflineDownloadSummary;
  } catch {
    return null;
  }
}

export async function clearOfflineDownload(): Promise<void> {
  await AsyncStorage.removeItem(STORAGE_KEY);
}

const SIZE_UNIT: Record<Locale, string> = { fr: "Mo", en: "MB", pt: "MB", es: "MB" };

export function formatApproxSize(bytes: number, language: Locale): string {
  const mb = bytes / 1_000_000;
  const value = mb < 10 ? mb.toFixed(1) : Math.round(mb);
  return `${value} ${SIZE_UNIT[language]}`;
}
