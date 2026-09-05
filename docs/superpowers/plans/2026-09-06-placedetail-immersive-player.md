# PlaceDetail Immersive Player Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign `PlaceDetail.tsx` as an immersive, Spotify-inspired "now playing" screen — a real
seekable progress bar, a 3-line synced-lyrics-style narration panel (replacing word-by-word
highlighting) with a toggle to plain freely-scrollable text, and no per-place language pills (the
screen now always follows the app's own reading locale).

**Architecture:** No new screen, no navigation change — `PlaceDetail.tsx` becomes dark/immersive in its
entirety (Approach A from the design doc). Two new pure functions in `syncedText.ts` group Polly's
word-level marks into short lyric-style lines; `SyncedNarration` renders either that 3-line panel or
plain scrollable static text, chosen by a `mode` prop; a new `AudioProgressBar` component replaces the
decorative waveform with a real, seekable progress indicator driven by `expo-audio`'s
`player.seekTo(seconds)`.

**Tech Stack:** TypeScript/React Native/Expo (mobile only — no backend change). No new npm dependency:
the progress bar's drag-to-seek uses React Native's built-in `PanResponder`, already part of
`react-native` (confirmed still supported, no deprecation, in the installed `react-native@0.86.2`).

**Spec:** `docs/superpowers/specs/2026-09-06-placedetail-immersive-player-design.md`

## Global Constraints

- Scope is `mobile/src/screens/PlaceDetail.tsx` and its direct dependencies only — no other screen
  changes in this plan (Map and Assistant are separate future sub-projects).
- No new npm dependency.
- The dark palette (`#1C0E07` background, dimmed-cream text variants) is defined as literal constants
  local to the files that need them (`PlaceDetail.tsx`, `AudioProgressBar.tsx`, `SyncedNarration.tsx`)
  — `mobile/src/theme/tokens.ts` (the shared `colors`/`fonts`/`radii` used by every other screen) is
  **not modified**. Only `colors.terracotta`, `colors.cream`, `colors.groundBg`, `colors.groundText`,
  `colors.sand`, `fonts.*`, and `radii.md`/`radii.pill` are imported from there, unchanged.
- `expo-audio@~57.0.3`'s `AudioPlayer.seekTo(seconds: number): Promise<void>` takes **seconds**, not
  milliseconds — confirmed against the Expo SDK 57 docs (this project's `AGENTS.md` flags that Expo's
  API surface changes between versions, so this was checked rather than assumed).
- Real per-place images, tap-a-line-to-seek, any content below the player (e.g. related places), and
  persisting the narration-mode choice across app restarts are explicitly out of scope for this plan.
- Manual verification uses the already-generated real Cristo Redentor Polly audio (all 4 languages) —
  no new audio generation needed. Verify via `npx expo start --web` or Expo Go on a real device
  (backend must be reachable — see the karaoke plan's verification notes for local backend setup).

---

### Task 1: `groupMarksIntoLines` — pure line-grouping logic

**Files:**
- Modify: `mobile/src/utils/syncedText.ts`
- Modify: `mobile/src/utils/__tests__/syncedText.test.ts`

**Interfaces:**
- Consumes: `WordMark` (already defined in this file: `{ time: number; value: string }`).
- Produces: `NarrationLine` type (`{ time: number; text: string }`), `groupMarksIntoLines(marks:
  WordMark[], targetChars?: number): NarrationLine[]` — consumed by Task 4 (`SyncedNarration`).

- [ ] **Step 1: Write the failing tests**

Add to `mobile/src/utils/__tests__/syncedText.test.ts` (append after the existing
`findActiveWordIndex` describe block, adding `groupMarksIntoLines` to the top import):

```ts
import { parseWordMarks, findActiveWordIndex, groupMarksIntoLines } from "../syncedText";
```

```ts
describe("groupMarksIntoLines", () => {
  it("groups words until adding the next would exceed the target length", () => {
    const marks = [
      { time: 0, value: "Au" },
      { time: 100, value: "sommet" },
      { time: 200, value: "du" },
      { time: 300, value: "Corcovado" },
    ];
    // Target 10: "Au sommet" is 9 chars (fits); "Au sommet du" would be 12
    // (flush, 2+ words already held) -- "du" alone is under the minimum of
    // 2 words, so it's forced together with "Corcovado" even though that
    // pair is also over the target.
    expect(groupMarksIntoLines(marks, 10)).toEqual([
      { time: 0, text: "Au sommet" },
      { time: 200, text: "du Corcovado" },
    ]);
  });

  it("does not split when the combined length exactly equals the target", () => {
    const marks = [
      { time: 0, value: "Au" },
      { time: 100, value: "sommet" },
    ];
    expect(groupMarksIntoLines(marks, 9)).toEqual([{ time: 0, text: "Au sommet" }]);
  });

  it("keeps a single trailing word alone when there is nothing left to combine it with", () => {
    expect(groupMarksIntoLines([{ time: 0, value: "Au" }], 10)).toEqual([
      { time: 0, text: "Au" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(groupMarksIntoLines([], 40)).toEqual([]);
  });

  it("defaults the target to 40 characters", () => {
    const marks = [
      { time: 0, value: "El" },
      { time: 50, value: "Cristo" },
      { time: 100, value: "Redentor" },
      { time: 150, value: "mira" },
      { time: 200, value: "hacia" },
      { time: 250, value: "la" },
      { time: 300, value: "bahía" },
      { time: 350, value: "de" },
      { time: 400, value: "Guanabara" },
    ];
    // "El Cristo Redentor mira hacia la bahía de" is 42 chars -- over 40,
    // so it flushes before "de", not after.
    expect(groupMarksIntoLines(marks)).toEqual([
      { time: 0, text: "El Cristo Redentor mira hacia la bahía" },
      { time: 350, text: "de Guanabara" },
    ]);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts -t groupMarksIntoLines`
Expected: FAIL — `groupMarksIntoLines` is not exported from `../syncedText`.

- [ ] **Step 3: Write the implementation**

Append to `mobile/src/utils/syncedText.ts`:

```ts
export type NarrationLine = { time: number; text: string };

// Groups consecutive word marks into short lyric-style lines -- greedily
// appends words until the joined text would exceed targetChars, then
// starts a new line. Never flushes a line holding fewer than 2 words
// (forces the next word in instead) so a line never ends up as a single
// orphan word when there's another word available to join it -- this is
// what actually fixes the "short word flashes past unseen" complaint from
// per-word highlighting: a line lasts several seconds, comfortably longer
// than one playback-status poll interval.
export function groupMarksIntoLines(marks: WordMark[], targetChars = 40): NarrationLine[] {
  if (marks.length === 0) return [];

  const lines: NarrationLine[] = [];
  let current: WordMark[] = [marks[0]];

  const flush = () => {
    lines.push({
      time: current[0].time,
      text: current.map((m) => m.value).join(" "),
    });
  };

  for (let i = 1; i < marks.length; i++) {
    const candidate = [...current, marks[i]].map((m) => m.value).join(" ");
    if (candidate.length > targetChars && current.length >= 2) {
      flush();
      current = [marks[i]];
    } else {
      current.push(marks[i]);
    }
  }
  flush();

  return lines;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts -t groupMarksIntoLines`
Expected: PASS (all 5 cases).

- [ ] **Step 5: Commit**

```bash
git add mobile/src/utils/syncedText.ts mobile/src/utils/__tests__/syncedText.test.ts
git commit -m "syncedText: group word marks into lyric-style lines"
```

---

### Task 2: `findActiveLineIndex` — pure line-lookup logic

**Files:**
- Modify: `mobile/src/utils/syncedText.ts`
- Modify: `mobile/src/utils/__tests__/syncedText.test.ts`

**Interfaces:**
- Consumes: `NarrationLine` (Task 1).
- Produces: `findActiveLineIndex(lines: NarrationLine[], currentTimeMs: number): number` — consumed by
  Task 4 (`SyncedNarration`).

- [ ] **Step 1: Write the failing tests**

Add to `mobile/src/utils/__tests__/syncedText.test.ts` (update the top import again):

```ts
import { parseWordMarks, findActiveWordIndex, groupMarksIntoLines, findActiveLineIndex } from "../syncedText";
```

```ts
describe("findActiveLineIndex", () => {
  const lines = [
    { time: 0, text: "Au sommet" },
    { time: 100, text: "du Corcovado" },
    { time: 250, text: "se dresse" },
  ];

  it("returns -1 before the first line", () => {
    expect(findActiveLineIndex(lines, -1)).toBe(-1);
  });

  it("returns the first index exactly at its own timestamp", () => {
    expect(findActiveLineIndex(lines, 0)).toBe(0);
  });

  it("returns the previous index between two timestamps", () => {
    expect(findActiveLineIndex(lines, 150)).toBe(1);
  });

  it("returns the last index once past the final timestamp", () => {
    expect(findActiveLineIndex(lines, 10000)).toBe(2);
  });

  it("returns -1 for an empty lines array", () => {
    expect(findActiveLineIndex([], 500)).toBe(-1);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts -t findActiveLineIndex`
Expected: FAIL — `findActiveLineIndex` is not exported from `../syncedText`.

- [ ] **Step 3: Write the implementation**

Append to `mobile/src/utils/syncedText.ts`:

```ts
// Same binary-search shape as findActiveWordIndex above, applied to
// NarrationLine[] instead of WordMark[] -- kept as a separate function
// (not a generic) so each call site's intent stays obvious from its name.
export function findActiveLineIndex(lines: NarrationLine[], currentTimeMs: number): number {
  if (lines.length === 0 || currentTimeMs < lines[0].time) return -1;
  let lo = 0;
  let hi = lines.length - 1;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (lines[mid].time <= currentTimeMs) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts`
Expected: PASS — every test in the file (old word-level tests untouched, both new describe blocks
green).

- [ ] **Step 5: Commit**

```bash
git add mobile/src/utils/syncedText.ts mobile/src/utils/__tests__/syncedText.test.ts
git commit -m "syncedText: find the active lyric-style line for a playback time"
```

---

### Task 3: `AudioProgressBar` — real, seekable progress bar

**Files:**
- Create: `mobile/src/components/AudioProgressBar.tsx`

**Interfaces:**
- Produces: `AudioProgressBar` component, props `{ progress: number; onSeek: (fraction: number) =>
  void }` — consumed by Task 5 (`PlaceDetail.tsx`).

No TDD here — same convention as the existing `SyncedNarration` component (no
`@testing-library/react-native` in this project; component rendering and gesture handling aren't
unit-tested, only the pure logic feeding them is, per Task 1/2 above).

- [ ] **Step 1: Write the component**

```tsx
// mobile/src/components/AudioProgressBar.tsx
import React, { useRef, useState } from "react";
import { View, StyleSheet, PanResponder, type PanResponderGestureState } from "react-native";
import { colors } from "../theme/tokens";

type Props = {
  /** Current playback position as a fraction of duration, 0-1. Ignored
   * while the user is actively dragging (the drag position takes over the
   * display until release). */
  progress: number;
  /** Called once, with a 0-1 fraction, when the user releases a tap or drag. */
  onSeek: (fraction: number) => void;
};

const TRACK_HEIGHT = 4;
const THUMB_SIZE = 12;

export function AudioProgressBar({ progress, onSeek }: Props) {
  const trackRef = useRef<View>(null);
  // pageX/width in screen coordinates -- refreshed on layout and again on
  // every gesture grant, since PanResponder's gestureState.moveX is a
  // screen coordinate, not one relative to this view (nativeEvent.locationX
  // is ambiguous enough across RN versions that this measures explicitly
  // rather than relying on it).
  const trackLayout = useRef({ pageX: 0, width: 0 });
  const [dragFraction, setDragFraction] = useState<number | null>(null);

  const measureTrack = () => {
    trackRef.current?.measure((_x, _y, width, _height, pageX) => {
      trackLayout.current = { pageX, width };
    });
  };

  const fractionFromGesture = (gestureState: PanResponderGestureState): number => {
    const { pageX, width } = trackLayout.current;
    if (width <= 0) return 0;
    return Math.min(1, Math.max(0, (gestureState.moveX - pageX) / width));
  };

  const panResponder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: () => true,
      onPanResponderGrant: (_evt, gestureState) => {
        measureTrack();
        setDragFraction(fractionFromGesture(gestureState));
      },
      onPanResponderMove: (_evt, gestureState) => {
        setDragFraction(fractionFromGesture(gestureState));
      },
      onPanResponderRelease: (_evt, gestureState) => {
        const fraction = fractionFromGesture(gestureState);
        setDragFraction(null);
        onSeek(fraction);
      },
      onPanResponderTerminate: () => setDragFraction(null),
    }),
  ).current;

  const displayProgress = dragFraction ?? progress;

  return (
    <View
      ref={trackRef}
      style={styles.track}
      onLayout={measureTrack}
      {...panResponder.panHandlers}
    >
      <View style={[styles.fill, { width: `${displayProgress * 100}%` }]} />
      <View style={[styles.thumb, { left: `${displayProgress * 100}%` }]} />
    </View>
  );
}

const styles = StyleSheet.create({
  track: {
    height: TRACK_HEIGHT,
    borderRadius: TRACK_HEIGHT / 2,
    backgroundColor: "rgba(250,245,238,0.2)",
    justifyContent: "center",
  },
  fill: {
    position: "absolute",
    left: 0,
    top: 0,
    height: TRACK_HEIGHT,
    borderRadius: TRACK_HEIGHT / 2,
    backgroundColor: colors.terracotta,
  },
  thumb: {
    position: "absolute",
    width: THUMB_SIZE,
    height: THUMB_SIZE,
    borderRadius: THUMB_SIZE / 2,
    backgroundColor: colors.terracotta,
    borderWidth: 2,
    borderColor: colors.cream,
    marginLeft: -(THUMB_SIZE / 2),
    top: (TRACK_HEIGHT - THUMB_SIZE) / 2,
  },
});
```

- [ ] **Step 2: Type-check**

Run: `cd mobile && npx tsc --noEmit`
Expected: no new type errors.

- [ ] **Step 3: Commit**

```bash
git add mobile/src/components/AudioProgressBar.tsx
git commit -m "mobile: AudioProgressBar with drag-to-seek, replaces decorative waveform"
```

---

### Task 4: `SyncedNarration` — 3-line panel + free-reading mode, retire word highlighting

**Files:**
- Modify: `mobile/src/components/SyncedNarration.tsx` (full rewrite — the word-by-word rendering path,
  its scroll-tracking refs, and its layout-measuring effects are all removed, not extended: a 3-line
  panel that swaps text is much simpler than an auto-scrolling word list, and none of that machinery
  is needed for it)

**Interfaces:**
- Consumes: `groupMarksIntoLines`, `findActiveLineIndex` (Tasks 1-2).
- Produces: `NarrationMode` type (`"scroll" | "free"`), `SyncedNarration` component with new props
  `{ text: string; marks: WordMark[] | null; currentTimeMs: number; mode: NarrationMode }` — consumed
  by Task 5 (`PlaceDetail.tsx`). The `mode` prop is new; the other three already existed.

No TDD here — same convention as before (component rendering not unit-tested in this project).

- [ ] **Step 1: Write the new component**

Replace the entire contents of `mobile/src/components/SyncedNarration.tsx`:

```tsx
// mobile/src/components/SyncedNarration.tsx
import React from "react";
import { Text, View, ScrollView, StyleSheet } from "react-native";
import { findActiveLineIndex, groupMarksIntoLines, type WordMark } from "../utils/syncedText";
import { colors, fonts } from "../theme/tokens";

export type NarrationMode = "scroll" | "free";

type Props = {
  text: string;
  marks: WordMark[] | null;
  currentTimeMs: number;
  mode: NarrationMode;
};

// Dimmed cream, matching the ~85% used for the hero eyebrow elsewhere in
// PlaceDetail.tsx -- this component now always renders against the same
// dark background, never the light theme it originally shared with the
// rest of the (pre-redesign) screen.
const DIM_TEXT = "rgba(250,245,238,0.45)";
const FREE_TEXT = "rgba(250,245,238,0.85)";

export function SyncedNarration({ text, marks, currentTimeMs, mode }: Props) {
  // "Lecture libre" is the same rendering whether or not marks exist (no
  // marks at all falls back here too) -- it's the full narration text,
  // scrollable by hand, completely decoupled from playback position. It
  // needs its own ScrollView: the *screen* around this component doesn't
  // scroll (a deliberate, separate decision — PlaceDetail is a fixed
  // single-viewport player), but the narration text itself can run far
  // longer than the space available to it, so this inner area scrolls
  // locally, within its own fixed-height slot.
  if (mode === "free" || !marks) {
    return (
      <ScrollView style={styles.freeScroll} contentContainerStyle={styles.freeContent}>
        <Text style={styles.freeText}>{text}</Text>
      </ScrollView>
    );
  }

  const lines = groupMarksIntoLines(marks);
  // Clamped to 0: before the first line's timestamp (findActiveLineIndex
  // returns -1), the first line displays as the upcoming/active one rather
  // than showing nothing.
  const activeIndex = Math.max(findActiveLineIndex(lines, currentTimeMs), 0);
  const prevLine = activeIndex > 0 ? lines[activeIndex - 1].text : "";
  const activeLine = lines[activeIndex]?.text ?? "";
  const nextLine = activeIndex + 1 < lines.length ? lines[activeIndex + 1].text : "";

  return (
    <View style={styles.panel}>
      <Text style={styles.dimLine}>{prevLine}</Text>
      <Text style={styles.activeLine}>{activeLine}</Text>
      <Text style={styles.dimLine}>{nextLine}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  freeScroll: { flex: 1, width: "100%" },
  freeContent: { paddingVertical: 4 },
  freeText: {
    fontFamily: fonts.body,
    fontSize: 15,
    lineHeight: 24,
    color: FREE_TEXT,
  },
  panel: {
    alignItems: "center",
    justifyContent: "center",
    gap: 14,
  },
  dimLine: {
    fontFamily: fonts.body,
    fontSize: 16,
    lineHeight: 22,
    color: DIM_TEXT,
    textAlign: "center",
    // Reserves a stable line height even when empty (first/last line has
    // no neighbor on one side) so the active line doesn't visually jump
    // up or down at the very start or end of playback.
    minHeight: 22,
  },
  activeLine: {
    fontFamily: fonts.bodyBold,
    fontSize: 22,
    lineHeight: 30,
    color: colors.cream,
    textAlign: "center",
  },
});
```

- [ ] **Step 2: Type-check**

Run: `cd mobile && npx tsc --noEmit`
Expected: no new type errors. (`PlaceDetail.tsx` will fail to compile until Task 5 updates its call
site to pass the new required `mode` prop — that's expected at this point in the plan; re-run this
check again after Task 5.)

- [ ] **Step 3: Commit**

```bash
git add mobile/src/components/SyncedNarration.tsx
git commit -m "mobile: SyncedNarration renders lyric-style lines or free-scrolling text, not word highlighting"
```

---

### Task 5: Wire it all into `PlaceDetail.tsx` — dark theme, no scroll, no language pills

**Files:**
- Modify: `mobile/src/screens/PlaceDetail.tsx` (full rewrite of the file's content — the change touches
  imports, state, the whole JSX tree, and the whole stylesheet, not an isolated region)

**Interfaces:**
- Consumes: `AudioProgressBar` (Task 3), `SyncedNarration`/`NarrationMode` (Task 4), `useLocale`,
  `placesRepository`, `fetchWordMarks` (all pre-existing, unchanged).

- [ ] **Step 1: Replace the file**

Replace the entire contents of `mobile/src/screens/PlaceDetail.tsx`:

```tsx
// mobile/src/screens/PlaceDetail.tsx
import React, { useEffect, useState } from "react";
import { View, Text, Pressable, Image, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { LinearGradient } from "expo-linear-gradient";
import { useAudioPlayer, useAudioPlayerStatus } from "expo-audio";
import Svg, { Polyline, Path, Line } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { placesRepository } from "../data/PlacesRepository";
import type { Place, AudioAvailability } from "../data/types";
import { colors, fonts, radii } from "../theme/tokens";
import { fetchWordMarks, type WordMark } from "../utils/syncedText";
import { SyncedNarration, type NarrationMode } from "../components/SyncedNarration";
import { AudioProgressBar } from "../components/AudioProgressBar";

type Props = NativeStackScreenProps<AppStackParamList, "PlaceDetail">;

// PlaceDetail is an immersive "now playing" screen with its own dark
// palette, derived from the app's existing brand tokens rather than an
// invented one. #1C0E07 is not a new color: it's the existing hero
// gradient's base (rgba(28,14,7,...), below), reused as a solid fill so
// the whole screen reads as one continuous dark surface, not just the
// hero image. Scoped to this screen's own files only -- mobile/src/theme/tokens.ts,
// shared by every other screen, is untouched.
const DARK_BG = "#1C0E07";
const DIM_55 = "rgba(250,245,238,0.55)";
const DIM_45 = "rgba(250,245,238,0.45)";
const DIM_18 = "rgba(250,245,238,0.18)";
const TINT_ACTIVE = "rgba(193,89,46,0.18)";

// currentTime/duration come from expo-audio's AudioStatus in seconds
// (fractional while loading) -- "0:00" rather than "0:NaN" before a source
// has loaded.
function formatTime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60)
    .toString()
    .padStart(2, "0");
  return `${m}:${s}`;
}

export function PlaceDetailScreen({ route, navigation }: Props) {
  const { t, locale } = useLocale();
  const [place, setPlace] = useState<Place | null>(null);
  const [audio, setAudio] = useState<AudioAvailability>({ state: "unavailable" });
  const [marks, setMarks] = useState<WordMark[] | null>(null);
  const [narrationMode, setNarrationMode] = useState<NarrationMode>("scroll");

  useEffect(() => {
    placesRepository.getById(route.params.placeId).then((p) => setPlace(p ?? null));
  }, [route.params.placeId]);

  // Narration audio and text both follow the app's own reading locale --
  // there is no more per-place language override (the removed language
  // pills). Changing language happens once, in Settings.
  useEffect(() => {
    if (!place) return;
    let cancelled = false;
    placesRepository.getAudioUrl(place.id, locale).then((result) => {
      if (!cancelled) setAudio(result);
    });
    return () => {
      cancelled = true;
    };
  }, [place?.id, locale]);

  // Vidé immédiatement, avant même que le fetch ne réponde -- sinon les
  // marks de l'ancienne langue resteraient affichées un instant pendant la
  // transition, surlignant les mauvais mots.
  useEffect(() => {
    setMarks(null);
    if (audio.state !== "ready" || !audio.timestampsUrl) return;
    let cancelled = false;
    fetchWordMarks(audio.timestampsUrl).then((result) => {
      if (!cancelled) setMarks(result);
    });
    return () => {
      cancelled = true;
    };
  }, [audio]);

  // Hooks must run unconditionally on every render -- source is null until
  // audio.state is "ready", which useAudioPlayer accepts (no source loaded
  // yet, not an error).
  const player = useAudioPlayer(audio.state === "ready" ? audio.url : null);
  const status = useAudioPlayerStatus(player);

  if (!place) return null;

  const canPlay = audio.state === "ready";
  // Falls back to the raw backend value for any category not yet in the
  // dictionary, rather than showing nothing.
  const categoryLabel =
    (t.categories as Record<string, string>)[place.category] ?? place.category;
  const progress = status.duration > 0 ? status.currentTime / status.duration : 0;

  return (
    <View style={styles.screen}>
      <View style={styles.hero}>
        <Image
          source={require("../../assets/images/place-hero.jpg")}
          style={StyleSheet.absoluteFill}
          resizeMode="cover"
        />
        <LinearGradient
          colors={["rgba(28,14,7,0.78)", "rgba(28,14,7,0.22)", "rgba(28,14,7,0)"]}
          start={{ x: 0, y: 1 }}
          end={{ x: 0, y: 0 }}
          style={StyleSheet.absoluteFill}
        />
        <SafeAreaView edges={["top"]}>
          <Pressable style={styles.back} onPress={() => navigation.goBack()}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Polyline points="15 6 9 12 15 18" stroke={colors.cream} strokeWidth={2.4} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
          </Pressable>
        </SafeAreaView>
        <View style={styles.heroText}>
          <Text style={styles.eyebrow}>
            {place.neighborhood ? `${categoryLabel} · ${place.neighborhood}` : categoryLabel}
          </Text>
          <Text style={styles.title}>{place.name}</Text>
        </View>
      </View>

      <SafeAreaView edges={["bottom"]} style={styles.rest}>
        <View style={styles.progressSection}>
          <AudioProgressBar
            progress={progress}
            onSeek={(fraction) => {
              if (status.duration > 0) player.seekTo(fraction * status.duration);
            }}
          />
          <View style={styles.timeRow}>
            <Text style={styles.timeText}>{formatTime(status.currentTime)}</Text>
            <Text style={styles.timeText}>
              {canPlay
                ? formatTime(status.duration)
                : audio.state === "pending"
                  ? t.placeDetail.narrationPending
                  : t.placeDetail.narrationUnavailable}
            </Text>
          </View>
        </View>

        <View style={styles.playRow}>
          <Pressable
            style={[styles.playBtn, !canPlay && styles.playBtnDisabled]}
            disabled={!canPlay}
            onPress={() => (status.playing ? player.pause() : player.play())}
          >
            {status.playing ? (
              <Svg width={20} height={20} viewBox="0 0 24 24" fill={colors.cream}>
                <Path d="M6 4h4v16H6zM14 4h4v16h-4z" />
              </Svg>
            ) : (
              <Svg width={20} height={20} viewBox="0 0 24 24" fill={colors.cream}>
                <Path d="M6 4l14 8-14 8V4z" />
              </Svg>
            )}
          </Pressable>
        </View>

        {place.narrationStatus === "ready" ? (
          <>
            <View style={styles.toggleRow}>
              <Pressable
                style={[styles.toggleBtn, narrationMode === "scroll" && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("scroll")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Line x1={4} y1={7} x2={20} y2={7} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={12} x2={16} y2={12} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                  <Line x1={4} y1={17} x2={12} y2={17} stroke={narrationMode === "scroll" ? colors.terracotta : DIM_45} strokeWidth={2} strokeLinecap="round" />
                </Svg>
              </Pressable>
              <Pressable
                style={[styles.toggleBtn, narrationMode === "free" && styles.toggleBtnActive]}
                onPress={() => setNarrationMode("free")}
              >
                <Svg width={17} height={17} viewBox="0 0 24 24" fill="none">
                  <Path d="M4 5.5C4 5.5 6 4.5 9 4.5S13 5.5 13 5.5V18.5C13 18.5 11 17.5 9 17.5S4 18.5 4 18.5V5.5Z" stroke={narrationMode === "free" ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                  <Path d="M20 5.5C20 5.5 18 4.5 15 4.5S11 5.5 11 5.5V18.5C11 18.5 13 17.5 15 17.5S20 18.5 20 18.5V5.5Z" stroke={narrationMode === "free" ? colors.terracotta : DIM_45} strokeWidth={1.7} strokeLinejoin="round" />
                </Svg>
              </Pressable>
            </View>

            <View style={styles.lyricsArea}>
              <SyncedNarration
                text={place.body}
                marks={marks}
                currentTimeMs={status.currentTime * 1000}
                mode={narrationMode}
              />
            </View>

            <View style={styles.ground}>
              <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
                <Polyline points="5 13 10 18 19 7" stroke={colors.groundText} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
              </Svg>
              <Text style={styles.groundText}>{t.placeDetail.groundBadge}</Text>
            </View>
          </>
        ) : (
          <View style={styles.lyricsArea}>
            <Text style={styles.pendingText}>
              {place.narrationStatus === "pending"
                ? t.placeDetail.narrationPending
                : t.placeDetail.narrationUnavailable}
            </Text>
          </View>
        )}

        <Pressable
          style={styles.ask}
          onPress={() => navigation.navigate("Assistant", { placeId: place.id })}
        >
          <Svg width={20} height={20} viewBox="0 0 24 24" fill="none">
            <Path
              d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
              stroke={colors.terracotta}
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </Svg>
          <Text style={styles.askText}>{t.placeDetail.ask}</Text>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="9 6 15 12 9 18" stroke={DIM_45} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </SafeAreaView>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: DARK_BG },
  hero: {
    height: 320,
    backgroundColor: colors.sand,
    justifyContent: "space-between",
    // Without this, react-native-web's absolutely-positioned cover Image
    // (and its gradient overlay) can render taller than this fixed-height
    // container and bleed into the content below on web -- native clips
    // this automatically, web doesn't.
    overflow: "hidden",
  },
  back: {
    marginLeft: 18,
    marginTop: 8,
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(28,14,7,0.4)",
    alignItems: "center",
    justifyContent: "center",
  },
  heroText: { paddingHorizontal: 20, paddingBottom: 18 },
  eyebrow: {
    fontFamily: fonts.bodyBold,
    fontSize: 11.5,
    letterSpacing: 1,
    textTransform: "uppercase",
    color: "rgba(250,245,238,0.85)",
    marginBottom: 6,
  },
  title: { fontFamily: fonts.displayBlack, fontSize: 30, color: colors.cream },
  rest: { flex: 1 },
  progressSection: { paddingHorizontal: 20, paddingTop: 24 },
  timeRow: { flexDirection: "row", justifyContent: "space-between", marginTop: 8 },
  timeText: { fontFamily: fonts.body, fontSize: 12, color: DIM_55 },
  playRow: { alignItems: "center", marginTop: 20 },
  playBtn: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  playBtnDisabled: { backgroundColor: "rgba(193,89,46,0.35)" },
  toggleRow: { flexDirection: "row", justifyContent: "flex-end", gap: 8, paddingHorizontal: 20, marginTop: 28 },
  toggleBtn: { width: 34, height: 34, borderRadius: radii.md, alignItems: "center", justifyContent: "center" },
  toggleBtnActive: { backgroundColor: TINT_ACTIVE },
  lyricsArea: { flex: 1, minHeight: 0, paddingHorizontal: 32, justifyContent: "center" },
  pendingText: { fontFamily: fonts.body, fontSize: 15, lineHeight: 24, color: DIM_55, textAlign: "center" },
  ground: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    alignSelf: "flex-start",
    backgroundColor: colors.groundBg,
    borderRadius: radii.pill,
    paddingVertical: 7,
    paddingHorizontal: 12,
    marginHorizontal: 20,
    marginTop: 16,
  },
  groundText: { fontFamily: fonts.bodyBold, fontSize: 12, color: colors.groundText },
  ask: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    borderWidth: 1,
    borderColor: DIM_18,
    borderRadius: radii.md,
    padding: 14,
    marginHorizontal: 20,
    marginTop: 16,
    marginBottom: 12,
  },
  askText: { flex: 1, fontFamily: fonts.bodyBold, fontSize: 14.5, color: colors.cream },
});
```

- [ ] **Step 2: Type-check**

Run: `cd mobile && npx tsc --noEmit`
Expected: no type errors — this is the call site Task 4 warned would be broken until now; it should be
clean at this point.

- [ ] **Step 3: Manually verify in the app**

Per this project's UI-change convention, start the app and manually verify against Cristo Redentor with
real Polly-generated audio+timestamps (all 4 languages, already generated):
- The progress bar fills in real time and its thumb position matches actual playback.
- Tapping anywhere on the bar seeks to that position; dragging the thumb scrubs live and commits the
  seek on release.
- Play/pause button toggles correctly and reflects `status.playing`.
- "Défilement doux" mode shows exactly 3 lines (dim/bold/dim) advancing in sync with the audio, with no
  single-word orphan lines and no visible position jump at the very start or end of the narration.
- Tapping the other toggle icon switches to "lecture libre": full static text, scrollable by hand,
  completely unaffected by continued playback.
- Switching the app's language in Settings and reopening this place plays the correct language's audio
  and text — there is no language control left on this screen itself.
- A place/language with no timestamps yet (or before playback starts) still renders sensibly: static
  text, no highlighting artifacts, no crash.

- [ ] **Step 4: Commit**

```bash
git add mobile/src/screens/PlaceDetail.tsx
git commit -m "mobile: PlaceDetail becomes an immersive dark now-playing screen"
```

---

## After this plan

Not covered here (explicitly out of scope per the spec):
- Real per-place photos (Wikidata P18 sourcing through the pipeline) — generic per-category placeholder
  stays for now.
- Tap-a-line-to-seek — line start times are already available (`NarrationLine.time`), so this is
  straightforward to add later without revisiting this plan's architecture.
- Anything below the fold on this screen (related/nearby places was floated as an idea, not decided).
- Persisting the "défilement doux"/"lecture libre" choice across app restarts.
- The `Map` and `Assistant` screen redesigns — separate sub-projects, agreed to follow this one.
