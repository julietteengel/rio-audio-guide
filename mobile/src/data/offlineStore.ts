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
