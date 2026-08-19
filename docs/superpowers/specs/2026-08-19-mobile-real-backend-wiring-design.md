# Mobile app — real backend wiring, real map, download-language UX fix

**Date:** 2026-08-19
**Branch/worktree:** `frontend`, `.worktrees/frontend/mobile/`
**Status:** done, retrospective — supersedes the "No backend calls yet" / mock-only decision in
`2026-08-18-mobile-app-design.md`. That spec's screen/navigation/architecture sections are still
accurate; only its data-layer section is now out of date. Specs are append-only history in this repo
(see `CLAUDE.md`), so that file was not edited — this one records what changed since.

## Context

Two things happened after the initial mobile app build (`2026-08-18-mobile-app-design.md` /
`2026-08-18-mobile-app.md`, commit `96bc8d1`):

1. The backend (`backend` branch) gained the routes the mobile spec's gap list said were missing.
2. The user flagged that the Propose screen's language picker was calling the app's global
   `setLocale` — so picking a *download* language also silently changed the *interface* language,
   which is confusing (reported live: "je suis en espagnol... si je clique sur une autre langue, ça me
   change la langue de la page").

## Backend routes now available (branch `backend`)

- `GET /places?q=<optional>` — list, or search by name substring (commit `a1c4390`)
- `GET /places/:id?language=xx` — place detail + narration + source; 202 while unpublished, 404 if
  missing (commit `8276b88`)
- `GET /places/:id/audio?language=xx` — presigned S3 URL (pre-existing)
- `GET /cities/:city/manifest?language=xx` — offline bundle for a city+language, only `"rio"` supported
  (commit `a1c4390`); omits any place whose narration or audio isn't both fully ready rather than
  failing the whole request

**Known caveat, not a bug**: the dev Postgres database likely has few or zero places with a fully
`Published` script + `Ready` audio end-to-end, so `/cities/rio/manifest` can legitimately return an
empty `places` array today, and `/places/:id` can 404 or 202 for most real IDs. The mobile code handles
this (empty states render, nothing crashes) — it needs real published content to look populated, not a
code fix.

## Mobile app changes (branch `frontend`, `mobile/`)

- **`HttpPlacesRepository`** (`src/data/PlacesRepository.ts`) replaces the mock as the real
  implementation, calling the routes above. Base URL: `EXPO_PUBLIC_API_BASE_URL` env var, else
  `app.json`'s `extra.apiBaseUrl` (defaults to a LAN IP — **update this per-network/per-machine**, a
  phone on Expo Go can't reach a Mac's `localhost`). `EXPO_PUBLIC_USE_MOCK_DATA=1` forces the old mock
  back for offline/demo use. Commit `f44c6a3`.
- **`Place` gained `narrationStatus: "ready" | "pending" | "unavailable"`** — `getById` on a 202/404
  falls back to list data (name/category/position) rather than nothing; `PlaceDetail.tsx` shows a
  translated status message instead of blank text.
- **Offline download** (`src/data/downloadManager.ts`) now calls `fetchCityManifest` for real; Propose/
  DownloadSuccess show the real place count and real estimated size instead of the old hardcoded
  "42 lieux · 184 Mo". Commit `6fd3758`.
- **Real map** (`src/screens/Map.tsx`): `react-native-maps` + `expo-location` replaced the fixed-pixel
  SVG pin mockup — real markers at each place's actual lat/lon, real user position (permission-gated,
  degrades gracefully if denied), nearest-place distance via haversine. Commit `9a925f5`.
- **Web fallback for the map**: `react-native-maps` has no browser build; a *static* `import` still
  gets executed by Metro on every platform, so it must be loaded through a runtime `require()` gated on
  `Platform.OS !== "web"`, or the browser build throws before anything renders. On web, `Map.tsx` shows
  a plain scrollable place list instead (same tap-through to `PlaceDetail`). This is also what makes it
  possible to test 6 of the 7 screens today with `npx expo start --web` — a Mac with too little
  RAM/disk for Xcode or an iOS Simulator can still exercise everything except the native map. Commit
  `ac81bb0`.
- **Download-language UX fix**: the language picker on `Propose` no longer calls the global
  `setLocale`. It's local state (`downloadLocale`, defaulted to the current UI `locale` — the sensible
  guess), shown as a plain sentence ("Vous allez télécharger le guide en {language}.") with a
  "Télécharger dans une autre langue ?" link that reveals the PT/EN/FR/ES picker only on demand.
  Choosing a language there changes only which guide gets downloaded, never the screen's own text.
  Commits `b61d64c` (first pass, still coupled to `setLocale`) then `9f0a221` (the actual fix).

## Verification

`npx tsc --noEmit` clean and `npx jest` (12/12) after every commit above. `npx expo export
--platform ios` confirmed the app still bundles. The web dev server (`npx expo start --web`) was used
to manually confirm the map's web fallback renders instead of crashing.

## Still open

- Real on-device persistence for downloaded bundles (`expo-sqlite`) — scoped out from
  `2026-08-18-mobile-app.md` Step 1, still not done; `downloadCity` currently persists to
  `AsyncStorage`, not a queryable local DB.
- No real audio file is fetched to disk yet during "download" — only metadata/manifest data. Actual
  offline audio playback needs that wired in.
- The backend gap list from `2026-08-18-mobile-app-design.md` (users table, listening history, search
  index at scale, `neighborhood` field) is otherwise unchanged — see that spec for the full list; the
  4 routes above close only the "how do we fetch data we already have" gap, not those larger ones.
