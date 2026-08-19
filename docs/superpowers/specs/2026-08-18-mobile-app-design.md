# Mobile app (React Native) — design spec

**Date:** 2026-08-18
**Branch/worktree:** `frontend`, `.worktrees/frontend/mobile/`
**Status:** approved — decomposed sub-project, decided autonomously per explicit user delegation
(see conversation: "travaille de manière indépendante... sans me demander mon avis"). Every decision
below is documented here rather than asked, per that instruction.

## Context

Second sub-project decomposed out of the original frontend brainstorm (the Next.js landing page was
the first, see `2026-08-18-landing-page-design.md`). This is the actual Memória Carioca product: an
offline cultural audio guide for Rio de Janeiro, targeting the App Store (iOS first). All 7 screens
were already designed and approved by the user in an earlier Claude Design prototype session — this
spec reproduces them as production code, it does not redesign them. Prototype source (visual/content
reference, not to be redesigned): `.../scratchpad/app-screens/{Main,Propose,Download,Settings,Map,
PlaceDetail,Assistant}.dc.html`.

Brand tokens (identical to the landing page): terracotta `#C1592E` / `#92401F` dark, cream `#FAF5EE`,
sand `#F0E6D8`, ink `#2B211B` / `#6B5D4F` soft / `#9A8B7C` faint, hairline `rgba(43,33,27,0.14)`.
Display font Playfair Display, body/UI font Inter (system fallback in the prototype; embedded properly
via Expo's font loading here).

## Screens (7, all already copy-complete from the prototype)

1. **Welcome** — terracotta gradient, "Memória Carioca" title, "carioca" gloss line, lede, dot
   progress (1 of 3), "Commencer".
2. **Propose** (offline-download opt-in) — light bg, explains the offline benefit, states it's
   optional, a Rio de Janeiro download-size card, dots (2 of 3), "Télécharger" primary + "Continuer
   sans télécharger" ghost button (skips step 3, goes straight to Map).
3. **Download success** — light bg, checkmark, "Votre guide est prêt", downloaded-city card with a
   "Téléchargé" badge, dots (3 of 3), "Continuer" → Map.
4. **Map** (root screen) — header with wordmark, "Hors ligne · 42 lieux" badge, settings gear icon;
   floating search bar; place pins + a "you are here" marker on the map surface; a bottom "nearby
   place" card (icon, name, distance, play button) that pushes PlaceDetail.
5. **PlaceDetail** (pushed from Map) — hero image with back button and scrim-legible title over it,
   per-place language pills (PT/EN/FR/ES), audio player (play button, waveform, elapsed/total time),
   "Sources vérifiées · contrôle anti-hallucination" badge, body paragraph, "Poser une question sur ce
   lieu" row that pushes Assistant.
6. **Assistant** (pushed from PlaceDetail, roadmap) — back button, "Roadmap · pas encore construit"
   badge, title, subtitle, one example user/AI chat exchange with a source chip, an input bar rendered
   at reduced opacity (not yet functional — no backend endpoint exists for this, see the backend gap
   list in the final report).
7. **Settings** (pushed from Map's gear icon) — back button, grouped iOS-style sections: offline data
   (city, size, delete), default language (radio-style list), about (version, privacy, terms — menu
   rows only, no real legal text to write).

## Navigation (already decided in the earlier conversation)

No tab bar — the app has exactly one real primary section (the map), so a tab bar would waste a whole
navigation affordance on "Settings" alone. Native stack navigation instead:

- **Root stack:** `Onboarding` (only shown once, gated on a stored flag) → `Map` (the app's true root
  after onboarding, or immediately if onboarding was already completed).
- Onboarding is its own 3-screen stack: `Welcome → Propose → DownloadSuccess`, with `Propose`'s ghost
  button jumping straight past `DownloadSuccess` to `Map`.
- From `Map`: tapping a pin or the nearby card pushes `PlaceDetail`; tapping the header gear icon
  pushes `Settings`.
- From `PlaceDetail`: tapping "Poser une question" pushes `Assistant`.
- All pushes use the platform-native back gesture/button (React Navigation's native stack, not the JS
  stack, so this actually feels like iOS rather than an approximation of it).

## Decisions (made autonomously, per delegation)

- **Framework: Expo (managed workflow), not bare React Native.** The app targets only iOS/App Store
  for now, has no need for exotic native modules Expo can't reach (its APIs cover file system/SQLite/
  fonts/haptics — everything this app's feature set needs), and Expo's build/update tooling (EAS)
  removes a large amount of Xcode-project bookkeeping the user would otherwise own by hand. Bare RN
  would only pay off if a native module Expo doesn't support turns up later.
- **Navigation: React Navigation** (native stack) — the de facto standard, and the only realistic
  choice for the push/back model already decided above.
- **Language: TypeScript**, matching the landing page and the project's general preference for typed
  code.
- **State management: React Context + component state, no external state library.** The app's shared
  state is small and shape-stable (which locale is selected, which cities are downloaded, onboarding-
  seen flag) — Redux/Zustand/etc. would add ceremony this app doesn't need. Revisit only if state
  genuinely outgrows this.
- **i18n: a hand-rolled per-locale dictionary** (same shape/spirit as the landing page's `messages/
  *.json`, adapted for RN with `expo-localization` to read the device's locale as the default,
  overridable by the Settings screen's language choice) — not `next-intl` (that's Next.js-specific)
  and not a heavier RN i18n library, for the same YAGNI reasoning as the landing page: a few dozen UI
  strings, no plural rules or date formatting needed yet.
- **Offline storage: `expo-file-system` + a local SQLite index (`expo-sqlite`).** A "city pack" is a
  downloaded ZIP-equivalent set of files (place JSON + one audio file per place in the user's selected
  language only, per the earlier conversation's decision) written under the app's document directory;
  SQLite holds the place metadata (id, name, coordinates, category, download status) so the Map screen
  can query "what's nearby" without re-parsing JSON on every render. This is a real architectural
  decision with runtime behavior, unlike the landing page — it gets actual unit tests (see Testing).
- **Testing: light for screens/UI (visual review, no snapshot-test suite), real unit tests for the
  offline-download/manifest logic** (the download-state reducer, the "which files does this pack need"
  computation, the SQLite read/write helpers) since that code has real branching logic worth locking
  down, unlike presentational screens. Jest + `@testing-library/react-native`, already bundled with
  Expo's default template.
- **No backend calls yet.** Every screen currently renders against local placeholder/mock data (the
  same example place — Cristo Redentor — used throughout the prototype) because, per the gap analysis
  below, the backend doesn't expose the endpoints this app needs yet. The data layer is written behind
  a small repository interface (`PlacesRepository`) so swapping the mock implementation for a real HTTP
  client later touches one file, not every screen.

## Out of scope

- Any real backend integration (see gap list in the build's final report).
- Push notifications, deep linking, analytics, crash reporting.
- Android — iOS/App Store only for this iteration; Expo keeps the door open without committing effort
  now.
- The Assistant screen's actual chat functionality (backend doesn't exist yet — UI only, matching the
  prototype's own "roadmap, not built" framing).
