import AsyncStorage from "@react-native-async-storage/async-storage";
import { Platform } from "react-native";
import { Directory, File, Paths } from "expo-file-system";
import type { Locale } from "../i18n/dictionary";
import type { Place } from "./types";
import { API_BASE_URL } from "../config";
import {
  saveCachedPlaces,
  getAllCachedPlaces,
  setCachedPlaceAudioUri,
  clearCachedPlaces,
  hasSufficientStorage,
  planResumableAudioDownloads,
} from "./offlineStore";

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

async function fetchManifestRaw(citySlug: string, language: Locale): Promise<ManifestPlace[]> {
  try {
    const res = await fetch(
      `${API_BASE_URL}/cities/${encodeURIComponent(citySlug)}/manifest?language=${language}`,
    );
    if (res.status !== 200) return [];
    const body = (await res.json()) as ManifestResponse;
    return body.places;
  } catch {
    return [];
  }
}

function manifestPlaceToPlace(p: ManifestPlace): Place {
  return {
    id: p.id,
    name: p.name,
    category: p.category,
    lat: p.lat,
    lon: p.lon,
    city: "Rio de Janeiro",
    body: p.narration,
    groundedSourceCount: p.source ? 1 : 0,
    narrationStatus: "ready" as const,
  };
}

/**
 * Calls GET /cities/:city/manifest?language=xx and maps the result to
 * `Place[]` so it can go straight through `planCityDownload`/
 * `estimateDownloadSizeBytes` like any other place list. Every place the
 * manifest returns already has a published narration and a ready audio
 * file -- that's the whole point of the route (see manifest_handler.go).
 *
 * Returns an empty array (not an error) on any non-200 response or network
 * failure -- a city with nothing published yet, or a briefly unreachable
 * backend, should read as "nothing downloadable right now", not crash the
 * onboarding flow.
 */
export async function fetchCityManifest(citySlug: string, language: Locale): Promise<Place[]> {
  const raw = await fetchManifestRaw(citySlug, language);
  return raw.map(manifestPlaceToPlace);
}

export type OfflineDownloadSummary = {
  city: string;
  language: Locale;
  placeCount: number;
  approxSizeBytes: number;
  downloadedAt: string;
};

const STORAGE_KEY = "memoria-carioca:offline-download";
const AUDIO_DIR_NAME = "offline-audio";

export class InsufficientStorageError extends Error {
  constructor(
    public requiredBytes: number,
    public availableBytes: number,
  ) {
    super(
      `Not enough free storage to download this city (${requiredBytes} bytes needed, ${availableBytes} available)`,
    );
    this.name = "InsufficientStorageError";
  }
}

/**
 * Some audio files failed to download but the rest of the city is saved and
 * the summary is written, so re-calling `downloadCity` resumes and retries
 * only what's missing. Distinct from a total failure: the caller should say
 * "N places still to go, try again", not "the download failed".
 */
export class PartialDownloadError extends Error {
  constructor(public failedCount: number) {
    super(`${failedCount} place(s) could not be downloaded`);
    this.name = "PartialDownloadError";
  }
}

// Same estimate as planCityDownload's, but computed straight off a manifest
// place list. downloadCity's own places come from the backend's manifest
// route, already scoped to one city -- routing them back through
// planCityDownload's `p.city === cityDisplayName` filter would silently
// yield an empty list (and so a zero-byte, always-passing storage check) the
// moment the caller's display name stopped matching the hardcoded city in
// manifestPlaceToPlace.
function estimateManifestSizeBytes(places: ManifestPlace[]): number {
  return estimateDownloadSizeBytes(
    places.flatMap((p) => [
      { placeId: p.id, kind: "metadata" as const, path: `${p.id}/metadata.json` },
      { placeId: p.id, kind: "audio" as const, path: `${p.id}/audio.mp3` },
    ]),
  );
}

/**
 * Downloads the real manifest, downloads and caches each place's audio file
 * on-device (expo-file-system), and persists the full place list — including
 * narration text, so PlaceDetail can render offline too — to SQLite via
 * offlineStore. This is what makes both proximity notifications and offline
 * playback actually work, not just the size-estimate summary this function
 * produced before.
 *
 * Resumable: re-calling this for the same city+language after an interrupted
 * download (app killed, network dropped mid-loop) skips any place whose
 * audio is already cached, via `planResumableAudioDownloads`, rather than
 * re-downloading everything from scratch. Switching to a different city or
 * language starts over clean.
 *
 * Throws `InsufficientStorageError` before writing anything, and
 * `PartialDownloadError` after saving everything it could when some audio
 * files failed -- the next call picks up exactly where this one stopped.
 *
 * On web this only ever writes the metadata: expo-file-system has no real
 * filesystem there, so there is no offline audio to cache. A deliberate,
 * visible platform limitation, in the same spirit as the map screen's
 * `webMapUnavailable`.
 */
export async function downloadCity(
  citySlug: string,
  cityDisplayName: string,
  language: Locale,
): Promise<OfflineDownloadSummary> {
  const rawPlaces = await fetchManifestRaw(citySlug, language);

  // Determine if resuming and get already-cached places early (before storage check)
  const existingSummary = await getOfflineDownloadSummary();
  const isResuming =
    existingSummary?.city === cityDisplayName && existingSummary?.language === language;
  const alreadyCached = isResuming ? await getAllCachedPlaces() : [];

  // Compute what still needs to be downloaded (resume-aware)
  const toDownload = planResumableAudioDownloads(rawPlaces, alreadyCached);
  const requiredBytesForDownload = estimateManifestSizeBytes(toDownload);

  if (!hasSufficientStorage(Paths.availableDiskSpace, requiredBytesForDownload)) {
    throw new InsufficientStorageError(requiredBytesForDownload, Paths.availableDiskSpace);
  }

  const isWeb = Platform.OS === "web";

  // Clear old data if switching to a different city/language
  if (!isResuming) {
    await clearCachedPlaces();
    if (!isWeb) {
      const staleAudioDir = new Directory(Paths.document, AUDIO_DIR_NAME);
      if (staleAudioDir.exists) {
        staleAudioDir.delete();
      }
    }
  }

  await saveCachedPlaces(
    rawPlaces.map((p) => {
      const existing = alreadyCached.find((c) => c.id === p.id);
      return {
        id: p.id,
        name: p.name,
        category: p.category,
        lat: p.lat,
        lon: p.lon,
        body: p.narration,
        audioLocalUri: existing?.audioLocalUri ?? null,
      };
    }),
  );

  const summary: OfflineDownloadSummary = {
    city: cityDisplayName,
    language,
    placeCount: rawPlaces.length,
    approxSizeBytes: estimateManifestSizeBytes(rawPlaces),
    downloadedAt: new Date().toISOString(),
  };
  // Written BEFORE the download loop, not after: it's what lets the next
  // call recognise an interrupted attempt as the same city+language and
  // resume it. Written after the loop, an interruption left no summary at
  // all, so the next call read "different city" and wiped every file the
  // interrupted attempt had already fetched.
  await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(summary));

  if (isWeb) return summary;

  const audioDir = new Directory(Paths.document, AUDIO_DIR_NAME);
  audioDir.create({ idempotent: true });

  // Per-place isolation matters here: the manifest's audio URLs are presigned
  // with a 15-minute TTL (see audio_handler.go) and a full city is ~450MB, so
  // late URLs expiring mid-loop is expected, not exceptional. A failed place
  // keeps its null audio_local_uri, so the next call retries exactly it.
  let failedCount = 0;
  for (const place of toDownload) {
    try {
      const destination = new File(audioDir, `${place.id}.mp3`);
      const downloaded = await File.downloadFileAsync(place.audio_url, destination, {
        idempotent: true,
      });
      await setCachedPlaceAudioUri(place.id, downloaded.uri);
    } catch (err) {
      failedCount += 1;
      console.warn(`offline download: skipping ${place.id}`, err);
    }
  }

  if (failedCount > 0) throw new PartialDownloadError(failedCount);
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

/**
 * Clears everything a download wrote: the AsyncStorage summary, every
 * cached_places row, and the on-disk audio files themselves — Settings.tsx's
 * delete button calls this directly.
 */
export async function clearOfflineDownload(): Promise<void> {
  await AsyncStorage.removeItem(STORAGE_KEY);
  await clearCachedPlaces();
  const audioDir = new Directory(Paths.document, AUDIO_DIR_NAME);
  if (audioDir.exists) {
    audioDir.delete();
  }
}

const SIZE_UNIT: Record<Locale, string> = { fr: "Mo", en: "MB", pt: "MB", es: "MB" };

export function formatApproxSize(bytes: number, language: Locale): string {
  const mb = bytes / 1_000_000;
  const value = mb < 10 ? mb.toFixed(1) : Math.round(mb);
  return `${value} ${SIZE_UNIT[language]}`;
}
