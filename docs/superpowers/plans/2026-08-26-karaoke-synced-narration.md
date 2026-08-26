# Karaoke-Style Synced Narration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface Amazon Polly's word-level timestamps (generated but unused since the Polly
integration landed) through the API and into the mobile app's `PlaceDetail` screen as a
Spotify-lyrics-style synced narration: the current word highlighted, the view auto-scrolling to keep
it visible, falling back silently to today's static text whenever timestamps aren't available.

**Architecture:** Backend gains one optional field (`timestamps_url`) on the existing
`GET /places/:id/audio` response, presigned the same way the audio URL already is — no new route, no
schema change. Mobile fetches that NDJSON file once audio is ready, parses it with pure functions
(tested in isolation, no rendering involved), and a new `SyncedNarration` component renders the
narration either as today's static `Text` (no marks) or as one `Text` per word inside a wrapping
`View` with the active word highlighted and the scroll position nudged to keep it visible.

**Tech Stack:** Go 1.25.0 (backend, no new dependency), TypeScript/React Native/Expo (mobile, no new
dependency — no test-rendering library is installed in this project, matching the existing convention
of testing pure logic only, e.g. `downloadManager.test.ts`).

**Spec:** `docs/superpowers/specs/2026-08-26-karaoke-synced-narration-design.md`

**Branch:** Continues on the current branch (`worktree-feature+aws-polly-tts`) — this is additive work
on top of the already-landed Polly integration, not a separate feature branch.

## Global Constraints

- Backend: Go 1.25.0, module `rioaudioguide/backend`. No `internal/domain/` or `internal/ports/`
  changes — this plan only touches `internal/adapters/http/audio_handler.go` and its test.
- Mobile: no new npm dependency. `SyncedNarration` is not unit-tested as a component (no
  `@testing-library/react-native` in this project) — only the pure logic it depends on
  (`src/utils/syncedText.ts`) gets Jest coverage, per the project's existing pure/IO separation
  convention.
- The displayed karaoke text is built FROM the marks' `value` fields, never by aligning character
  offsets against `place.body` — mark offsets are relative to the SSML-wrapped text Polly actually
  synthesized (`<speak><prosody rate="90%">...`), not the raw narration text. `place.body` is used
  only before playback starts and in the no-marks fallback.
- Auto-scroll re-centers only when the active word leaves a comfortable middle band of the viewport —
  never on every single word (would read as jittery, not "Spotify lyrics").
- No tap-to-seek in this plan (explicitly out of scope per the spec).

---

### Task 1: Backend — presign `timestamps_url` on `GET /places/:id/audio`

**Files:**
- Modify: `internal/adapters/http/audio_handler.go`
- Modify: `internal/adapters/http/audio_handler_test.go`

**Interfaces:**
- Produces: `audioResponse.TimestampsURL *string` (JSON key `timestamps_url`, `omitempty`) — consumed
  by the mobile `PlacesRepository.getAudioUrl` in Task 2.

- [ ] **Step 1: Write the failing tests**

Add two tests to `internal/adapters/http/audio_handler_test.go`, following the exact pattern of the
existing `TestGetPlaceAudio_Ready` (same setup: place, script marked reviewed+published, audio file
marked ready, server built with `fakeAudioStorage{}` whose `PresignURL` returns
`"https://presigned.example.com/" + key + "?X-Amz-Signature=fake"`).

```go
func TestGetPlaceAudio_Ready_IncludesPresignedTimestampsURLWhenPresent(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(placeName, "monument", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = script.Publish()

	audio, _ := domain.NewGeneratedAudio("s3://rio-audio-guide/abc123.mp3", "s3://rio-audio-guide/abc123.marks", 30*time.Second)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady(audio)

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo, newFakeUserRepo(),
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"timestamps_url":"https://presigned.example.com/abc123.marks?X-Amz-Signature=fake"`) {
		t.Fatalf("expected a presigned timestamps_url in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_Ready_OmitsTimestampsURLWhenAbsent(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(placeName, "monument", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = script.Publish()

	// timestampsURL vide -- reproduit une génération ElevenLabs d'avant Polly.
	audio, _ := domain.NewGeneratedAudio("s3://rio-audio-guide/abc123.mp3", "", 30*time.Second)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady(audio)

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo, newFakeUserRepo(),
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// La clé elle-même doit être absente (omitempty), pas juste vide -- un
	// simple `"timestamps_url":""` serait un contrat différent (le client
	// devrait alors distinguer "absent" de "vide", inutilement).
	if strings.Contains(rec.Body.String(), "timestamps_url") {
		t.Fatalf("expected no timestamps_url key at all, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/http/... -run TestGetPlaceAudio_Ready -v`
Expected: FAIL — `TestGetPlaceAudio_Ready_IncludesPresignedTimestampsURLWhenPresent` fails because
`audioResponse` has no `timestamps_url` key yet (the string `strings.Contains` check finds nothing).
The second test (`OmitsTimestampsURLWhenAbsent`) will actually PASS even before the change (there's
nothing to omit yet) — that's expected and fine, it's a regression guard for after Step 3, not a
red/green pair on its own.

- [ ] **Step 3: Write the implementation**

In `internal/adapters/http/audio_handler.go`:

```go
type audioResponse struct {
	URL           string  `json:"url"`
	TimestampsURL *string `json:"timestamps_url,omitempty"`
}
```

Replace the response-building block (currently `body, err := json.Marshal(audioResponse{URL: url})`)
with:

```go
	resp := audioResponse{URL: url}
	if timestampsStorageURL := audioFile.Audio().TimestampsURL(); timestampsStorageURL != "" {
		timestampsKey, err := parseS3Key(timestampsStorageURL)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
		}
		timestampsURL, err := s.storage.PresignURL(c.Request().Context(), timestampsKey, presignExpiry)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
		}
		resp.TimestampsURL = &timestampsURL
	}

	body, err := json.Marshal(resp)
```

(`parseS3Key` and `presignExpiry` are both already defined in this file, reused as-is.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS — every test in the package, including both new ones and the pre-existing
`TestGetPlaceAudio_Ready` (which passes an empty `timestampsURL` via `NewGeneratedAudio(..., "",
...)`, so it continues exercising the omit path too).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/audio_handler.go internal/adapters/http/audio_handler_test.go
git commit -m "http: presign timestamps_url on GET /places/:id/audio when available"
```

---

### Task 2: Mobile — carry `timestampsUrl` through `AudioAvailability`

**Files:**
- Modify: `mobile/src/data/types.ts`
- Modify: `mobile/src/data/PlacesRepository.ts`

**Interfaces:**
- Produces: `AudioAvailability`'s `"ready"` variant gains `timestampsUrl?: string` — consumed by
  `PlaceDetail.tsx` in Task 5 to decide whether to fetch marks.

- [ ] **Step 1: Edit `AudioAvailability`**

In `mobile/src/data/types.ts`, replace:

```ts
export type AudioAvailability =
  | { state: "ready"; url: string }
  | { state: "pending" }
  | { state: "unavailable" };
```

with:

```ts
export type AudioAvailability =
  | { state: "ready"; url: string; timestampsUrl?: string }
  | { state: "pending" }
  | { state: "unavailable" };
```

- [ ] **Step 2: Edit `HttpPlacesRepository.getAudioUrl`**

In `mobile/src/data/PlacesRepository.ts`, replace:

```ts
  async getAudioUrl(placeId: string, language: Locale): Promise<AudioAvailability> {
    const { status, body } = await fetchJson<{ url?: string }>(
      `/places/${encodeURIComponent(placeId)}/audio?language=${language}`,
    );
    if (status === 200 && body?.url) {
      return { state: "ready", url: body.url };
    }
    if (status === 202) {
      return { state: "pending" };
    }
    return { state: "unavailable" };
  }
```

with:

```ts
  async getAudioUrl(placeId: string, language: Locale): Promise<AudioAvailability> {
    const { status, body } = await fetchJson<{ url?: string; timestamps_url?: string }>(
      `/places/${encodeURIComponent(placeId)}/audio?language=${language}`,
    );
    if (status === 200 && body?.url) {
      return { state: "ready", url: body.url, timestampsUrl: body.timestamps_url };
    }
    if (status === 202) {
      return { state: "pending" };
    }
    return { state: "unavailable" };
  }
```

(`body.timestamps_url` is `undefined` when the backend omits the key, which is exactly the "no
timestamps" case — no extra branching needed.)

- [ ] **Step 3: Type-check**

Run: `cd mobile && npx tsc --noEmit`
Expected: no new type errors (this task has no runtime behavior to test — `MockPlacesRepository`'s
`getAudioUrl` already returns `{state: "unavailable"}` unconditionally, unaffected by the type
widening).

- [ ] **Step 4: Commit**

```bash
git add mobile/src/data/types.ts mobile/src/data/PlacesRepository.ts
git commit -m "mobile: carry timestampsUrl through AudioAvailability"
```

---

### Task 3: Mobile — pure sync logic (`src/utils/syncedText.ts`)

**Files:**
- Create: `mobile/src/utils/syncedText.ts`
- Test: `mobile/src/utils/__tests__/syncedText.test.ts`

**Interfaces:**
- Produces: `WordMark` type, `parseWordMarks(ndjson: string): WordMark[]`,
  `findActiveWordIndex(marks: WordMark[], currentTimeMs: number): number`,
  `fetchWordMarks(url: string): Promise<WordMark[] | null>` — consumed by `PlaceDetail.tsx` (Task 5)
  and `SyncedNarration` (Task 4, receives `WordMark[] | null` and `findActiveWordIndex` as props/logic,
  doesn't call `fetchWordMarks` itself).

- [ ] **Step 1: Write the failing tests**

```ts
// mobile/src/utils/__tests__/syncedText.test.ts
import { parseWordMarks, findActiveWordIndex } from "../syncedText";

describe("parseWordMarks", () => {
  it("parses one word mark per line", () => {
    const ndjson = [
      '{"time":25,"type":"word","start":27,"end":29,"value":"Au"}',
      '{"time":136,"type":"word","start":30,"end":36,"value":"sommet"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([
      { time: 25, value: "Au" },
      { time: 136, value: "sommet" },
    ]);
  });

  it("skips non-word mark types", () => {
    const ndjson = [
      '{"time":0,"type":"sentence","start":0,"end":10,"value":"Au sommet."}',
      '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([{ time: 25, value: "Au" }]);
  });

  it("skips a corrupted line without failing the whole file", () => {
    const ndjson = [
      '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}',
      "not json at all",
      '{"time":136,"type":"word","start":3,"end":9,"value":"sommet"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([
      { time: 25, value: "Au" },
      { time: 136, value: "sommet" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(parseWordMarks("")).toEqual([]);
  });

  it("ignores blank lines", () => {
    const ndjson = '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}\n\n';
    expect(parseWordMarks(ndjson)).toEqual([{ time: 25, value: "Au" }]);
  });
});

describe("findActiveWordIndex", () => {
  const marks = [
    { time: 0, value: "Au" },
    { time: 100, value: "sommet" },
    { time: 250, value: "du" },
  ];

  it("returns -1 before the first mark", () => {
    expect(findActiveWordIndex(marks, -1)).toBe(-1);
  });

  it("returns the first index exactly at its own timestamp", () => {
    expect(findActiveWordIndex(marks, 0)).toBe(0);
  });

  it("returns the previous index between two timestamps", () => {
    expect(findActiveWordIndex(marks, 150)).toBe(1);
  });

  it("returns the last index once past the final timestamp", () => {
    expect(findActiveWordIndex(marks, 10000)).toBe(2);
  });

  it("returns -1 for an empty marks array", () => {
    expect(findActiveWordIndex([], 500)).toBe(-1);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts`
Expected: FAIL — `../syncedText` module doesn't exist yet.

- [ ] **Step 3: Write the implementation**

```ts
// mobile/src/utils/syncedText.ts

export type WordMark = { time: number; value: string };

type RawMark = { time?: number; type?: string; value?: string };

// Découpe le NDJSON en marks de type "word" uniquement -- une ligne
// corrompue (JSON invalide) est ignorée plutôt que de faire échouer tout le
// fichier : un seul mot mal timé ne doit pas priver toute la narration de
// surlignage.
export function parseWordMarks(ndjson: string): WordMark[] {
  const marks: WordMark[] = [];
  for (const line of ndjson.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let raw: RawMark;
    try {
      raw = JSON.parse(trimmed) as RawMark;
    } catch {
      continue;
    }
    if (raw.type !== "word" || typeof raw.time !== "number" || typeof raw.value !== "string") {
      continue;
    }
    marks.push({ time: raw.time, value: raw.value });
  }
  return marks;
}

// Recherche binaire : le dernier indice dont marks[i].time <= currentTimeMs.
// -1 si currentTimeMs précède le premier mark, ou si marks est vide.
export function findActiveWordIndex(marks: WordMark[], currentTimeMs: number): number {
  if (marks.length === 0 || currentTimeMs < marks[0].time) return -1;
  let lo = 0;
  let hi = marks.length - 1;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (marks[mid].time <= currentTimeMs) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo;
}

// fetchWordMarks ne lève jamais -- toute erreur réseau ou de parsing devient
// null, qui déclenche le fallback texte statique côté SyncedNarration sans
// état d'erreur séparé à gérer.
export async function fetchWordMarks(url: string): Promise<WordMark[] | null> {
  try {
    const res = await fetch(url);
    if (!res.ok) return null;
    const text = await res.text();
    return parseWordMarks(text);
  } catch {
    return null;
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd mobile && npx jest src/utils/__tests__/syncedText.test.ts`
Expected: PASS — all cases.

- [ ] **Step 5: Commit**

```bash
git add mobile/src/utils/syncedText.ts mobile/src/utils/__tests__/syncedText.test.ts
git commit -m "mobile: pure NDJSON mark parsing + active-word lookup for synced narration"
```

---

### Task 4: Mobile — `SyncedNarration` component

**Files:**
- Create: `mobile/src/components/SyncedNarration.tsx`

**Interfaces:**
- Consumes: `WordMark`, `findActiveWordIndex` (Task 3).
- Produces: `SyncedNarration` component, consumed by `PlaceDetail.tsx` in Task 5.

- [ ] **Step 1: Write the component**

No TDD here (no component-rendering test harness in this project, per Global Constraints) — write it
directly, then verify by running the app (Task 5's manual verification step covers this too).

```tsx
// mobile/src/components/SyncedNarration.tsx
import React, { useEffect, useRef } from "react";
import { View, Text, ScrollView, StyleSheet, type LayoutChangeEvent } from "react-native";
import { findActiveWordIndex, type WordMark } from "../utils/syncedText";
import { colors, fonts } from "../theme/tokens";

type Props = {
  text: string;
  marks: WordMark[] | null;
  currentTimeMs: number;
};

// Fraction de la hauteur visible qui délimite la "bande confortable" au
// centre -- le scroll ne bouge que quand le mot actif en sort, pas à chaque
// mot (ça donnerait un défilement saccadé plutôt que l'effet "paroles
// synchronisées" recherché).
const COMFORT_BAND_TOP = 0.3;
const COMFORT_BAND_BOTTOM = 0.7;

export function SyncedNarration({ text, marks, currentTimeMs }: Props) {
  const scrollRef = useRef<ScrollView>(null);
  const scrollYRef = useRef(0);
  const viewportHeightRef = useRef(0);
  const wordYPositions = useRef<number[]>([]);

  const activeIndex = marks ? findActiveWordIndex(marks, currentTimeMs) : -1;

  useEffect(() => {
    if (activeIndex < 0) return;
    const wordY = wordYPositions.current[activeIndex];
    if (wordY === undefined) return;
    const viewportHeight = viewportHeightRef.current;
    if (viewportHeight === 0) return;
    const bandTop = scrollYRef.current + viewportHeight * COMFORT_BAND_TOP;
    const bandBottom = scrollYRef.current + viewportHeight * COMFORT_BAND_BOTTOM;
    if (wordY < bandTop || wordY > bandBottom) {
      scrollRef.current?.scrollTo({
        y: Math.max(0, wordY - viewportHeight * COMFORT_BAND_TOP),
        animated: true,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- ne doit
    // re-déclencher que sur un changement de mot actif, pas à chaque frame.
  }, [activeIndex]);

  if (!marks) {
    return <Text style={styles.body}>{text}</Text>;
  }

  return (
    <ScrollView
      ref={scrollRef}
      onScroll={(e) => {
        scrollYRef.current = e.nativeEvent.contentOffset.y;
      }}
      onLayout={(e) => {
        viewportHeightRef.current = e.nativeEvent.layout.height;
      }}
      scrollEventThrottle={100}
    >
      <View style={styles.wordWrap}>
        {marks.map((mark, i) => (
          <Text
            key={i}
            onLayout={(e: LayoutChangeEvent) => {
              wordYPositions.current[i] = e.nativeEvent.layout.y;
            }}
            style={[styles.body, i === activeIndex && styles.activeWord]}
          >
            {mark.value + " "}
          </Text>
        ))}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  body: {
    fontFamily: fonts.body,
    fontSize: 15,
    lineHeight: 24,
    color: colors.inkSoft,
  },
  wordWrap: { flexDirection: "row", flexWrap: "wrap" },
  activeWord: { color: colors.terracotta, fontFamily: fonts.bodyBold },
});
```

Note: `LayoutChangeEvent`'s `layout` field carries `x`/`y` relative to the parent `View`
(`wordWrap`), which itself starts at the top of the `ScrollView`'s content — so `wordY` and
`scrollYRef.current` are in the same coordinate space, no offset correction needed.

- [ ] **Step 2: Type-check**

Run: `cd mobile && npx tsc --noEmit`
Expected: no new type errors.

- [ ] **Step 3: Commit**

```bash
git add mobile/src/components/SyncedNarration.tsx
git commit -m "mobile: SyncedNarration component (word highlight + keep-in-view scroll)"
```

---

### Task 5: Mobile — wire into `PlaceDetail.tsx`

**Files:**
- Modify: `mobile/src/screens/PlaceDetail.tsx`

**Interfaces:**
- Consumes: `AudioAvailability.timestampsUrl` (Task 2), `fetchWordMarks`/`WordMark` (Task 3),
  `SyncedNarration` (Task 4).

- [ ] **Step 1: Add imports and marks state**

At the top of `mobile/src/screens/PlaceDetail.tsx`, add:

```ts
import { fetchWordMarks, type WordMark } from "../utils/syncedText";
import { SyncedNarration } from "../components/SyncedNarration";
```

Inside `PlaceDetailScreen`, alongside the existing `audio` state, add:

```ts
const [marks, setMarks] = useState<WordMark[] | null>(null);
```

- [ ] **Step 2: Fetch marks whenever ready audio changes**

Add a new effect right after the existing `getAudioUrl` effect (which sets `audio`):

```ts
useEffect(() => {
  // Vidé immédiatement, avant même que le fetch ne réponde -- sinon les
  // marks de l'ancienne langue resteraient affichées un instant pendant la
  // transition, surlignant les mauvais mots.
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
```

(Depending on `audio` as a whole, not just `audio.state`/`audio.timestampsUrl` individually, matches
how the existing `getAudioUrl` effect above it already depends on `place?.id`/`playerLocale` — keeps
the same style. `audio` is a fresh object on every fetch resolution, including when it resolves to the
same values, so this re-runs on every language switch as intended.)

- [ ] **Step 3: Replace the static narration text with `SyncedNarration`**

Replace:

```tsx
      {place.narrationStatus === "ready" ? (
        <>
          <View style={styles.ground}>
            <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
              <Polyline points="5 13 10 18 19 7" stroke={colors.groundText} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
            <Text style={styles.groundText}>{t.placeDetail.groundBadge}</Text>
          </View>
          <Text style={styles.body}>{place.body}</Text>
        </>
      ) : (
```

with:

```tsx
      {place.narrationStatus === "ready" ? (
        <>
          <View style={styles.ground}>
            <Svg width={12} height={12} viewBox="0 0 24 24" fill="none">
              <Polyline points="5 13 10 18 19 7" stroke={colors.groundText} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
            <Text style={styles.groundText}>{t.placeDetail.groundBadge}</Text>
          </View>
          <View style={{ marginHorizontal: 20, marginTop: 18 }}>
            <SyncedNarration text={place.body} marks={marks} currentTimeMs={status.currentTime * 1000} />
          </View>
        </>
      ) : (
```

(The `marginHorizontal`/`marginTop` move from the old `styles.body` — which `SyncedNarration` no
longer owns the outer spacing for — onto a wrapping `View`, so the screen's layout is pixel-identical
to before. `styles.body` itself stays defined in `PlaceDetail.tsx`'s stylesheet only if still used
elsewhere in the file; check with a search — if this was its only use, remove the now-dead
`styles.body` entry rather than leaving unused styles behind.)

- [ ] **Step 4: Type-check and manually verify in the app**

Run: `cd mobile && npx tsc --noEmit`
Expected: no new type errors.

Per this project's UI-change convention, start the app (`npx expo start`, or however this project's
`run` skill launches it) and manually verify against a place with real Polly-generated audio+timestamps
(Cristo Redentor, all 4 languages, already generated this session):
- Word highlighting tracks playback and the view auto-scrolls to keep the active word visible, without
  visibly jittering on every word.
- Switching the language pill mid-playback doesn't show stale highlighting from the previous language.
- A place/language with no timestamps (or before playback starts) renders exactly as before —
  plain static text, no highlighting artifacts.

- [ ] **Step 5: Commit**

```bash
git add mobile/src/screens/PlaceDetail.tsx
git commit -m "mobile: wire SyncedNarration into PlaceDetail for karaoke-style playback"
```

---

## After this plan

Not covered here (explicitly out of scope per the spec):
- Tap-to-seek (word → playback position) — the marks/word mapping this plan builds doesn't block
  adding it later.
- Offline timestamps — `downloadManager.ts` doesn't download audio bytes yet either, so nothing to
  synchronize offline until that exists.
