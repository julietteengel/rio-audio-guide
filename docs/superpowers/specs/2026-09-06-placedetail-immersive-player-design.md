# PlaceDetail immersive player redesign

**Status:** design approved (mockup: https://claude.ai/code/artifact/0ecbd9d0-1179-4dde-bd4c-fa6d645fdada), spec pending implementation plan.

## Why

The karaoke-synced narration shipped in `2026-08-26-karaoke-synced-narration-design.md` works
end-to-end (verified live against real Cristo Redentor Polly audio in all 4 languages, including
fixing a missing S3 CORS policy that blocked browser-based testing), but manual testing surfaced
three UX problems with `PlaceDetail.tsx` as it stands:

- The audio "progress bar" is a decorative static waveform (`WAVE_HEIGHTS_PLAYED`/`WAVE_HEIGHTS_REST`)
  never wired to `status.currentTime`/`status.duration` — it doesn't move and can't be tapped to seek.
- Word-by-word highlighting is visually heavy at real reading sizes and drops short words (e.g. "de"
  between "Rio" and "Janeiro") when a playback-status update lands after a short word has already
  passed — not a bug so much as a granularity mismatch between word-level marks and a periodically-
  polled clock.
- Four always-visible language pills (PT/EN/FR/ES) are redundant: `playerLocale` already defaults to
  the app's own reading locale (`useLocale()`), itself set once at onboarding/in Settings. Almost no
  one needs to override it per-place.

Product direction (from user conversation): redesign this screen as an immersive, Spotify-inspired
"now playing" experience — on-brand, not a generic dark music-app skin.

## Scope

**In scope:** `PlaceDetail.tsx` only. One screen.
**Out of scope for this pass** (explicitly deferred, not forgotten):
- Real per-place photos (Wikidata P18 sourcing through the pipeline) — this screen ships with the
  existing generic per-category placeholder image; photo sourcing is its own future chantier.
- Tap-a-line-to-seek — the architecture supports it later (line start times are already known), not
  built now.
- Any scrollable content below the player (related/nearby places, narration credits, etc.) — explicitly
  discussed and deferred; this screen is a fixed, single-viewport "proof of concept" for now, no scroll.
- The `Map` and `Assistant` screens — separate sub-projects, agreed to be tackled after this one.

## Visual direction

Mockup: https://claude.ai/code/artifact/0ecbd9d0-1179-4dde-bd4c-fa6d645fdada (static, one artboard,
390×844). Approved as the visual reference for implementation, with two confirmed adjustments already
folded into the mockup: the play button is a single large centered control below the progress bar
(toggling play/pause by `status.playing`, not flanked by prev/next track controls — this is a single-
narration player, not a playlist), and the screen does not scroll.

The dark theme is **derived from the app's existing brand tokens** (`mobile/src/theme/tokens.ts`), not
an invented palette — this was a specific concern raised during design and is a hard constraint on
implementation:
- Background: `#1C0E07` — this is not a new color, it's the existing hero gradient's base
  (`rgba(28,14,7,...)`, already used in `PlaceDetail.tsx`'s `LinearGradient`), reused as a solid fill.
- Accent: `colors.terracotta` (`#C1592E`) — same accent used everywhere else in the app (play button,
  active progress fill, active toggle state).
- Text on dark: `colors.cream` (`#FAF5EE`) at full opacity for primary/active text, ~45-55% opacity for
  dimmed/secondary text (previous/next lyric lines, time labels) — mirrors the existing
  `rgba(250,245,238,0.85)` already used for the hero eyebrow text.
- Fonts unchanged: `fonts.displayBlack` (Playfair Display) for the place name, `fonts.body`/`bodyBold`
  (Inter) for everything else.
- The "✓ sources verified" badge keeps its existing light pill style (`groundBg`/`groundText`) even
  against the dark background — it's a trust signal, not a themed element.
- This dark theme is scoped to `PlaceDetail.tsx`'s own styles only. It does not touch the shared
  `colors`/`fonts` tokens used by `Map.tsx`, `Settings.tsx`, etc. — those screens stay on the existing
  light/cream theme. (Whether `Map` or other screens later adopt a dark treatment is a decision for
  their own sub-project, not implied by this one.)

## Architecture: one screen, not two

Considered and rejected: splitting into a light "browse" `PlaceDetail` plus a separate full-screen dark
"NowPlaying" screen reached by tapping a mini-player (closer to how Spotify actually separates its
artist page from its Now Playing view). Rejected because this screen's surrounding content (the
sources-verified badge, the "ask a question" link) is short — not enough to justify a second screen and
a new navigation route. `PlaceDetail.tsx` becomes immersive/dark in its entirety; no new screen, no
navigation change.

## Components

- **`AudioProgressBar`** (new, `mobile/src/components/`) — replaces the decorative waveform. A thin
  track (`rgba(250,245,238,0.2)`), a terracotta fill sized to `status.currentTime / status.duration`,
  and a small round terracotta+cream-ring thumb at the current position. Tap or drag anywhere on the
  track computes a fraction from touch-x/track-width and calls `player.seekTo(fraction *
  status.duration)` (the `expo-audio` player already available in `PlaceDetailScreen`). Existing time
  labels (`formatTime(status.currentTime)` / `formatTime(status.duration)`) move here, styled for the
  dark background.
- **Play/pause button** — unchanged behavior (`status.playing ? player.pause() : player.play()`),
  restyled: single large (56px) centered circular terracotta button below the progress bar, not beside
  it. No prev/next controls.
- **`SyncedNarration`** (existing component, evolves) — the word-by-word highlight path is retired.
  Two modes, switched by a small two-icon toggle:
  - **"Défilement doux" (default)** — a 3-line lyric-style panel: the line before the active one
    (dimmed cream), the active line (full-opacity cream, bold, larger), the line after (dimmed cream).
    Lines are groups of a few words each (see "Line grouping" below), not single words and not full
    sentences — this is what actually fixes the "de" skip complaint, since a line lasts several
    seconds, comfortably longer than one playback-status polling interval. At the very first or last
    line, the missing neighbor renders as empty space (reserving its layout slot) rather than
    re-centering the remaining one or two lines — avoids the active line visually jumping position at
    the start/end of playback.
  - **"Lecture libre"** — fully static text, freely scrollable by hand, completely decoupled from
    playback position. This is exactly the existing "no marks" fallback rendering already in
    `SyncedNarration` today — reused as-is, just made selectable even when marks ARE available.
  - Mode selection is local component state for this pass (not persisted to AsyncStorage across app
    restarts) — defaults to "défilement doux".
- **Language pills**: removed entirely from `PlaceDetail.tsx`. `playerLocale` state is deleted; both the
  displayed narration text and the fetched audio use `locale` from `useLocale()` directly. To change
  narration language, the user changes the app's language in Settings — there is no per-place override
  anymore.
- **Hero image**: unchanged mechanism (`require("../../assets/images/place-hero.jpg")`, a single
  generic image for every place) — no per-place photo work in this pass.

## Line grouping (new pure logic)

`mobile/src/utils/syncedText.ts` gains:

- **`groupMarksIntoLines(marks: WordMark[], targetChars = 40): { time: number; text: string }[]`** —
  greedily appends word values (joined with spaces) into the current line until adding the next word
  would push the line past `targetChars`, then starts a new line; enforces a minimum of 2 words per
  line so a line never ends up holding a single orphan word. Each line's `time` is its first word's
  mark time.
- **`findActiveLineIndex(lines: {time:number}[], currentTimeMs: number): number`** — same binary-search
  shape as the existing `findActiveWordIndex`, applied to lines instead of words.

Both are pure functions, unit-tested in Jest exactly like the existing `parseWordMarks`/
`findActiveWordIndex` in this file (per this project's convention: pure logic gets test coverage,
component rendering does not — no `@testing-library/react-native` in this project).

## Data flow

Unchanged from the karaoke plan for fetching: `fetchWordMarks(audio.timestampsUrl)` still runs once
audio is `"ready"`, still clears immediately on a place/language change to avoid showing stale
highlighting mid-transition. What changes is what `PlaceDetailScreen` does with the result: instead of
passing raw `WordMark[]` straight to `SyncedNarration`, it also derives `lines = groupMarksIntoLines(marks)`
and passes both `lines` and the selected mode down, letting `SyncedNarration` pick which of its two
render paths to use.

## Testing

- `groupMarksIntoLines` / `findActiveLineIndex`: Jest, following the existing `syncedText.test.ts`
  patterns (empty input, single word under the target length, a line exactly at the character
  boundary, minimum-2-words enforcement).
- `AudioProgressBar`: no component-render test (project convention) — if the fraction-from-touch-
  position calculation is extracted as a pure function, that alone is testable; the touch handling
  itself is not.
- Manual verification: `npx expo start --web` or Expo Go on a real device against the real Cristo
  Redentor Polly audio (already generated, all 4 languages) — confirm seek works, mode toggle switches
  correctly mid-playback, and no stale state leaks across a language change (Settings, not pills, now
  that pills are gone).

## Open questions for later (explicitly deferred, listed for traceability)

- Real per-place images (pipeline sourcing).
- What (if anything) belongs below the fold on this screen — related/nearby places was floated as an
  idea, not decided.
- Whether "défilement doux" vs "lecture libre" should persist per-user across app restarts.
- Tap-a-line-to-seek.
