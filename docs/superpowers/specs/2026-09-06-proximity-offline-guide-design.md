# Proximity notifications & real offline downloads

**Status:** design approved, spec pending implementation plan.

## Why

A scope audit against `docs/superpowers/specs/2026-08-06-product-prd.md` on 2026-09-06 found the PRD's
#1 "Must" differentiator — a geolocation proximity prompt ("not auto-play") — completely unbuilt:
`Map.tsx` only does a one-shot foreground location fetch. The same audit found the "offline-first maps
+ downloaded audio" Must is also incomplete: `downloadManager.ts` already downloads real place
metadata (narration text, size estimate) and the onboarding flow (`Propose.tsx`/`DownloadSuccess.tsx`)
already calls it, but it does not fetch or cache actual audio bytes, and `Settings.tsx` still shows a
hardcoded "42 lieux · 184 Mo" with a no-op delete button.

These two features are merged into one spec because they're genuinely coupled: proximity notifications
are only useful once a user has downloaded a city (see User Flow below), and honoring a proximity
prompt offline requires the audio itself to already be cached on-device — which is exactly the missing
half of the download feature.

## Scope

**In scope (v1):**
- Real audio file download and on-device caching, extending `downloadManager.ts`'s existing metadata
  download (which stays as-is) to also fetch and store audio bytes via `expo-file-system`.
- A local SQLite store (`expo-sqlite`, already installed) of the downloaded city's full place list
  (id, name, lat, lon, local audio file path) — replaces the current `AsyncStorage` summary-only
  persistence for anything geofencing or offline playback needs to read.
- Background geofencing: native OS region monitoring (`Location.startGeofencingAsync`) over the
  nearest ~20 downloaded places (iOS's simultaneous-region cap; Android allows up to 100 but the
  same nearest-N logic is shared across platforms for one code path), periodically re-registered as
  the user moves, via a coarse background location trigger (`Location.startLocationUpdatesAsync`).
- A local notification (`expo-notifications`' `scheduleNotificationAsync`, no push infrastructure
  needed) fired on region entry only (`notifyOnEnter: true`, `notifyOnExit: false`), naming the place.
- Tapping the notification deep-links straight into `PlaceDetail` for that place (a navigation-ref
  listener at the app root, since the notification-response listener lives outside the navigator).
- Network-aware audio serving in `PlacesRepository.getAudioUrl`: online always takes priority (current
  streaming behavior, unchanged); the on-device cached file is used only when actually offline
  (checked via `expo-network`). This intentionally avoids ever needing to reconcile "is the cached
  copy stale vs. the live one" — online simply always wins when available.
- Per-place notification cooldown (6h default) to avoid re-notifying for the same place repeatedly
  while lingering nearby.
- `Settings.tsx` wired to real data: the real `getOfflineDownloadSummary()` result instead of the
  hardcoded string, a working delete (clears cached audio files + SQLite rows + unregisters
  geofencing), and a new "Notifications de proximité" toggle that requests both the background
  location ("Always") and notification permissions together when turned on.
- New dependencies: `expo-task-manager` (background task registration, required by both
  `startLocationUpdatesAsync` and `startGeofencingAsync`) and `expo-network` (connectivity check).
  `expo-file-system`, `expo-sqlite`, `expo-location`, `expo-notifications`-equivalent, and
  `expo-dev-client` are already installed/configured (a custom dev-client build already exists via
  `eas.json`'s `development` profile — no new native-build infrastructure needed).
- No backend changes: the existing `GET /cities/:city/manifest` response already includes a direct
  `audio_url` per place, which is all the audio-caching step needs.

**Explicitly out of scope for v1 (deferred, not decided now):**
- A foreground-only fallback mode for users who deny "Always" location. Denying it simply keeps the
  Settings toggle off, with an explanation and a deep link to the system settings screen
  (`Linking.openSettings()`) — no parallel code path.
- Partial/selective per-place downloads. The download unit stays the whole city, matching the existing
  single "Rio de Janeiro" row in `Settings.tsx` — there is no case of "nearby but not downloaded" to
  handle as a result.
- Notifying about a nearby place the user hasn't downloaded.
- Multi-city support (the manifest route only serves Rio today; a second city is a separate scope).
- Richer notification content (e.g. a place image) — depends on the separate, not-yet-scoped
  real-per-place-images work.

## User Flow

Unchanged from the PRD's original framing, now made real: tourist downloads the city at the hotel
(wifi) via onboarding → walks with geofencing running fully offline (place coordinates are local,
via SQLite) → gets a proximity notification near a covered site → taps it, opens `PlaceDetail`,
hears the narration (from the local cache if currently offline, streamed normally if online).

## Technical Approach

**Components:**
1. **Extended `downloadManager.ts`** — after the existing metadata fetch, download each place's audio
   file to `expo-file-system`'s document directory and persist the full place list (not just a
   summary) to SQLite, with a per-place `audio_cached` status so an interrupted download can resume
   without re-fetching already-cached files. Checks free disk space
   (`expo-file-system.getFreeDiskStorageAsync`) before starting; ~254 places × ~1.8MB/file (the
   existing `AUDIO_BYTES_PER_FILE` estimate) is roughly 450MB+ for one language, so this check is not
   optional polish.
2. **`src/location/geofenceTask.ts` (new)** — defines the background tasks via
   `TaskManager.defineTask`. On app start (if a city is downloaded and permission is granted), reads
   place coordinates from SQLite, computes the nearest ~20 to the last known position, and calls
   `Location.startGeofencingAsync`. A coarse `startLocationUpdatesAsync` trigger periodically
   recomputes and re-registers the set if the user has moved far enough that it's stale. All region-
   selection and cooldown logic lives in small, pure, unit-tested functions; the `TaskManager.defineTask`
   callbacks themselves stay thin wrappers that call those functions and catch/log any
   `startGeofencingAsync` failure without crashing the task.
3. **Notification firing** — on a geofencing Enter event, checks the per-place cooldown (SQLite
   `last_notified_at`), and if clear, calls `scheduleNotificationAsync` immediately (no online/offline
   branching needed here — see below) and updates the cooldown timestamp.
4. **App-root deep link listener** (`App.tsx`) — `Notifications.addNotificationResponseReceivedListener`
   plus a `createNavigationContainerRef`, navigating to `PlaceDetail` with the tapped notification's
   place id.
5. **Network-aware `PlacesRepository.getAudioUrl`** — checks `expo-network`'s connectivity; online
   returns the existing streamed URL unchanged, offline returns the local cached file URI if present,
   otherwise signals "unavailable offline" (existing `AudioAvailability` states already model this).
6. **`Settings.tsx`** — real summary + working delete (audio files + SQLite rows + `stopGeofencingAsync`)
   + a "Notifications de proximité" toggle requesting both permissions together via
   `Location.requestBackgroundPermissionsAsync()` and notifications' `requestPermissionsAsync()`.

**Known, accepted limitation:** iOS stops all background location/geofencing if the user force-quits
the app from the app switcher, until they manually reopen it. This is standard iOS platform behavior,
not a bug — documented here so it isn't later "discovered" as a regression to fix.

## Testing

- Pure logic (nearest-N selection, cooldown check, disk-space/size estimation, download planning) gets
  Jest coverage, matching `downloadManager.ts`'s existing test file's pattern.
- Native geofencing itself is not unit-testable (depends on real OS APIs) — task-manager callbacks stay
  thin specifically so the untested surface is minimal.
- Real end-to-end verification requires a physical device (no reliable simulator support for region
  monitoring) running an EAS dev-client build, not Expo Go.

## After this plan (explicitly deferred)

- Foreground-only fallback for denied "Always" permission.
- Partial/selective per-place downloads.
- Multi-city downloads.
- Real per-place images (separate, already-flagged content-sourcing gap — not part of this spec).
