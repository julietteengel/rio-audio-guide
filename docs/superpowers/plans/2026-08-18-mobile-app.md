# Mobile App — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the 7 already-designed screens as a working Expo/React Native app with native-stack
navigation, matching `docs/superpowers/specs/2026-08-18-mobile-app-design.md` exactly.

**Testing:** light for screens (visual review only); real Jest unit tests for the offline-download
data layer (Task 6).

---

## Task 1: Scaffold Expo project + design tokens

**Files:** `mobile/` (new Expo app), `mobile/src/theme/tokens.ts`

- [ ] Step 1: `npx create-expo-app@latest mobile --template blank-typescript` inside `.worktrees/frontend/`
- [ ] Step 2: `npx expo install @react-navigation/native @react-navigation/native-stack react-native-screens react-native-safe-area-context expo-font expo-splash-screen expo-file-system expo-sqlite expo-localization`
- [ ] Step 3: `src/theme/tokens.ts` exporting the brand color/spacing constants from the spec
- [ ] Step 4: Copy Playfair Display + Inter font files (reuse the same Google Fonts TTF already
      fetched for the icon compositing work at
      `/private/tmp/claude-501/.../scratchpad/icon/PlayfairDisplay-Variable.ttf`, or fetch Inter fresh)
      into `mobile/assets/fonts/`, load via `expo-font` in the root component with a splash-screen gate
- [ ] Step 5: `npx tsc --noEmit` passes on the untouched scaffold

## Task 2: Brand assets

**Files:** `mobile/assets/images/`, `mobile/app.json` (icon/splash config)

- [ ] Step 1: Copy the app icon (`.../scratchpad/icon/icon-final-1024.png`) to
      `mobile/assets/images/icon.png` and wire it into `app.json`'s `expo.icon`
- [ ] Step 2: Copy the PlaceDetail hero illustration (`.../scratchpad/app-screens/detail-hero.jpg`) to
      `mobile/assets/images/place-hero.jpg`

## Task 3: i18n dictionary

**Files:** `mobile/src/i18n/dictionary.ts`, `mobile/src/i18n/LocaleContext.tsx`

- [ ] Step 1: Port every string from the 7 prototype files into a typed `{ fr, en, pt, es }` dictionary
      object (same content already extracted in this session's read of the prototype files)
- [ ] Step 2: `LocaleContext` — reads `expo-localization`'s device locale as the initial default
      (falling back to `en` if it's not one of the 4), exposes `locale`/`setLocale`/`t()`; `setLocale`
      is what the Settings screen's language rows call

## Task 4: Navigation shell

**Files:** `mobile/src/navigation/RootNavigator.tsx`, `OnboardingNavigator.tsx`, `AppNavigator.tsx`

- [ ] Step 1: `OnboardingNavigator` — native stack: Welcome → Propose → DownloadSuccess
- [ ] Step 2: `AppNavigator` — native stack: Map (initial) → PlaceDetail → Assistant, and Map → Settings
- [ ] Step 3: `RootNavigator` — reads an `onboarding-complete` flag from `AsyncStorage`/`expo-file-system`;
      renders `OnboardingNavigator` or `AppNavigator` accordingly; `DownloadSuccess`'s "Continuer" and
      `Propose`'s ghost button both set the flag and switch to `AppNavigator`

## Task 5: Onboarding screens

**Files:** `mobile/src/screens/onboarding/{Welcome,Propose,DownloadSuccess}.tsx`

- [ ] Step 1: `Welcome.tsx` — terracotta gradient (`expo-linear-gradient`), title, gloss, lede, dot
      progress (1/3), CTA
- [ ] Step 2: `Propose.tsx` — light bg, download-icon circle, lede, Rio de Janeiro size card, dots
      (2/3), primary + ghost buttons wired per Task 4 Step 3
- [ ] Step 3: `DownloadSuccess.tsx` — checkmark circle, downloaded-city card with badge, dots (3/3),
      "Continuer"
- [ ] Step 4: Shared `Dots.tsx`, `Card.tsx` components extracted once duplication across the 3 screens
      becomes obvious (don't pre-abstract before writing at least 2 of them)

## Task 6: Offline data layer (the one part of this app with real logic — gets real tests)

**Files:** `mobile/src/data/PlacesRepository.ts`, `mobile/src/data/db.ts`, `mobile/src/data/
downloadManager.ts`, `mobile/src/data/__tests__/downloadManager.test.ts`

- [ ] Step 1: `db.ts` — `expo-sqlite` schema: a `places` table (id, name, category, lat, lon, city,
      downloaded boolean) seeded at first launch from a bundled mock JSON (one entry: Cristo Redentor,
      matching the prototype) — no network call, this is the mock-repository decision from the spec
- [ ] Step 2: `PlacesRepository` interface (`listNearby()`, `getById()`, `search(query)`) + a
      `MockPlacesRepository` implementation reading the SQLite seed data
- [ ] Step 3: `downloadManager.ts` — pure function(s) computing "which files does downloading city X in
      language Y require" from a place list, independent of any actual network/file I/O, so it's
      unit-testable without mocking `expo-file-system`
- [ ] Step 4: Jest tests for Step 3's pure logic (empty city, single place, language filtering) —
      `npx jest` passes

## Task 7: Map screen

**Files:** `mobile/src/screens/Map.tsx`

- [ ] Step 1: Header (wordmark, offline badge sourced from `PlacesRepository`'s downloaded-count,
      settings gear button navigating to `Settings`)
- [ ] Step 2: Floating search bar (visual only for now — no search screen/results list exists yet,
      matches the prototype exactly; flag this as a known gap, don't invent a search results UI that
      wasn't designed)
- [ ] Step 3: Static pin layout matching the prototype's positions + "you are here" marker (this is
      placeholder positioning, not a real map — the prototype never specified a real map SDK/provider,
      and adding one is a real architecture decision the user should make, not assume; flag in the
      final report)
- [ ] Step 4: Bottom nearby-place card (from `PlacesRepository.listNearby()`), tapping it or a pin
      navigates to `PlaceDetail`

## Task 8: PlaceDetail screen

**Files:** `mobile/src/screens/PlaceDetail.tsx`

- [ ] Step 1: Hero image (`place-hero.jpg`) with scrim, back button, title overlay
- [ ] Step 2: Language pills (from `LocaleContext`, selecting one is local to this screen per the
      prototype, distinct from the app-wide default in Settings)
- [ ] Step 3: Audio player UI (play/pause icon toggle, static waveform bars, elapsed/total text) — no
      real audio file exists yet (backend gap), so playback is stubbed/disabled with the UI fully built
- [ ] Step 4: Sources-verified badge, body paragraph, "Poser une question" row navigating to `Assistant`

## Task 9: Assistant + Settings screens

**Files:** `mobile/src/screens/Assistant.tsx`, `mobile/src/screens/Settings.tsx`

- [ ] Step 1: `Assistant.tsx` — back button, roadmap badge, title/subtitle, the one static example
      exchange from the prototype, input bar rendered disabled (opacity, non-interactive — no backend
      endpoint to call)
- [ ] Step 2: `Settings.tsx` — offline-data section (reads `PlacesRepository`, delete button removes the
      SQLite rows + files via `downloadManager`), language section (radio rows calling
      `LocaleContext.setLocale`), about section (version from `app.json`, privacy/terms rows as inert
      menu items — no legal copy exists to link to yet)

## Task 10: Wire it all up, verify

- [ ] Step 1: `npx tsc --noEmit`, `npx jest`, and `npx expo start` (or `npx expo export` for a
      headless build check) all pass
- [ ] Step 2: Manually trace every navigation edge in this plan's Navigation section against the actual
      code (no dead-end screens, no missing back handlers)

## Out of scope (see spec)

- Real backend integration, real map SDK, real audio playback, Assistant functionality, Android.
