# Real Offline Downloads Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the mocked "42 lieux · 184 Mo" offline-download UI with a real one: actually download
and cache each place's audio file on-device, persist the full downloaded place list (including narration
text) so it survives offline, and wire `Settings.tsx` to the real data with a working delete.

**Architecture:** A new `offlineStore.ts` module owns a SQLite table of cached places (coordinates,
narration text, local audio file path) plus small pure planning helpers. `downloadManager.ts`'s existing
`downloadCity()` is extended to actually fetch audio bytes via `expo-file-system` and persist through
`offlineStore`, resuming cleanly if interrupted. `PlacesRepository`'s `getById`/`getAudioUrl` become
network-aware: online behavior is completely unchanged, offline reads from the cache instead.

**Tech Stack:** React Native / Expo SDK 57, TypeScript, `expo-file-system` (class-based `File`/`Directory`/
`Paths` API), `expo-sqlite`, `expo-network` (new dependency), Jest.

**Spec:** `docs/superpowers/specs/2026-09-06-proximity-offline-guide-design.md`

This is plan 1 of 2 for that spec — geofencing, background location, and notifications are a separate,
later plan that depends on this one's `offlineStore` table but is entirely out of scope here.

## Global Constraints

- No backend changes. `GET /cities/:city/manifest` already returns an `audio_url` per place — that's
  all this plan's downloading needs.
- The download unit stays the whole city (one row in Settings, no per-place selective download) —
  matches the single "Rio de Janeiro" download already in `Settings.tsx`.
- Add the new dependency via `npx expo install expo-network` (not a manual `npm install`/version edit),
  matching this project's Expo-managed dependency convention.
- Network-aware fallback pattern, everywhere it applies: **online always takes priority** when
  reachable; the on-device cache is used only when actually offline. Never attempt to reconcile
  freshness between the two.
- Use `expo-file-system`'s current class-based API (`File`, `Directory`, `Paths`). Its older
  `FileSystem.downloadAsync()`-style API is deprecated in the installed version (57.0.4) — do not use it,
  even if training data or older examples suggest it.
- Screens are not component-tested in this app (existing convention) — `Settings.tsx`'s changes need no
  new test file. Pure/logic-bearing modules (`offlineStore.ts`'s planning helpers, the repository's
  online/offline branching) do get Jest coverage, matching `downloadManager.ts`'s existing test file.

---

### Task 1: Offline place cache — SQLite schema, CRUD, and pure planning helpers

**Files:**
- Create: `mobile/src/data/offlineStore.ts`
- Test: `mobile/src/data/__tests__/offlineStore.test.ts`

**Interfaces:**
- Produces: `CachedPlace` type (`{id, name, category, lat, lon, body, audioLocalUri}`, `audioLocalUri:
  string | null`); `saveCachedPlaces(places: CachedPlace[]): Promise<void>` (upsert); `getCachedPlace(id:
  string): Promise<CachedPlace | null>`; `getAllCachedPlaces(): Promise<CachedPlace[]>`;
  `setCachedPlaceAudioUri(id: string, audioLocalUri: string): Promise<void>`; `clearCachedPlaces():
  Promise<void>`; `hasSufficientStorage(availableBytes: number, requiredBytes: number): boolean`;
  `planResumableAudioDownloads<T extends {id: string}>(manifestPlaces: T[], cachedPlaces: CachedPlace[]):
  T[]` (generic so it stays decoupled from `downloadManager.ts`'s own place-shape types).

- [ ] **Step 1: Write the failing tests for the pure helpers**

```ts
// mobile/src/data/__tests__/offlineStore.test.ts
import { hasSufficientStorage, planResumableAudioDownloads } from "../offlineStore";
import type { CachedPlace } from "../offlineStore";

describe("hasSufficientStorage", () => {
  it("is true when available space meets the requirement exactly", () => {
    expect(hasSufficientStorage(1000, 1000)).toBe(true);
  });

  it("is true when available space exceeds the requirement", () => {
    expect(hasSufficientStorage(2000, 1000)).toBe(true);
  });

  it("is false when available space is short", () => {
    expect(hasSufficientStorage(500, 1000)).toBe(false);
  });
});

describe("planResumableAudioDownloads", () => {
  const cachedWithAudio: CachedPlace = {
    id: "a",
    name: "A",
    category: "monument",
    lat: 0,
    lon: 0,
    body: "text",
    audioLocalUri: "file:///a.mp3",
  };
  const cachedWithoutAudio: CachedPlace = { ...cachedWithAudio, id: "b", audioLocalUri: null };

  it("skips places already cached with audio", () => {
    const result = planResumableAudioDownloads(
      [{ id: "a" }, { id: "b" }],
      [cachedWithAudio, cachedWithoutAudio],
    );
    expect(result).toEqual([{ id: "b" }]);
  });

  it("includes a place with no cache row at all", () => {
    const result = planResumableAudioDownloads([{ id: "c" }], [cachedWithAudio]);
    expect(result).toEqual([{ id: "c" }]);
  });

  it("returns everything when nothing is cached yet", () => {
    const manifest = [{ id: "a" }, { id: "b" }];
    expect(planResumableAudioDownloads(manifest, [])).toEqual(manifest);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npx jest offlineStore -t "hasSufficientStorage|planResumableAudioDownloads" -v` (from `mobile/`)
Expected: FAIL with "Cannot find module '../offlineStore'"

- [ ] **Step 3: Write `offlineStore.ts`**

```ts
// mobile/src/data/offlineStore.ts
import * as SQLite from "expo-sqlite";

export type CachedPlace = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  // Narration text -- cached so PlaceDetail can render offline too, not just
  // play the audio. Only ever populated from an already-published manifest
  // entry (see downloadManager.ts), so it's always real, grounded content.
  body: string;
  // null until downloadManager.ts's download loop actually writes the audio
  // file and records its local file:// URI here.
  audioLocalUri: string | null;
};

type CachedPlaceRow = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  body: string;
  audio_local_uri: string | null;
};

const DB_NAME = "memoria-carioca-offline.db";

function rowToCachedPlace(row: CachedPlaceRow): CachedPlace {
  return {
    id: row.id,
    name: row.name,
    category: row.category,
    lat: row.lat,
    lon: row.lon,
    body: row.body,
    audioLocalUri: row.audio_local_uri,
  };
}

let dbPromise: Promise<SQLite.SQLiteDatabase> | null = null;

function getDb(): Promise<SQLite.SQLiteDatabase> {
  if (!dbPromise) {
    dbPromise = SQLite.openDatabaseAsync(DB_NAME).then(async (db) => {
      await db.execAsync(
        `CREATE TABLE IF NOT EXISTS cached_places (
          id TEXT PRIMARY KEY NOT NULL,
          name TEXT NOT NULL,
          category TEXT NOT NULL,
          lat REAL NOT NULL,
          lon REAL NOT NULL,
          body TEXT NOT NULL,
          audio_local_uri TEXT
        );`,
      );
      return db;
    });
  }
  return dbPromise;
}

// Upsert: called both for a fresh download (every row's audioLocalUri is
// null) and when resuming an interrupted one (downloadManager.ts passes the
// already-known audioLocalUri through so a resume never forgets progress).
export async function saveCachedPlaces(places: CachedPlace[]): Promise<void> {
  const db = await getDb();
  for (const p of places) {
    await db.runAsync(
      `INSERT INTO cached_places (id, name, category, lat, lon, body, audio_local_uri)
       VALUES (?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(id) DO UPDATE SET
         name = excluded.name,
         category = excluded.category,
         lat = excluded.lat,
         lon = excluded.lon,
         body = excluded.body`,
      p.id,
      p.name,
      p.category,
      p.lat,
      p.lon,
      p.body,
      p.audioLocalUri,
    );
  }
}

export async function setCachedPlaceAudioUri(id: string, audioLocalUri: string): Promise<void> {
  const db = await getDb();
  await db.runAsync(`UPDATE cached_places SET audio_local_uri = ? WHERE id = ?`, audioLocalUri, id);
}

export async function getCachedPlace(id: string): Promise<CachedPlace | null> {
  const db = await getDb();
  const row = await db.getFirstAsync<CachedPlaceRow>(`SELECT * FROM cached_places WHERE id = ?`, id);
  return row ? rowToCachedPlace(row) : null;
}

export async function getAllCachedPlaces(): Promise<CachedPlace[]> {
  const db = await getDb();
  const rows = await db.getAllAsync<CachedPlaceRow>(`SELECT * FROM cached_places`);
  return rows.map(rowToCachedPlace);
}

export async function clearCachedPlaces(): Promise<void> {
  const db = await getDb();
  await db.execAsync(`DELETE FROM cached_places;`);
}

// --- pure helpers (no I/O -- unit-tested directly, see Step 1) -------------

export function hasSufficientStorage(availableBytes: number, requiredBytes: number): boolean {
  return availableBytes >= requiredBytes;
}

// Which manifest places still need their audio downloaded, given what's
// already cached -- generic over T so downloadManager.ts's own place shape
// doesn't need to be imported here (this module stays a leaf dependency).
export function planResumableAudioDownloads<T extends { id: string }>(
  manifestPlaces: T[],
  cachedPlaces: CachedPlace[],
): T[] {
  const cachedWithAudio = new Set(
    cachedPlaces.filter((p) => p.audioLocalUri !== null).map((p) => p.id),
  );
  return manifestPlaces.filter((p) => !cachedWithAudio.has(p.id));
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npx jest offlineStore -t "hasSufficientStorage|planResumableAudioDownloads" -v` (from `mobile/`)
Expected: PASS, 6 tests

- [ ] **Step 5: Commit**

```bash
git add mobile/src/data/offlineStore.ts mobile/src/data/__tests__/offlineStore.test.ts
git commit -m "mobile: offline place cache (SQLite) with resumable-download planning"
```

---

### Task 2: Real audio download with resume support, extending `downloadCity()`

**Files:**
- Modify: `mobile/src/data/downloadManager.ts`
- Test: `mobile/src/data/__tests__/downloadManager.test.ts`

**Interfaces:**
- Consumes: `offlineStore.ts`'s `saveCachedPlaces`, `getAllCachedPlaces`, `setCachedPlaceAudioUri`,
  `clearCachedPlaces`, `hasSufficientStorage`, `planResumableAudioDownloads`, `CachedPlace` (Task 1).
- Produces: `InsufficientStorageError` class (`{requiredBytes, availableBytes}`); `downloadCity()`'s
  existing signature and `OfflineDownloadSummary` shape stay unchanged; `fetchCityManifest()`'s existing
  signature and behavior stay unchanged (used by `Propose.tsx`, untouched by this task); `clearOfflineDownload()`
  now also deletes cached audio files and SQLite rows, not just the summary.

- [ ] **Step 1: Write the failing test for `InsufficientStorageError`**

Add to `mobile/src/data/__tests__/downloadManager.test.ts` (add `InsufficientStorageError` to the
existing `import { ... } from "../downloadManager";` line at the top of the file):

```ts
describe("InsufficientStorageError", () => {
  it("carries the required and available byte counts", () => {
    const err = new InsufficientStorageError(500_000_000, 100_000_000);
    expect(err.requiredBytes).toBe(500_000_000);
    expect(err.availableBytes).toBe(100_000_000);
    expect(err.name).toBe("InsufficientStorageError");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx jest downloadManager -t "InsufficientStorageError" -v` (from `mobile/`)
Expected: FAIL with "InsufficientStorageError is not defined" (or a TypeScript error if run through
`tsc` first — either way, it doesn't exist yet)

- [ ] **Step 3: Rewrite `downloadManager.ts`**

Replace the entire file with:

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";
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
 * Downloads the real manifest, downloads and caches each place's audio file
 * on-device (expo-file-system), and persists the full place list -- including
 * narration text, so PlaceDetail can render offline too -- to SQLite via
 * offlineStore. This is what makes both proximity notifications and offline
 * playback actually work, not just the size-estimate summary this function
 * produced before.
 *
 * Resumable: re-calling this for the same city+language after an interrupted
 * download (app killed, network dropped mid-loop) skips any place whose
 * audio is already cached, via `planResumableAudioDownloads`, rather than
 * re-downloading everything from scratch. Switching to a different city or
 * language starts over clean.
 */
export async function downloadCity(
  citySlug: string,
  cityDisplayName: string,
  language: Locale,
): Promise<OfflineDownloadSummary> {
  const rawPlaces = await fetchManifestRaw(citySlug, language);
  const places = rawPlaces.map(manifestPlaceToPlace);
  const files = planCityDownload(places, cityDisplayName, language);
  const requiredBytes = estimateDownloadSizeBytes(files);

  if (!hasSufficientStorage(Paths.availableDiskSpace, requiredBytes)) {
    throw new InsufficientStorageError(requiredBytes, Paths.availableDiskSpace);
  }

  const existingSummary = await getOfflineDownloadSummary();
  const isResuming =
    existingSummary?.city === cityDisplayName && existingSummary?.language === language;
  const alreadyCached = isResuming ? await getAllCachedPlaces() : [];
  if (!isResuming) {
    await clearCachedPlaces();
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

  const audioDir = new Directory(Paths.document, AUDIO_DIR_NAME);
  audioDir.create({ idempotent: true });

  const toDownload = planResumableAudioDownloads(rawPlaces, alreadyCached);
  for (const place of toDownload) {
    const destination = new File(audioDir, `${place.id}.mp3`);
    const downloaded = await File.downloadFileAsync(place.audio_url, destination, {
      idempotent: true,
    });
    await setCachedPlaceAudioUri(place.id, downloaded.uri);
  }

  const summary: OfflineDownloadSummary = {
    city: cityDisplayName,
    language,
    placeCount: rawPlaces.length,
    approxSizeBytes: requiredBytes,
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

/**
 * Clears everything a download wrote: the AsyncStorage summary, every
 * cached_places row, and the on-disk audio files themselves -- Settings.tsx's
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npx jest downloadManager -v` (from `mobile/`)
Expected: PASS, all existing tests plus the new `InsufficientStorageError` test (the existing
`planCityDownload`/`estimateDownloadSizeBytes`/`formatApproxSize`/`fetchCityManifest` tests are
unaffected — their public behavior didn't change)

- [ ] **Step 5: Commit**

```bash
git add mobile/src/data/downloadManager.ts mobile/src/data/__tests__/downloadManager.test.ts
git commit -m "mobile: real audio download+cache, resumable, extending downloadCity()"
```

---

### Task 3: Network-aware `PlacesRepository` (`getAudioUrl` and `getById`)

**Files:**
- Create: `mobile/src/utils/network.ts`
- Modify: `mobile/src/data/PlacesRepository.ts`
- Test: `mobile/src/data/__tests__/PlacesRepository.test.ts` (new file)

**Interfaces:**
- Consumes: `offlineStore.ts`'s `getCachedPlace` (Task 1).
- Produces: `isOnline(): Promise<boolean>` from `mobile/src/utils/network.ts`. `HttpPlacesRepository`'s
  public interface (`PlacesRepository`) is unchanged — only internal behavior of `getAudioUrl`/`getById`
  changes (online path unchanged, offline path newly falls back to the cache instead of failing).

- [ ] **Step 1: Add the new dependency**

Run (from `mobile/`): `npx expo install expo-network`

- [ ] **Step 2: Write the failing tests**

```ts
// mobile/src/utils/__tests__/network.test.ts
import { isOnline } from "../network";
import * as Network from "expo-network";

jest.mock("expo-network");

const mockedGetNetworkStateAsync = Network.getNetworkStateAsync as jest.MockedFunction<
  typeof Network.getNetworkStateAsync
>;

describe("isOnline", () => {
  it("is true when isInternetReachable is true", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: true,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(true);
  });

  it("is false when isInternetReachable is false even if isConnected is true", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: false,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(false);
  });

  it("falls back to isConnected when isInternetReachable is undefined", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: undefined,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(true);
  });
});
```

```ts
// mobile/src/data/__tests__/PlacesRepository.test.ts
import { HttpPlacesRepository } from "../PlacesRepository";
import * as offlineStore from "../offlineStore";
import * as network from "../../utils/network";

jest.mock("../offlineStore");
jest.mock("../../utils/network");
jest.mock("../downloadManager", () => ({
  getOfflineDownloadSummary: jest.fn().mockResolvedValue(null),
}));

const mockedIsOnline = network.isOnline as jest.MockedFunction<typeof network.isOnline>;
const mockedGetCachedPlace = offlineStore.getCachedPlace as jest.MockedFunction<
  typeof offlineStore.getCachedPlace
>;

const CACHED_CRISTO = {
  id: "cristo-redentor",
  name: "Cristo Redentor",
  category: "monument",
  lat: -22.9519,
  lon: -43.2105,
  body: "Inaugurée en 1931...",
  audioLocalUri: "file:///offline-audio/cristo-redentor.mp3",
};

describe("HttpPlacesRepository.getAudioUrl", () => {
  const repo = new HttpPlacesRepository(() => "fr");
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    jest.clearAllMocks();
  });

  it("streams the network URL when online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockResolvedValue({
      status: 200,
      json: async () => ({
        url: "https://cdn.example.com/audio.mp3",
        timestamps_url: "https://cdn.example.com/marks.json",
      }),
    }) as unknown as typeof fetch;

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "https://cdn.example.com/audio.mp3",
      timestampsUrl: "https://cdn.example.com/marks.json",
    });
  });

  it("serves the cached local file when offline and the place was downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "file:///offline-audio/cristo-redentor.mp3",
    });
  });

  it("reports unavailable when offline and the place was never downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(null);

    expect(await repo.getAudioUrl("never-downloaded", "fr")).toEqual({ state: "unavailable" });
  });

  it("falls back to the cache when the network request itself fails despite reporting online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockRejectedValue(new Error("flaky connection")) as unknown as typeof fetch;
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "file:///offline-audio/cristo-redentor.mp3",
    });
  });
});

describe("HttpPlacesRepository.getById", () => {
  const repo = new HttpPlacesRepository(() => "fr");
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    jest.clearAllMocks();
  });

  it("returns the network result unchanged when online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockResolvedValue({
      status: 200,
      json: async () => ({
        id: "cristo-redentor",
        name: "Cristo Redentor",
        category: "monument",
        lat: -22.9519,
        lon: -43.2105,
        language: "fr",
        narration: "Inaugurée en 1931...",
        source: "wikidata",
        source_richness: "rich",
      }),
    }) as unknown as typeof fetch;

    const result = await repo.getById("cristo-redentor");
    expect(result?.narrationStatus).toBe("ready");
    expect(result?.body).toBe("Inaugurée en 1931...");
  });

  it("falls back to the cached place when offline", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    const result = await repo.getById("cristo-redentor");
    expect(result).toEqual({
      id: "cristo-redentor",
      name: "Cristo Redentor",
      category: "monument",
      lat: -22.9519,
      lon: -43.2105,
      city: "Rio de Janeiro",
      body: "Inaugurée en 1931...",
      groundedSourceCount: 1,
      narrationStatus: "ready",
    });
  });

  it("returns undefined when offline and the place was never downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(null);

    expect(await repo.getById("never-downloaded")).toBeUndefined();
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `npx jest network.test PlacesRepository.test -v` (from `mobile/`)
Expected: FAIL — `../network` doesn't exist yet, and `PlacesRepository.ts` doesn't yet import
`isOnline`/`getCachedPlace` so the offline-branch assertions fail against the current (throwing/network-only) behavior.

- [ ] **Step 4: Write `mobile/src/utils/network.ts`**

```ts
// mobile/src/utils/network.ts
import * as Network from "expo-network";

/**
 * expo-network's `isConnected` means "an active network interface exists"
 * (e.g. connected to Wi-Fi with no real internet behind it) -- not what this
 * app actually needs to decide "should I stream or use the offline cache".
 * Prefer `isInternetReachable` when the platform reports it, falling back to
 * `isConnected` on platforms/situations where it's undefined (expo-network's
 * own docs note this is platform-specific).
 */
export async function isOnline(): Promise<boolean> {
  const state = await Network.getNetworkStateAsync();
  return state.isInternetReachable ?? state.isConnected ?? false;
}
```

- [ ] **Step 5: Modify `PlacesRepository.ts`**

Modify the imports at the top of `mobile/src/data/PlacesRepository.ts`:

```ts
import type { Place, NarrationStatus, AudioAvailability } from "./types";
import { MOCK_PLACES } from "./types";
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";
import { getOfflineDownloadSummary } from "./downloadManager";
import { getCachedPlace } from "./offlineStore";
import { isOnline } from "../utils/network";
```

Modify `fetchJson` (wraps the fetch call itself in try/catch — a request to an unreachable host
throws a `TypeError` regardless of any online/offline check, and previously that exception was
uncaught all the way up through `getById`/`getAudioUrl`, which is a real crash risk, offline or not):

```ts
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
```

Replace `HttpPlacesRepository`'s `getById` and `getAudioUrl` methods with:

```ts
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
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `npx jest network.test PlacesRepository.test -v` (from `mobile/`)
Expected: PASS, all tests

- [ ] **Step 7: Commit**

```bash
git add mobile/package.json mobile/package-lock.json mobile/src/utils/network.ts mobile/src/utils/__tests__/network.test.ts mobile/src/data/PlacesRepository.ts mobile/src/data/__tests__/PlacesRepository.test.ts
git commit -m "mobile: network-aware getById/getAudioUrl, offline cache fallback"
```

(Adjust the lockfile filename in the `git add` if this project uses `yarn.lock`/`pnpm-lock.yaml`
instead — check which lockfile exists in `mobile/` before running.)

---

### Task 4: `Settings.tsx` wired to real download data, working delete

**Files:**
- Modify: `mobile/src/screens/Settings.tsx`
- Modify: `mobile/src/i18n/dictionary.ts` (add one key, 4 locales)

**Interfaces:**
- Consumes: `downloadManager.ts`'s `getOfflineDownloadSummary`, `clearOfflineDownload`,
  `formatApproxSize`, `OfflineDownloadSummary` type (Task 2). `t.downloadSuccess.cityMeta` (existing key,
  reused rather than duplicated).
- No test file — screens aren't component-tested in this app (Global Constraints).

- [ ] **Step 1: Add the `noDownload` dictionary key**

In `mobile/src/i18n/dictionary.ts`, find the `settings:` block for each of the 4 locales (search for
`settings: {` — there are 4 occurrences, one per locale) and add a `noDownload` key inside each,
alongside the other `settings.*` keys already there (e.g. next to `delete:`):

- `fr`: `noDownload: "Aucune ville téléchargée.",`
- `en`: `noDownload: "No city downloaded yet.",`
- `pt`: `noDownload: "Nenhuma cidade baixada ainda.",`
- `es`: `noDownload: "Ninguna ciudad descargada todavía.",`

- [ ] **Step 2: Rewrite the offline-data section of `Settings.tsx`**

In `mobile/src/screens/Settings.tsx`, add to the imports:

```ts
import {
  getOfflineDownloadSummary,
  clearOfflineDownload,
  formatApproxSize,
  type OfflineDownloadSummary,
} from "../data/downloadManager";
```

Add state near the top of `SettingsScreen`, alongside the existing `useLocale`/`useAuth` calls:

```ts
const [summary, setSummary] = useState<OfflineDownloadSummary | null>(null);

useEffect(() => {
  getOfflineDownloadSummary().then(setSummary);
}, []);

async function handleDeleteDownload() {
  await clearOfflineDownload();
  setSummary(null);
}
```

(`useState`/`useEffect` need to be added to the existing `import React from "react";` line — change
it to `import React, { useEffect, useState } from "react";`.)

Replace the offline-data section's body (the `<View style={styles.row}>` containing the hardcoded
`"Rio de Janeiro"`/`"42 lieux · 184 Mo"` and the no-op delete `Pressable`) with:

```tsx
            <View style={styles.row}>
              <View style={{ flex: 1 }}>
                <Text style={styles.rowLabel}>Rio de Janeiro</Text>
                <Text style={styles.rowSub}>
                  {summary
                    ? t.downloadSuccess.cityMeta
                        .replace("{count}", String(summary.placeCount))
                        .replace("{size}", formatApproxSize(summary.approxSizeBytes, locale))
                    : t.settings.noDownload}
                </Text>
              </View>
              {summary && (
                <Pressable onPress={handleDeleteDownload}>
                  <Text style={styles.link}>{t.settings.delete}</Text>
                </Pressable>
              )}
            </View>
```

- [ ] **Step 3: Manually verify**

Run the app (`npx expo start --web` from `mobile/`), sign in, go through onboarding to download Rio,
then open Settings → the offline-data row should show the real place count and size (not "42 lieux ·
184 Mo"), and the delete link should make it disappear and revert to the "no city downloaded" message.

- [ ] **Step 4: Commit**

```bash
git add mobile/src/screens/Settings.tsx mobile/src/i18n/dictionary.ts
git commit -m "mobile: wire Settings offline-data row to real download data"
```

## Known limitation carried forward (not a gap in this plan)

`listNearby()` (map browsing) is not made offline-aware by this plan — it still requires network to
list places at all. This is a deliberate scope line: proximity notifications (the next plan) deep-link
straight into `PlaceDetail` for a specific place id, bypassing `listNearby()` entirely, so this plan's
`getById`/`getAudioUrl` offline support is what's actually load-bearing for that flow to work.
