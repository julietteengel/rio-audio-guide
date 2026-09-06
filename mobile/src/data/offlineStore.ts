import { Platform } from "react-native";
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
  // Optional: Plan 1's downloadManager.ts never sets this, and doesn't need
  // to -- it's populated only by setLastNotifiedAt, below.
  lastNotifiedAt?: number | null;
};

type CachedPlaceRow = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  body: string;
  audio_local_uri: string | null;
  last_notified_at: number | null;
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
    lastNotifiedAt: row.last_notified_at,
  };
}

// expo-sqlite's web build resolves to a real WASM-backed implementation
// (see expo-sqlite/src/ExpoSQLite.web.ts), not a clean no-op like
// expo-file-system's web shim -- but this project has none of the Metro/
// cross-origin-isolation setup that WASM SQLite needs to actually load in a
// browser, so `openDatabaseAsync` never resolves there: not an error to
// catch, a genuine hang. Web is a secondary testing surface for this app
// (see mobile/AGENTS.md), never the shipping target, so rather than solving
// WASM bundling for a platform that was never going to have real offline
// audio anyway (downloadCity's own isWeb branch already skips that), this
// swaps in an in-memory Map on web -- same interface, lost on refresh, never
// hangs. Native platforms are completely unaffected; this branch never runs
// there.
const webStore = new Map<string, CachedPlace>();

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
      const versionRow = await db.getFirstAsync<{ user_version: number }>(`PRAGMA user_version;`);
      const version = versionRow?.user_version ?? 0;
      if (version < 1) {
        // Both statements in one transaction: a crash between them would
        // leave user_version at 0 with the column already added, so the next
        // launch's ALTER TABLE throws "duplicate column name" -- and that
        // rejection sticks in dbPromise, breaking the whole offline store for
        // the life of the install.
        await db.withTransactionAsync(async () => {
          await db.execAsync(`ALTER TABLE cached_places ADD COLUMN last_notified_at INTEGER;`);
          await db.execAsync(`PRAGMA user_version = 1;`);
        });
      }
      return db;
    });
  }
  return dbPromise;
}

// Upsert: called both for a fresh download (every row's audioLocalUri is
// null) and when resuming an interrupted one (downloadManager.ts passes the
// already-known audioLocalUri through so a resume never forgets progress).
export async function saveCachedPlaces(places: CachedPlace[]): Promise<void> {
  if (Platform.OS === "web") {
    for (const p of places) webStore.set(p.id, p);
    return;
  }
  const db = await getDb();
  // One transaction for the whole manifest (~254 rows): without it, being
  // killed mid-loop leaves the table half-upserted.
  await db.withTransactionAsync(async () => {
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
  });
}

export async function setCachedPlaceAudioUri(id: string, audioLocalUri: string): Promise<void> {
  if (Platform.OS === "web") {
    const existing = webStore.get(id);
    if (existing) webStore.set(id, { ...existing, audioLocalUri });
    return;
  }
  const db = await getDb();
  await db.runAsync(`UPDATE cached_places SET audio_local_uri = ? WHERE id = ?`, audioLocalUri, id);
}

export async function getCachedPlace(id: string): Promise<CachedPlace | null> {
  if (Platform.OS === "web") return webStore.get(id) ?? null;
  const db = await getDb();
  const row = await db.getFirstAsync<CachedPlaceRow>(`SELECT * FROM cached_places WHERE id = ?`, id);
  return row ? rowToCachedPlace(row) : null;
}

export async function getAllCachedPlaces(): Promise<CachedPlace[]> {
  if (Platform.OS === "web") return Array.from(webStore.values());
  const db = await getDb();
  const rows = await db.getAllAsync<CachedPlaceRow>(`SELECT * FROM cached_places`);
  return rows.map(rowToCachedPlace);
}

export async function clearCachedPlaces(): Promise<void> {
  if (Platform.OS === "web") {
    webStore.clear();
    return;
  }
  const db = await getDb();
  await db.execAsync(`DELETE FROM cached_places;`);
}

export async function getLastNotifiedAt(id: string): Promise<number | null> {
  if (Platform.OS === "web") return webStore.get(id)?.lastNotifiedAt ?? null;
  const db = await getDb();
  const row = await db.getFirstAsync<{ last_notified_at: number | null }>(
    `SELECT last_notified_at FROM cached_places WHERE id = ?`,
    id,
  );
  return row?.last_notified_at ?? null;
}

export async function setLastNotifiedAt(id: string, timestampMs: number): Promise<void> {
  if (Platform.OS === "web") {
    const existing = webStore.get(id);
    if (existing) webStore.set(id, { ...existing, lastNotifiedAt: timestampMs });
    return;
  }
  const db = await getDb();
  await db.runAsync(`UPDATE cached_places SET last_notified_at = ? WHERE id = ?`, timestampMs, id);
}

// --- pure helpers (no I/O -- unit-tested directly, see Step 1) -------------

// A non-positive `availableBytes` means "unknown", not "the disk is full":
// expo-file-system's web shim returns 0 for Paths.availableDiskSpace with a
// console warning, and there's no way to tell that apart from a genuinely
// full disk. Refusing every download on web because of an unreadable value
// would be worse than not enforcing the limit at all, so an unknown value
// is treated as unenforceable and lets the download through.
export function hasSufficientStorage(availableBytes: number, requiredBytes: number): boolean {
  if (availableBytes <= 0) return true;
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
