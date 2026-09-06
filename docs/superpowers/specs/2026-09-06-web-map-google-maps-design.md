# Web map: Google Maps replaces Leaflet

**Status:** design approved, spec pending implementation plan.

## Why

The web build's map (`PlaceMap.web.tsx`, Leaflet + raw OpenStreetMap tiles) looks noticeably plainer
than the native build's (`PlaceMap.tsx`, `react-native-maps`) — flagged directly during manual testing
of the immersive-player redesign, when the web build was used as the primary way to preview mobile
screens without a physical device. Google Maps is the requested replacement for the web tile/rendering
layer.

## Scope

**In scope:** `mobile/src/components/PlaceMap.web.tsx` only, plus a small new `GOOGLE_MAPS_API_KEY`
config export and a new map-style constants file.
**Out of scope:**
- Native (iOS/Android) stays on `react-native-maps` exactly as today — Android already renders Google
  Maps by default through that library; iOS stays on Apple Maps. No native config change, no
  `PROVIDER_GOOGLE` forcing, no Info.plist/native API key.
- `Map.tsx` and `PlaceMap.tsx` (native) are untouched — they already consume the shared `PlaceMapProps`
  interface, which does not change.
- Any change to marker data, place filtering, search, or the "nearby" card on `Map.tsx` — this plan
  touches only what renders inside the map surface itself.

## Approach

`@react-google-maps/api` (`^2.20.8`, supports React 19 — confirmed via its published peer deps)
replaces `react-leaflet`/`leaflet` inside `PlaceMap.web.tsx` only. This is the same component-based
model the current Leaflet code already uses (`<GoogleMap>`/`<MarkerF>` vs. today's
`<MapContainer>`/`<Marker>`), so the migration is a like-for-like rewrite of one file, not a new
paradigm. Considered and rejected: hand-rolling the Google Maps JS API script-loading and marker
lifecycle by hand to avoid a new dependency — a mapping library is required either way (Leaflet is
already exactly that), so avoiding this one doesn't reduce real dependency surface, it just means
writing and maintaining more code for the same job.

## API key and graceful fallback

New env-driven config, following the exact existing pattern for `API_BASE_URL` in
`mobile/src/config.ts` (env var, then `app.json`'s `expo.extra`, no hardcoded value, never committed):
`EXPO_PUBLIC_GOOGLE_MAPS_API_KEY`. The key must be created in Google Cloud Console (Maps JavaScript
API enabled, billing configured) — that account/billing setup is the founder's own action, not
something this plan's implementation can do.

**When the key is absent** (a contributor's machine without it configured, or CI), `PlaceMap.web.tsx`
falls back to today's Leaflet/OpenStreetMap rendering automatically, rather than breaking `expo start
--web` for anyone without the key. Both libraries stay dependencies; the file picks one at render time
based on whether `GOOGLE_MAPS_API_KEY` is a non-empty string.

## Visual design: on-brand map style

Per the same principle already applied to the immersive player redesign (derive from the existing
brand tokens, don't invent an unrelated look): Google Maps' JS API supports fully custom styling via a
JSON style array (`google.maps.MapTypeStyle[]`), passed once the map component initializes it. New file
`mobile/src/components/googleMapStyle.ts` exports:

- `MAP_STYLE: google.maps.MapTypeStyle[]` — landscape/land fill in `colors.sand` (`#F0E6D8`), water in
  a muted warm-adjacent blue (`#D9E6F5`, in the same family as the existing `groundBg` used for the
  "sources verified" badge, not an arbitrary new blue), road geometry in `colors.cream`
  (`#FAF5EE`) with labels in `colors.inkSoft` (`#6B5D4F`), and Google's own default POI icons/labels
  hidden entirely (`featureType: "poi", elementType: "all", visibility: "off"`) — the app already shows
  its own place markers, so Google's default restaurant/shop icons would compete visually with them
  and encourage tapping into Google's own (non-app) info windows.
- Markers keep their existing look exactly, translated to Google's marker `icon` format: place pins as
  a small filled circle (`colors.terracottaDark` fill, `colors.cream` 2px stroke), and the "you are
  here" marker as the same two-layer look already defined in Leaflet's `meIcon()`/native's
  `me`/`meDot` styles (a soft translucent terracotta halo behind a solid terracotta dot) — reproduced
  as an inline SVG data-URI icon (Google Maps' `icon` prop accepts a URL, and a `data:image/svg+xml`
  URI is a URL), matching pixel-for-pixel what Leaflet's `divIcon` HTML string already draws today.

## Zoom / region

The current Leaflet code already ignores `region`'s `latitudeDelta`/`longitudeDelta` entirely and
hardcodes `zoom={12}` — Google's implementation keeps that same simplification (a fixed initial zoom
of 12 centered on `region.latitude`/`region.longitude`) rather than introducing a delta-to-zoom
conversion that doesn't exist anywhere in this codebase today. Not a regression: matches current
behavior exactly.

## Testing

No component-render test for this file, matching the existing convention (no test today for either
`PlaceMap.tsx` or `PlaceMap.web.tsx` — this is UI/gesture code, not pure logic). Manual verification:
`npx expo start --web` with a real key configured (Google-styled map, on-brand colors, custom
markers, click-to-navigate to `PlaceDetail` still works) and, separately, with the env var unset
(confirms the Leaflet fallback still renders correctly, unchanged from today).
