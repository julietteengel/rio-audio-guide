# Proximity Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Background geofencing that fires a local notification when the user walks near a downloaded
place, and tapping it opens that place's `PlaceDetail` screen — the PRD's #1 "Must" differentiator,
until now completely unbuilt.

**Architecture:** Native OS region monitoring (`expo-location`'s geofencing API) over the nearest ~20
downloaded places, re-registered as the user moves (iOS's hard cap on simultaneously-monitored
regions). A coarse background location trigger recomputes and re-registers the set; a geofence "enter"
event fires a local notification (`expo-notifications`), gated by a per-place cooldown persisted in
SQLite. Tapping the notification deep-links into `PlaceDetail` via a navigation ref held outside the
component tree.

**Tech Stack:** React Native / Expo SDK 57, TypeScript, `expo-location` (already installed, its
background/geofencing surface newly used), `expo-task-manager` (new), `expo-notifications` (new),
`@react-navigation/native`'s `createNavigationContainerRef`, Jest.

**Spec:** `docs/superpowers/specs/2026-09-06-proximity-offline-guide-design.md`

This is plan 2 of 2 for that spec — plan 1 (`docs/superpowers/plans/2026-09-06-real-offline-downloads.md`,
already merged) built the SQLite place cache (`mobile/src/data/offlineStore.ts`) this plan reads from.
Nothing in plan 1 needs to change except one additive schema column (see Task 2).

## Global Constraints

- No backend changes. This plan only reads `offlineStore.ts`'s existing `getAllCachedPlaces()`.
- iOS allows at most 20 simultaneously-monitored geofence regions (confirmed against the installed
  `expo-location@57.0.11` typings — `startGeofencingAsync`'s doc comment). Android allows up to 100, but
  this plan shares one `MAX_MONITORED_REGIONS = 20` constant across both platforms for a single code path.
- Add new dependencies via `npx expo install expo-task-manager expo-notifications` (not manual
  `npm install`/version edits), matching this project's Expo-managed dependency convention.
- Only the entry (Enter) event is monitored (`notifyOnEnter: true, notifyOnExit: false`) — exits are
  never reported, to limit notification noise, per the design spec.
- Per-place notification cooldown: 6 hours (`NOTIFICATION_COOLDOWN_MS` in Task 1), matching the design
  spec's default.
- Denying "Always" location or notification permissions never gets a parallel foreground-only fallback
  mode — the Settings toggle simply stays off, with an explanation and a link to the system settings
  screen (`Linking.openSettings()`). This was explicitly decided during this feature's brainstorming.
- `TaskManager.defineTask(...)` calls must run unconditionally at module scope (never inside a React
  component or effect) — confirmed against the installed `expo-task-manager@57.0.16` typings' own
  doc comment: "It must be called in the global scope of your JavaScript bundle... when the application
  is launched in the background, we need to spin up your JavaScript app, run your task and then shut
  down — no views are mounted in this scenario."
- A user force-quitting the app from the OS app switcher stops all background location/geofencing until
  they manually reopen the app — standard iOS platform behavior, not a bug to work around (already
  documented in the design spec).

---

### Task 1: Pure geofencing logic — region selection, re-registration trigger, cooldown

**Files:**
- Create: `mobile/src/location/geofenceLogic.ts`
- Test: `mobile/src/location/__tests__/geofenceLogic.test.ts`

**Interfaces:**
- Produces: `GeofenceCandidate = {id: string; name: string; lat: number; lon: number}`;
  `GeofenceRegion = {identifier: string; latitude: number; longitude: number; radius: number;
  notifyOnEnter: true; notifyOnExit: false}`; `MAX_MONITORED_REGIONS = 20`;
  `GEOFENCE_RADIUS_METERS = 150`; `REREGISTER_THRESHOLD_METERS = 500`;
  `NOTIFICATION_COOLDOWN_MS = 6 * 60 * 60 * 1000`;
  `selectNearestRegions(places: GeofenceCandidate[], center: {latitude: number; longitude: number}, limit?: number): GeofenceRegion[]`;
  `shouldReregister(lastRegisteredCenter: {latitude: number; longitude: number} | null, currentPosition: {latitude: number; longitude: number}): boolean`;
  `isCooldownActive(lastNotifiedAtMs: number | null, nowMs: number): boolean`.

- [ ] **Step 1: Write the failing tests**

```ts
// mobile/src/location/__tests__/geofenceLogic.test.ts
import {
  selectNearestRegions,
  shouldReregister,
  isCooldownActive,
  NOTIFICATION_COOLDOWN_MS,
  GEOFENCE_RADIUS_METERS,
  type GeofenceCandidate,
} from "../geofenceLogic";

// Roughly Rio de Janeiro coordinates, spread a few km apart -- exact values
// don't matter, only relative distances do.
const CRISTO: GeofenceCandidate = { id: "cristo", name: "Cristo Redentor", lat: -22.9519, lon: -43.2105 };
const PAO_DE_ACUCAR: GeofenceCandidate = { id: "pao", name: "Pão de Açúcar", lat: -22.9491, lon: -43.1545 };
const SELARON: GeofenceCandidate = { id: "selaron", name: "Escadaria Selarón", lat: -22.9147, lon: -43.1796 };
const CENTER_NEAR_CRISTO = { latitude: -22.952, longitude: -43.211 };

describe("selectNearestRegions", () => {
  it("returns the closest place first", () => {
    const regions = selectNearestRegions([PAO_DE_ACUCAR, CRISTO, SELARON], CENTER_NEAR_CRISTO);
    expect(regions[0].identifier).toBe("cristo");
  });

  it("caps the result at the given limit", () => {
    const regions = selectNearestRegions([CRISTO, PAO_DE_ACUCAR, SELARON], CENTER_NEAR_CRISTO, 2);
    expect(regions).toHaveLength(2);
  });

  it("defaults the limit to MAX_MONITORED_REGIONS", () => {
    const manyPlaces: GeofenceCandidate[] = Array.from({ length: 30 }, (_, i) => ({
      id: `place-${i}`,
      name: `Place ${i}`,
      lat: -22.95 + i * 0.001,
      lon: -43.2 + i * 0.001,
    }));
    expect(selectNearestRegions(manyPlaces, CENTER_NEAR_CRISTO)).toHaveLength(20);
  });

  it("sets notifyOnEnter true and notifyOnExit false on every region", () => {
    const regions = selectNearestRegions([CRISTO], CENTER_NEAR_CRISTO);
    expect(regions[0].notifyOnEnter).toBe(true);
    expect(regions[0].notifyOnExit).toBe(false);
  });

  it("uses the place id as the region identifier and GEOFENCE_RADIUS_METERS as the radius", () => {
    const regions = selectNearestRegions([CRISTO], CENTER_NEAR_CRISTO);
    expect(regions[0].identifier).toBe("cristo");
    expect(regions[0].radius).toBe(GEOFENCE_RADIUS_METERS);
  });

  it("returns an empty array for an empty place list", () => {
    expect(selectNearestRegions([], CENTER_NEAR_CRISTO)).toEqual([]);
  });
});

describe("shouldReregister", () => {
  it("is true when there is no previously-registered center", () => {
    expect(shouldReregister(null, CENTER_NEAR_CRISTO)).toBe(true);
  });

  it("is false when the user hasn't moved far from the last registered center", () => {
    const barelyMoved = { latitude: CENTER_NEAR_CRISTO.latitude + 0.0001, longitude: CENTER_NEAR_CRISTO.longitude };
    expect(shouldReregister(CENTER_NEAR_CRISTO, barelyMoved)).toBe(false);
  });

  it("is true once the user has moved past REREGISTER_THRESHOLD_METERS", () => {
    // Roughly 1.1km north -- well past the 500m threshold.
    const movedFar = { latitude: CENTER_NEAR_CRISTO.latitude + 0.01, longitude: CENTER_NEAR_CRISTO.longitude };
    expect(shouldReregister(CENTER_NEAR_CRISTO, movedFar)).toBe(true);
  });
});

describe("isCooldownActive", () => {
  it("is false when the place has never been notified about", () => {
    expect(isCooldownActive(null, Date.now())).toBe(false);
  });

  it("is true immediately after a notification", () => {
    const now = Date.now();
    expect(isCooldownActive(now, now)).toBe(true);
  });

  it("is true just before the cooldown window elapses", () => {
    const now = Date.now();
    expect(isCooldownActive(now - NOTIFICATION_COOLDOWN_MS + 1000, now)).toBe(true);
  });

  it("is false once the cooldown window has elapsed", () => {
    const now = Date.now();
    expect(isCooldownActive(now - NOTIFICATION_COOLDOWN_MS - 1000, now)).toBe(false);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd mobile && npx jest geofenceLogic --verbose`
Expected: FAIL with "Cannot find module '../geofenceLogic'"

- [ ] **Step 3: Write `geofenceLogic.ts`**

```ts
// mobile/src/location/geofenceLogic.ts

export type GeofenceCandidate = {
  id: string;
  name: string;
  lat: number;
  lon: number;
};

export type GeofenceRegion = {
  identifier: string;
  latitude: number;
  longitude: number;
  radius: number;
  notifyOnEnter: true;
  notifyOnExit: false;
};

// iOS's hard cap on simultaneously-monitored regions (Android allows up to
// 100, but sharing one number keeps geofenceTask.ts to a single code path
// across platforms).
export const MAX_MONITORED_REGIONS = 20;

// A radius generous enough to trigger reliably on foot in a dense urban
// area (GPS drift, tall buildings) without being so wide it fires blocks
// away from the actual site.
export const GEOFENCE_RADIUS_METERS = 150;

// Re-registering the monitored region set is a real native-state churn
// operation -- only worth doing once the user has moved far enough that the
// currently-monitored "nearest 20" might no longer be accurate.
export const REREGISTER_THRESHOLD_METERS = 500;

// Per-place notification cooldown, per the design spec: don't re-notify for
// the same place while lingering nearby.
export const NOTIFICATION_COOLDOWN_MS = 6 * 60 * 60 * 1000;

const EARTH_RADIUS_METERS = 6371000;

function toRadians(degrees: number): number {
  return (degrees * Math.PI) / 180;
}

function haversineMeters(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const dLat = toRadians(lat2 - lat1);
  const dLon = toRadians(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRadians(lat1)) * Math.cos(toRadians(lat2)) * Math.sin(dLon / 2) ** 2;
  return EARTH_RADIUS_METERS * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

/**
 * The nearest `limit` places to `center`, as geofence regions ready to pass
 * straight to `Location.startGeofencingAsync`. Always entry-only
 * (`notifyOnExit: false`) -- exits are never reported, to limit notification
 * noise, per the design spec.
 */
export function selectNearestRegions(
  places: GeofenceCandidate[],
  center: { latitude: number; longitude: number },
  limit: number = MAX_MONITORED_REGIONS,
): GeofenceRegion[] {
  return [...places]
    .map((place) => ({
      place,
      distance: haversineMeters(center.latitude, center.longitude, place.lat, place.lon),
    }))
    .sort((a, b) => a.distance - b.distance)
    .slice(0, limit)
    .map(({ place }) => ({
      identifier: place.id,
      latitude: place.lat,
      longitude: place.lon,
      radius: GEOFENCE_RADIUS_METERS,
      notifyOnEnter: true as const,
      notifyOnExit: false as const,
    }));
}

/**
 * Whether the monitored region set is stale enough to recompute and
 * re-register -- true with no prior registration (first run), or once the
 * user has moved past REREGISTER_THRESHOLD_METERS from where the current
 * set was last centered.
 */
export function shouldReregister(
  lastRegisteredCenter: { latitude: number; longitude: number } | null,
  currentPosition: { latitude: number; longitude: number },
): boolean {
  if (!lastRegisteredCenter) return true;
  return (
    haversineMeters(
      lastRegisteredCenter.latitude,
      lastRegisteredCenter.longitude,
      currentPosition.latitude,
      currentPosition.longitude,
    ) >= REREGISTER_THRESHOLD_METERS
  );
}

/**
 * Whether a place's notification cooldown is still active -- never
 * notified (`null`) is never on cooldown.
 */
export function isCooldownActive(lastNotifiedAtMs: number | null, nowMs: number): boolean {
  if (lastNotifiedAtMs === null) return false;
  return nowMs - lastNotifiedAtMs < NOTIFICATION_COOLDOWN_MS;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd mobile && npx jest geofenceLogic --verbose`
Expected: PASS, 15 tests

- [ ] **Step 5: Commit**

```bash
git add mobile/src/location/geofenceLogic.ts mobile/src/location/__tests__/geofenceLogic.test.ts
git commit -m "mobile: pure geofencing logic (region selection, re-registration, cooldown)"
```

---

### Task 2: Background tasks — schema migration, geofencing/location tasks, start/stop/permissions

**Files:**
- Modify: `mobile/src/data/offlineStore.ts` (adds a `last_notified_at` column via a versioned
  migration, plus `getLastNotifiedAt`/`setLastNotifiedAt`)
- Modify: `mobile/src/data/__tests__/offlineStore.test.ts`
- Modify: `mobile/app.json` (config plugin changes for background location + notifications)
- Create: `mobile/src/location/geofenceTask.ts`

**Interfaces:**
- Consumes: Task 1's `selectNearestRegions`, `shouldReregister`, `isCooldownActive`, `MAX_MONITORED_REGIONS`.
- Consumes: `offlineStore.ts`'s existing `getAllCachedPlaces(): Promise<CachedPlace[]>` (unchanged).
- Produces: `offlineStore.ts`'s `getLastNotifiedAt(id: string): Promise<number | null>`,
  `setLastNotifiedAt(id: string, timestampMs: number): Promise<void>`.
- Produces: `geofenceTask.ts`'s `GEOFENCE_TASK_NAME`, `LOCATION_UPDATE_TASK_NAME` (string constants);
  `startProximityMonitoring(): Promise<void>`; `stopProximityMonitoring(): Promise<void>`;
  `requestProximityPermissions(): Promise<boolean>`; `isProximityMonitoringActive(): Promise<boolean>`.

- [ ] **Step 1: Write the failing test for the schema migration's new functions**

Add to `mobile/src/data/__tests__/offlineStore.test.ts` (add `getLastNotifiedAt`, `setLastNotifiedAt`,
`saveCachedPlaces`, `clearCachedPlaces` to the existing `import { ... } from "../offlineStore";` line —
`saveCachedPlaces`/`clearCachedPlaces` aren't imported there yet):

```ts
describe("getLastNotifiedAt / setLastNotifiedAt", () => {
  afterEach(async () => {
    await clearCachedPlaces();
  });

  it("is null for a place that was never notified about", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    expect(await getLastNotifiedAt("cristo")).toBeNull();
  });

  it("returns the timestamp set by setLastNotifiedAt", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    const ts = Date.now();
    await setLastNotifiedAt("cristo", ts);
    expect(await getLastNotifiedAt("cristo")).toBe(ts);
  });

  it("is not clobbered by a later saveCachedPlaces upsert of the same place", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    const ts = Date.now();
    await setLastNotifiedAt("cristo", ts);
    // Re-saving (as a fresh download would) must not reset the cooldown clock.
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "updated text", audioLocalUri: null },
    ]);
    expect(await getLastNotifiedAt("cristo")).toBe(ts);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd mobile && npx jest offlineStore -t "getLastNotifiedAt" --verbose`
Expected: FAIL — `getLastNotifiedAt`/`setLastNotifiedAt` don't exist yet.

- [ ] **Step 3: Modify `offlineStore.ts`**

Add `lastNotifiedAt` to the `CachedPlace` type as an optional field (optional so Plan 1's existing
object literals in `downloadManager.ts`, which don't know about this field, keep compiling unchanged):

```ts
export type CachedPlace = {
  id: string;
  name: string;
  category: string;
  lat: number;
  lon: number;
  body: string;
  audioLocalUri: string | null;
  // Optional: Plan 1's downloadManager.ts never sets this, and doesn't need
  // to -- it's populated only by setLastNotifiedAt, below.
  lastNotifiedAt?: number | null;
};
```

Add `last_notified_at` to `CachedPlaceRow` and `rowToCachedPlace`:

```ts
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
```

```ts
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
```

Replace `getDb()` with a version that runs a versioned migration (safe to run against a device that
already has the table from Plan 1, and safe to run repeatedly):

```ts
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
        await db.execAsync(`ALTER TABLE cached_places ADD COLUMN last_notified_at INTEGER;`);
        await db.execAsync(`PRAGMA user_version = 1;`);
      }
      return db;
    });
  }
  return dbPromise;
}
```

Update the web fallback's `webStore` usages: `saveCachedPlaces`'s web branch and `getAllCachedPlaces`'s
web branch are unaffected (they already copy whatever fields the passed/stored objects have). Add the
two new exported functions, following the same `Platform.OS === "web"` branching pattern as every other
function in this file:

```ts
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
```

Note `saveCachedPlaces`'s `ON CONFLICT(id) DO UPDATE SET` clause already omits both `audio_local_uri`
and (unchanged by this task) `last_notified_at` from its column list — neither is touched by the INSERT
statement at all, so re-saving an existing place via a fresh download can never reset either field.
No change needed there.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd mobile && npx jest offlineStore --verbose`
Expected: PASS, all existing offlineStore tests plus the 3 new ones.

- [ ] **Step 5: Commit**

```bash
git add mobile/src/data/offlineStore.ts mobile/src/data/__tests__/offlineStore.test.ts
git commit -m "mobile: add last_notified_at to the offline place cache (versioned migration)"
```

- [ ] **Step 6: Add the new dependencies**

Run (from `mobile/`): `npx expo install expo-task-manager expo-notifications`

- [ ] **Step 7: Update `app.json`'s plugin config**

Replace the `expo-location` plugin entry:

```json
    [
      "expo-location",
      {
        "locationWhenInUsePermission": "Memória Carioca uses your location to show which places are nearby on the map.",
        "locationAlwaysAndWhenInUsePermission": "Memória Carioca uses your location in the background to let you know when you're near a place with a story to tell.",
        "isIosBackgroundLocationEnabled": true,
        "isAndroidBackgroundLocationEnabled": true,
        "isAndroidForegroundServiceEnabled": true
      }
    ],
```

Add a new `expo-notifications` entry to the `plugins` array (after `expo-audio`) — no options needed,
this app only uses local notifications, not push, so none of the plugin's remote-notification/icon
options apply:

```json
    "expo-notifications"
```

- [ ] **Step 8: Write `geofenceTask.ts`**

```ts
// mobile/src/location/geofenceTask.ts
import * as Location from "expo-location";
import * as TaskManager from "expo-task-manager";
import * as Notifications from "expo-notifications";
import { getAllCachedPlaces, getLastNotifiedAt, setLastNotifiedAt } from "../data/offlineStore";
import { selectNearestRegions, shouldReregister, isCooldownActive } from "./geofenceLogic";

export const GEOFENCE_TASK_NAME = "memoria-carioca-geofence";
export const LOCATION_UPDATE_TASK_NAME = "memoria-carioca-location-update";

// Module-scope, not component state -- deliberately survives across
// app-background/foreground cycles within one process lifetime. Reset to
// null (forcing an immediate re-registration) on every fresh
// startProximityMonitoring() call, which itself only runs once per
// permission grant or app launch, not per re-render.
let lastRegisteredCenter: { latitude: number; longitude: number } | null = null;

// Per expo-task-manager's own docs: must be called unconditionally at
// module scope, not inside any React lifecycle method, so both tasks are
// always defined the moment this module is imported (from App.tsx), on
// every app launch -- including a background launch with no UI mounted.
TaskManager.defineTask(GEOFENCE_TASK_NAME, async ({ data, error }) => {
  if (error) {
    console.warn("geofence task error", error);
    return;
  }
  const { eventType, region } = data as {
    eventType: Location.LocationGeofencingEventType;
    region: Location.LocationRegion;
  };
  if (eventType !== Location.LocationGeofencingEventType.Enter) return;

  const placeId = region.identifier;
  if (!placeId) return;

  const lastNotifiedAt = await getLastNotifiedAt(placeId);
  if (isCooldownActive(lastNotifiedAt, Date.now())) return;

  const places = await getAllCachedPlaces();
  const place = places.find((p) => p.id === placeId);
  if (!place) return;

  try {
    await Notifications.scheduleNotificationAsync({
      content: {
        title: place.name,
        body: "Écoute son histoire.",
        data: { placeId },
      },
      trigger: null,
    });
    await setLastNotifiedAt(placeId, Date.now());
  } catch (err) {
    console.warn("geofence: failed to schedule notification", err);
  }
});

TaskManager.defineTask(LOCATION_UPDATE_TASK_NAME, async ({ data, error }) => {
  if (error) {
    console.warn("location update task error", error);
    return;
  }
  const { locations } = data as { locations: Location.LocationObject[] };
  const latest = locations[locations.length - 1];
  if (!latest) return;

  const currentPosition = {
    latitude: latest.coords.latitude,
    longitude: latest.coords.longitude,
  };
  if (!shouldReregister(lastRegisteredCenter, currentPosition)) return;

  const places = await getAllCachedPlaces();
  const regions = selectNearestRegions(places, currentPosition);
  if (regions.length === 0) return;

  try {
    // stopGeofencingAsync before re-starting: startGeofencingAsync replaces
    // the monitored set, but going through stop first avoids relying on
    // that being safe to call twice with an already-active task under the
    // hood (undocumented either way -- this is the conservative order).
    await Location.stopGeofencingAsync(GEOFENCE_TASK_NAME).catch(() => {});
    await Location.startGeofencingAsync(GEOFENCE_TASK_NAME, regions);
    lastRegisteredCenter = currentPosition;
  } catch (err) {
    console.warn("geofence: failed to re-register regions", err);
  }
});

/**
 * Starts background location updates and registers the initial geofence
 * region set. Call only after `requestProximityPermissions()` has resolved
 * `true`. No-ops if there's nothing downloaded to monitor.
 */
export async function startProximityMonitoring(): Promise<void> {
  const places = await getAllCachedPlaces();
  if (places.length === 0) return;

  await Location.startLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME, {
    accuracy: Location.LocationAccuracy.Balanced,
    distanceInterval: 300,
    showsBackgroundLocationIndicator: false,
  });

  // Register a region set immediately, from the last known position (or,
  // failing that, the first cached place's own coordinates) -- otherwise
  // geofencing sits idle until the first location update fires, which could
  // be a while depending on distanceInterval and how much the user moves.
  const lastKnown = await Location.getLastKnownPositionAsync();
  const initialCenter = lastKnown
    ? { latitude: lastKnown.coords.latitude, longitude: lastKnown.coords.longitude }
    : { latitude: places[0].lat, longitude: places[0].lon };
  const regions = selectNearestRegions(places, initialCenter);
  await Location.startGeofencingAsync(GEOFENCE_TASK_NAME, regions);
  lastRegisteredCenter = initialCenter;
}

export async function stopProximityMonitoring(): Promise<void> {
  lastRegisteredCenter = null;
  if (await Location.hasStartedGeofencingAsync(GEOFENCE_TASK_NAME)) {
    await Location.stopGeofencingAsync(GEOFENCE_TASK_NAME);
  }
  if (await Location.hasStartedLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME)) {
    await Location.stopLocationUpdatesAsync(LOCATION_UPDATE_TASK_NAME);
  }
}

/**
 * Requests foreground location, then background location, then
 * notifications -- in that order, since each is a prerequisite the OS
 * expects before the next (background permission cannot be granted without
 * foreground already granted). Returns whether every one of the three was
 * granted.
 */
export async function requestProximityPermissions(): Promise<boolean> {
  const foreground = await Location.requestForegroundPermissionsAsync();
  if (foreground.status !== "granted") return false;
  const background = await Location.requestBackgroundPermissionsAsync();
  if (background.status !== "granted") return false;
  const notifications = await Notifications.requestPermissionsAsync();
  return notifications.granted;
}

export async function isProximityMonitoringActive(): Promise<boolean> {
  return Location.hasStartedGeofencingAsync(GEOFENCE_TASK_NAME);
}
```

- [ ] **Step 9: Run the full test suite and typecheck**

Run: `cd mobile && npx jest --verbose && npx tsc --noEmit`
Expected: PASS, no new failures (this task adds no new pure-logic tests of its own beyond Task 1's —
`geofenceTask.ts`'s `TaskManager.defineTask` callbacks and the `Location`/`Notifications` calls are
integration glue against native modules, not unit-tested, matching this project's established
convention for untestable native glue — see the design spec's own Testing section).

- [ ] **Step 10: Commit**

```bash
git add mobile/app.json mobile/package.json mobile/package-lock.json mobile/src/location/geofenceTask.ts
git commit -m "mobile: background geofencing + location-update tasks, permission orchestration"
```

---

### Task 3: Notification tap deep-links to `PlaceDetail`

**Files:**
- Modify: `mobile/src/navigation/RootNavigator.tsx` (export its param list, typed for nested navigation)
- Create: `mobile/src/utils/navigationRef.ts`
- Modify: `mobile/App.tsx`

**Interfaces:**
- Consumes: Task 2's nothing directly (this task only reacts to `expo-notifications`' tap events —
  the notification's `data.placeId`, which Task 2's `geofenceTask.ts` already sets when scheduling).
- Produces: `navigationRef.ts`'s `navigationRef` (a `NavigationContainerRef<RootStackParamList>`) and
  `navigateToPlace(placeId: string): void`.

- [ ] **Step 1: Export `RootStackParamList` from `RootNavigator.tsx`, typed for nested navigation**

Modify `mobile/src/navigation/RootNavigator.tsx`:

```ts
import React, { useEffect, useState } from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import type { NavigatorScreenParams } from "@react-navigation/native";
import { OnboardingNavigator } from "./OnboardingNavigator";
import { AppNavigator } from "./AppNavigator";
import type { AppStackParamList } from "./types";
import { isOnboardingComplete } from "../onboarding/onboardingStorage";

export type RootStackParamList = {
  Onboarding: undefined;
  App: NavigatorScreenParams<AppStackParamList> | undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();
```

(The rest of the file — the `RootNavigator` component itself — is unchanged; only the type moves from
an unexported local `type` to an exported one, and its `App` field's type widens from `undefined` to
also accept nested screen params, needed so `navigationRef.navigate("App", {screen: "PlaceDetail", ...})`
type-checks.)

- [ ] **Step 2: Write `navigationRef.ts`**

```ts
// mobile/src/utils/navigationRef.ts
import { createNavigationContainerRef } from "@react-navigation/native";
import type { RootStackParamList } from "../navigation/RootNavigator";

// Held outside the component tree so a notification-tap listener (which
// runs from expo-notifications' own event system, not from any React
// component) can navigate without needing a ref threaded through props.
export const navigationRef = createNavigationContainerRef<RootStackParamList>();

export function navigateToPlace(placeId: string): void {
  if (!navigationRef.isReady()) return;
  navigationRef.navigate("App", { screen: "PlaceDetail", params: { placeId } });
}
```

- [ ] **Step 3: Wire the ref and the tap listener into `App.tsx`**

Modify `mobile/App.tsx`:

```ts
import React, { useCallback, useEffect, useState } from "react";
import { View } from "react-native";
import { StatusBar } from "expo-status-bar";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import * as SplashScreen from "expo-splash-screen";
import * as Font from "expo-font";
import * as Notifications from "expo-notifications";
import { LocaleProvider } from "./src/i18n/LocaleContext";
import { AuthProvider } from "./src/auth/AuthContext";
import { RootNavigator } from "./src/navigation/RootNavigator";
import { navigationRef, navigateToPlace } from "./src/utils/navigationRef";
import { colors } from "./src/theme/tokens";

SplashScreen.preventAutoHideAsync().catch(() => {});

// Must be set once, at module scope, before any notification could arrive
// (including one that woke the app from the background) -- controls how a
// notification presents while the app is in the foreground.
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

export default function App() {
  const [fontsLoaded, setFontsLoaded] = useState(false);

  useEffect(() => {
    Font.loadAsync({
      "PlayfairDisplay-Bold": require("./assets/fonts/PlayfairDisplay-Bold.ttf"),
      "PlayfairDisplay-Black": require("./assets/fonts/PlayfairDisplay-Black.ttf"),
      "Inter-Regular": require("./assets/fonts/Inter-Regular.ttf"),
      "Inter-Medium": require("./assets/fonts/Inter-Medium.ttf"),
      "Inter-SemiBold": require("./assets/fonts/Inter-SemiBold.ttf"),
      "Inter-Bold": require("./assets/fonts/Inter-Bold.ttf"),
    })
      .then(() => setFontsLoaded(true))
      .catch(() => setFontsLoaded(true));
  }, []);

  useEffect(() => {
    const subscription = Notifications.addNotificationResponseReceivedListener((response) => {
      const placeId = response.notification.request.content.data?.placeId;
      if (typeof placeId === "string") navigateToPlace(placeId);
    });
    return () => subscription.remove();
  }, []);

  const onLayoutRootView = useCallback(async () => {
    if (fontsLoaded) {
      await SplashScreen.hideAsync();
    }
  }, [fontsLoaded]);

  if (!fontsLoaded) return null;

  return (
    <View style={{ flex: 1, backgroundColor: colors.cream }} onLayout={onLayoutRootView}>
      <SafeAreaProvider>
        <LocaleProvider>
          <AuthProvider>
            <NavigationContainer
              ref={navigationRef}
              documentTitle={{ formatter: () => "Memória Carioca" }}
            >
              <RootNavigator />
            </NavigationContainer>
          </AuthProvider>
        </LocaleProvider>
      </SafeAreaProvider>
      <StatusBar style="dark" />
    </View>
  );
}
```

- [ ] **Step 4: Typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: no errors (this task has no new automated tests of its own — a notification tap can't be
simulated in Jest without a real native event system; matches this project's convention for
native-glue code, and is covered instead by this plan's manual verification step in Task 4).

- [ ] **Step 5: Commit**

```bash
git add mobile/src/navigation/RootNavigator.tsx mobile/src/utils/navigationRef.ts mobile/App.tsx
git commit -m "mobile: notification tap deep-links into PlaceDetail via a root navigation ref"
```

---

### Task 4: Settings.tsx "Notifications de proximité" toggle

**Files:**
- Modify: `mobile/src/screens/Settings.tsx`
- Modify: `mobile/src/i18n/dictionary.ts`

**Interfaces:**
- Consumes: Task 2's `requestProximityPermissions`, `isProximityMonitoringActive`,
  `startProximityMonitoring`, `stopProximityMonitoring` (`hasProximityPermissions` is not used here —
  the toggle's displayed state reflects whether monitoring is actually running, which correctly
  disambiguates "permission granted but manually stopped" from "never granted," a distinction
  permission status alone can't make).
- No test file — screens aren't component-tested in this app (existing convention, also true of
  `Settings.tsx`'s changes in Plan 1).

- [ ] **Step 1: Add the dictionary keys**

In `mobile/src/i18n/dictionary.ts`, find the `settings:` block for each of the 4 locales (search for
`settings: {` — there are 4 occurrences, one per locale) and add these keys inside each, alongside the
existing `settings.*` keys:

- `fr`:
  ```
  proximitySection: "Notifications de proximité",
  proximityToggleLabel: "Activer les notifications de proximité",
  proximityToggleSub: "Reçois une notification quand tu passes près d'un lieu téléchargé.",
  proximityPermissionDeniedTitle: "Permission requise",
  proximityPermissionDeniedBody: "Pour activer les notifications de proximité, autorise la localisation \"Toujours\" et les notifications dans les réglages du téléphone.",
  openSystemSettings: "Ouvrir les réglages",
  ```
- `en`:
  ```
  proximitySection: "Proximity notifications",
  proximityToggleLabel: "Enable proximity notifications",
  proximityToggleSub: "Get notified when you're near a place you've downloaded.",
  proximityPermissionDeniedTitle: "Permission required",
  proximityPermissionDeniedBody: "To enable proximity notifications, allow \"Always\" location and notifications in your phone's settings.",
  openSystemSettings: "Open settings",
  ```
- `pt`:
  ```
  proximitySection: "Notificações de proximidade",
  proximityToggleLabel: "Ativar notificações de proximidade",
  proximityToggleSub: "Receba uma notificação ao passar perto de um lugar baixado.",
  proximityPermissionDeniedTitle: "Permissão necessária",
  proximityPermissionDeniedBody: "Para ativar as notificações de proximidade, permita a localização \"Sempre\" e as notificações nas configurações do telefone.",
  openSystemSettings: "Abrir configurações",
  ```
- `es`:
  ```
  proximitySection: "Notificaciones de proximidad",
  proximityToggleLabel: "Activar notificaciones de proximidad",
  proximityToggleSub: "Recibe una notificación al pasar cerca de un lugar descargado.",
  proximityPermissionDeniedTitle: "Permiso necesario",
  proximityPermissionDeniedBody: "Para activar las notificaciones de proximidad, permite la ubicación \"Siempre\" y las notificaciones en los ajustes del teléfono.",
  openSystemSettings: "Abrir ajustes",
  ```

- [ ] **Step 2: Add the toggle to `Settings.tsx`**

Modify the imports at the top of `mobile/src/screens/Settings.tsx`:

```ts
import React, { useEffect, useState } from "react";
import { View, Text, Pressable, ScrollView, StyleSheet, Alert, Linking } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { SUPPORTED_LOCALES, Locale } from "../i18n/dictionary";
import { useAuth } from "../auth/AuthContext";
import { colors, fonts, radii } from "../theme/tokens";
import {
  getOfflineDownloadSummary,
  clearOfflineDownload,
  formatApproxSize,
  type OfflineDownloadSummary,
} from "../data/downloadManager";
import {
  requestProximityPermissions,
  isProximityMonitoringActive,
  startProximityMonitoring,
  stopProximityMonitoring,
} from "../location/geofenceTask";
```

Add state and its mount-time check, alongside the existing `summary` state:

```ts
  const [proximityEnabled, setProximityEnabled] = useState(false);

  useEffect(() => {
    getOfflineDownloadSummary().then(setSummary);
    isProximityMonitoringActive().then(setProximityEnabled);
  }, []);

  async function toggleProximity() {
    if (proximityEnabled) {
      await stopProximityMonitoring();
      setProximityEnabled(false);
      return;
    }
    const granted = await requestProximityPermissions();
    if (!granted) {
      Alert.alert(t.settings.proximityPermissionDeniedTitle, t.settings.proximityPermissionDeniedBody, [
        { text: t.settings.deleteAccountCancel, style: "cancel" },
        { text: t.settings.openSystemSettings, onPress: () => Linking.openSettings() },
      ]);
      return;
    }
    await startProximityMonitoring();
    setProximityEnabled(true);
  }
```

Add a new section, placed after the existing "Données hors ligne" (`offlineDataSection`) section and
before the "Langue" (`languageSection`) section:

```tsx
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.proximitySection}</Text>
          <View style={styles.group}>
            <Pressable style={[styles.row, styles.rowLast]} onPress={toggleProximity}>
              <View style={{ flex: 1 }}>
                <Text style={styles.rowLabel}>{t.settings.proximityToggleLabel}</Text>
                <Text style={styles.rowSub}>{t.settings.proximityToggleSub}</Text>
              </View>
              {proximityEnabled && (
                <View style={styles.check}>
                  <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
                    <Polyline points="5 13 10 18 19 7" stroke={colors.cream} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
                  </Svg>
                </View>
              )}
            </Pressable>
          </View>
        </View>
```

- [ ] **Step 3: Typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Manual verification (requires a physical device — see Global Constraints and the
  design spec's Testing section: no simulator reliably supports region monitoring, and this needs an
  EAS dev-client build, not Expo Go)**

1. Build and install a dev-client build on a real device (`eas build --profile development --platform ios`
   or `android`, per this project's existing `eas.json`), then run `npx expo start --dev-client`.
2. Complete onboarding, download the city (Settings → offline-data row shows a real place count).
3. Settings → tap "Activer les notifications de proximité" → grant "Always" location and notifications
   when prompted → the row should show a checkmark.
4. Walk (or simulate location in Xcode's/Android Studio's location simulator) near a downloaded place's
   coordinates → a notification should appear within the geofence radius.
5. Tap the notification → the app should open directly to that place's `PlaceDetail` screen.
6. Deny permissions instead (or revoke them in the phone's system settings after granting) → tapping the
   Settings row again should show the permission-denied alert with a working "Ouvrir les réglages" link.

- [ ] **Step 5: Commit**

```bash
git add mobile/src/screens/Settings.tsx mobile/src/i18n/dictionary.ts
git commit -m "mobile: proximity notifications toggle in Settings"
```
