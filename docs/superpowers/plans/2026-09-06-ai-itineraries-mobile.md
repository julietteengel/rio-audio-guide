# AI Itineraries Mobile Screens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the three mobile screens the AI Itineraries feature's approved design calls for —
"Mes itinéraires" (list), a free-text chat creation flow, and a vertical-timeline detail view — wired to
the already-merged backend (`POST/GET /itineraries`, `GET /itineraries/:id`).

**Architecture:** Three new screens added to the existing single-stack navigation
(`AppStackParamList`/`AppNavigator.tsx`), no tab bar. A new `ItinerariesRepository.ts` API client
following the exact pattern already established by `AssistantRepository.ts` (token-bearing `fetch`,
snake_case wire fields mapped to camelCase TS types). No client-side LLM calls — the backend holds the
Claude integration, matching every other AI feature in this app.

**Tech Stack:** React Native / Expo (existing app conventions), no new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-06-ai-itineraries-design.md`

## Global Constraints

- All three screens use the app's existing **light/warm theme** (cream/terracotta, Playfair Display +
  Inter) — explicitly **not** `PlaceDetail`'s dark immersive theme, which the spec reserves for audio
  playback only. Reuse `mobile/src/theme/tokens.ts`'s existing `colors`/`fonts`/`radii` exports; do not
  invent new colors.
- The backend's `POST /itineraries` takes **only** `{"request": string}` — no `language` field exists on
  this route (unlike `/places/:id/audio` or `/places/:id/assistant`). The itinerary's title and stop
  labels come back in whatever language the model infers from the free-text request itself; this plan
  does not add language control to the backend, which doesn't support it yet.
- The meal-break slot (`kind: "suggestion"` in the wire response) must **never** receive the same visual
  treatment as a real place stop (`kind: "place"`) anywhere it's rendered — no shared numbered badge, no
  "✓ vérifié" styling. This is called out in the spec as "the feature's one real trust-distinction
  requirement carried into the UI" — treat it as load-bearing, not decorative, in every screen that shows
  stops.
- No screen in this plan gets a component test, matching every other screen in this app (see
  `docs/superpowers/specs/2026-09-06-ai-itineraries-design.md`'s own Testing section). Pure logic
  (duration formatting, meal-break detection) is extracted into small testable functions instead.
- `PlaceDetail.tsx`'s existing "Ajouter à un itinéraire" button stays a placeholder
  (`t.placeDetail.itineraryComingSoonTitle`/`itineraryComingSoonBody`) — there is no backend action to
  add a single existing place to an already-created itinerary (the only creation path is the AI chat
  flow below), so wiring that button to something real is out of scope here and left for a future
  plan once that backend action exists.

---

### Task 1: Repository, navigation types, and dictionary

**Files:**
- Create: `mobile/src/data/ItinerariesRepository.ts`
- Create: `mobile/src/utils/itineraryFormat.ts`
- Test: `mobile/src/utils/__tests__/itineraryFormat.test.ts`
- Modify: `mobile/src/navigation/types.ts`
- Modify: `mobile/src/i18n/dictionary.ts`

**Interfaces:**
- Produces: `createItinerary(token, request: string): Promise<Itinerary>`,
  `listItineraries(token): Promise<Itinerary[]>`, `getItinerary(token, id): Promise<Itinerary>`,
  `ItinerariesApiError` (mirrors `AssistantApiError`), the `Itinerary`/`ItineraryStop` TS types, and
  `formatDuration(totalMinutes: number): string` — consumed by Tasks 2-4.
- Produces: `AppStackParamList` gains `ItinerariesList: undefined`, `ItineraryChat: undefined`,
  `ItineraryDetail: { itineraryId: string }` — consumed by every later task and by Task 2's entry-point
  wiring in `Map.tsx`/`Settings.tsx`.

- [ ] **Step 1: Write the failing test for `formatDuration`**

```typescript
// mobile/src/utils/__tests__/itineraryFormat.test.ts
import { formatDuration } from "../itineraryFormat";

test("formats sub-hour durations as plain minutes", () => {
  expect(formatDuration(0)).toBe("0 min");
  expect(formatDuration(45)).toBe("45 min");
});

test("formats hour-plus durations as Nh + remaining minutes", () => {
  expect(formatDuration(60)).toBe("1h");
  expect(formatDuration(90)).toBe("1h30");
  expect(formatDuration(125)).toBe("2h05");
});
```

Run: `cd mobile && npx jest src/utils/__tests__/itineraryFormat.test.ts`
Expected: FAIL (module doesn't exist yet).

- [ ] **Step 2: Write `itineraryFormat.ts`**

```typescript
// mobile/src/utils/itineraryFormat.ts

// "1h" / "1h30" / "2h05" / "45 min" -- matches how duration is written
// colloquially in fr/en/pt/es alike (unlike most copy in this app, this
// abbreviation doesn't need a per-locale dictionary entry).
export function formatDuration(totalMinutes: number): string {
  if (totalMinutes < 60) return `${totalMinutes} min`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (minutes === 0) return `${hours}h`;
  return `${hours}h${minutes.toString().padStart(2, "0")}`;
}
```

- [ ] **Step 3: Run the test to verify it passes**

Run: `cd mobile && npx jest src/utils/__tests__/itineraryFormat.test.ts`
Expected: PASS.

- [ ] **Step 4: Write `ItinerariesRepository.ts`**

```typescript
// mobile/src/data/ItinerariesRepository.ts
import { API_BASE_URL } from "../config";

export type ItineraryStopKind = "place" | "suggestion";

export type ItineraryStop = {
  kind: ItineraryStopKind;
  placeId?: string;
  label: string;
  timeOnSiteMinutes: number;
  walkToNextMinutes: number;
};

export type Itinerary = {
  id: string;
  title: string;
  totalMinutes: number;
  placeCount: number;
  stops: ItineraryStop[];
};

export class ItinerariesApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

type WireStop = {
  kind: string;
  place_id?: string;
  label: string;
  time_on_site_minutes: number;
  walk_to_next_minutes: number;
};

type WireItinerary = {
  id: string;
  title: string;
  total_minutes: number;
  place_count: number;
  stops: WireStop[];
};

function fromWire(wire: WireItinerary): Itinerary {
  return {
    id: wire.id,
    title: wire.title,
    totalMinutes: wire.total_minutes,
    placeCount: wire.place_count,
    stops: wire.stops.map((s) => ({
      kind: s.kind === "suggestion" ? "suggestion" : "place",
      placeId: s.place_id,
      label: s.label,
      timeOnSiteMinutes: s.time_on_site_minutes,
      walkToNextMinutes: s.walk_to_next_minutes,
    })),
  };
}

async function itinerariesFetch<T>(
  path: string,
  options: { method: string; token: string; body?: unknown },
): Promise<T> {
  const headers: Record<string, string> = { Authorization: `Bearer ${options.token}` };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method,
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }

  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : "request failed";
    throw new ItinerariesApiError(message, res.status);
  }

  return body as T;
}

export async function createItinerary(token: string, request: string): Promise<Itinerary> {
  const wire = await itinerariesFetch<WireItinerary>("/itineraries", {
    method: "POST",
    token,
    body: { request },
  });
  return fromWire(wire);
}

export async function listItineraries(token: string): Promise<Itinerary[]> {
  const wire = await itinerariesFetch<WireItinerary[]>("/itineraries", { method: "GET", token });
  return wire.map(fromWire);
}

export async function getItinerary(token: string, id: string): Promise<Itinerary> {
  const wire = await itinerariesFetch<WireItinerary>(`/itineraries/${encodeURIComponent(id)}`, {
    method: "GET",
    token,
  });
  return fromWire(wire);
}
```

- [ ] **Step 5: Add the three new routes to `AppStackParamList`**

In `mobile/src/navigation/types.ts`, change:

```typescript
export type AppStackParamList = {
  Map: undefined;
  PlaceDetail: { placeId: string };
  Assistant: { placeId: string };
  Settings: undefined;
  Auth: undefined;
  EditProfile: undefined;
};
```

to:

```typescript
export type AppStackParamList = {
  Map: undefined;
  PlaceDetail: { placeId: string };
  Assistant: { placeId: string };
  Settings: undefined;
  Auth: undefined;
  EditProfile: undefined;
  ItinerariesList: undefined;
  ItineraryChat: undefined;
  ItineraryDetail: { itineraryId: string };
};
```

- [ ] **Step 6: Add the `itineraries` dictionary namespace, all four locales**

In `mobile/src/i18n/dictionary.ts`, add a new `itineraries` block immediately after each locale's
`assistant` block (i.e. 4 insertions, one per locale — do not skip any).

`fr`:

```typescript
    itineraries: {
      listTitle: "Mes itinéraires",
      listEmptyTitle: "Aucun itinéraire pour l'instant",
      listEmptyBody: "Décris ce que tu as envie de faire, l'IA te propose un parcours à partir des lieux vérifiés du guide.",
      createButton: "Créer un itinéraire",
      stopCount: "{count} lieux",
      chatTitle: "Créer un itinéraire",
      chatSubtitle: "Décris ta contrainte de temps, ton thème, ton quartier — l'IA compose un parcours à partir des lieux vérifiés du guide.",
      chatInputPlaceholder: "Ex. : 1h à Santa Teresa, focus street art…",
      chatSendError: "La génération a échoué. Réessaie.",
      viewFullItinerary: "Voir l'itinéraire complet",
      suggestionLabel: "Suggestion de l'IA, non vérifiée",
      walkToNext: "{minutes} min à pied",
      detailNotFound: "Cet itinéraire est introuvable.",
    },
```

`en`:

```typescript
    itineraries: {
      listTitle: "My itineraries",
      listEmptyTitle: "No itineraries yet",
      listEmptyBody: "Describe what you'd like to do, and the AI will propose a route built from the guide's verified places.",
      createButton: "Create an itinerary",
      stopCount: "{count} places",
      chatTitle: "Create an itinerary",
      chatSubtitle: "Describe your time budget, theme, or neighborhood — the AI composes a route from the guide's verified places.",
      chatInputPlaceholder: "E.g.: 1h in Santa Teresa, focus on street art…",
      chatSendError: "Generation failed. Please try again.",
      viewFullItinerary: "View full itinerary",
      suggestionLabel: "AI suggestion, unverified",
      walkToNext: "{minutes} min walk",
      detailNotFound: "This itinerary could not be found.",
    },
```

`pt`:

```typescript
    itineraries: {
      listTitle: "Meus roteiros",
      listEmptyTitle: "Nenhum roteiro ainda",
      listEmptyBody: "Descreva o que você quer fazer, e a IA propõe um roteiro a partir dos lugares verificados do guia.",
      createButton: "Criar um roteiro",
      stopCount: "{count} lugares",
      chatTitle: "Criar um roteiro",
      chatSubtitle: "Descreva seu tempo disponível, tema ou bairro — a IA monta um roteiro com lugares verificados do guia.",
      chatInputPlaceholder: "Ex.: 1h em Santa Teresa, foco em arte de rua…",
      chatSendError: "A geração falhou. Tente novamente.",
      viewFullItinerary: "Ver roteiro completo",
      suggestionLabel: "Sugestão da IA, não verificada",
      walkToNext: "{minutes} min a pé",
      detailNotFound: "Este roteiro não foi encontrado.",
    },
```

`es`:

```typescript
    itineraries: {
      listTitle: "Mis itinerarios",
      listEmptyTitle: "Todavía no hay itinerarios",
      listEmptyBody: "Describe lo que quieres hacer y la IA propondrá un recorrido con lugares verificados de la guía.",
      createButton: "Crear un itinerario",
      stopCount: "{count} lugares",
      chatTitle: "Crear un itinerario",
      chatSubtitle: "Describe tu tiempo disponible, tema o barrio — la IA arma un recorrido con lugares verificados de la guía.",
      chatInputPlaceholder: "Ej.: 1h en Santa Teresa, con foco en arte urbano…",
      chatSendError: "La generación falló. Inténtalo de nuevo.",
      viewFullItinerary: "Ver itinerario completo",
      suggestionLabel: "Sugerencia de la IA, no verificada",
      walkToNext: "{minutes} min a pie",
      detailNotFound: "No se encontró este itinerario.",
    },
```

- [ ] **Step 7: Run tsc and the full test suite**

Run: `cd mobile && npx tsc --noEmit && npx jest`
Expected: `tsc` clean (the new `AppStackParamList` entries aren't consumed by any screen yet, which is
fine — no screen references them until Tasks 2-4); all jest suites pass, including the new
`itineraryFormat.test.ts`.

- [ ] **Step 8: Commit**

```bash
cd mobile && git add src/data/ItinerariesRepository.ts src/utils/itineraryFormat.ts src/utils/__tests__/itineraryFormat.test.ts src/navigation/types.ts src/i18n/dictionary.ts
git commit -m "mobile: itineraries API client, nav routes, and dictionary entries"
```

---

### Task 2: "Mes itinéraires" list screen + entry points

**Files:**
- Create: `mobile/src/screens/ItinerariesList.tsx`
- Modify: `mobile/src/navigation/AppNavigator.tsx`
- Modify: `mobile/src/screens/Map.tsx`
- Modify: `mobile/src/screens/Settings.tsx`

**Interfaces:**
- Consumes: `listItineraries` (Task 1), `useAuth()` (existing, returns `{ token }`), `useLocale()`
  (existing, returns `{ t }`), `formatDuration` (Task 1).
- Produces: `ItinerariesListScreen`, registered as route `"ItinerariesList"` — consumed by Task 3's
  "back"/navigation flow and by this task's own entry points.

- [ ] **Step 1: Write `ItinerariesList.tsx`**

```tsx
// mobile/src/screens/ItinerariesList.tsx
import React, { useCallback, useState } from "react";
import { View, Text, Pressable, FlatList, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useFocusEffect } from "@react-navigation/native";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { listItineraries, type Itinerary } from "../data/ItinerariesRepository";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ItinerariesList">;

export function ItinerariesListScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { token } = useAuth();
  const [itineraries, setItineraries] = useState<Itinerary[] | null>(null);

  // Refetches every time this screen regains focus (not just on first
  // mount) -- coming back here after creating a new itinerary in the chat
  // screen should show it without a manual pull-to-refresh.
  useFocusEffect(
    useCallback(() => {
      if (!token) return;
      let cancelled = false;
      listItineraries(token)
        .then((result) => {
          if (!cancelled) setItineraries(result);
        })
        .catch(() => {
          if (!cancelled) setItineraries([]);
        });
      return () => {
        cancelled = true;
      };
    }, [token]),
  );

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
        <Text style={styles.h1}>{t.itineraries.listTitle}</Text>
      </View>

      <Pressable style={styles.createBtn} onPress={() => navigation.navigate("ItineraryChat")}>
        <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
          <Path d="M12 5v14M5 12h14" stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" />
        </Svg>
        <Text style={styles.createBtnText}>{t.itineraries.createButton}</Text>
      </Pressable>

      {itineraries === null ? (
        <ActivityIndicator style={styles.loading} color={colors.terracotta} />
      ) : itineraries.length === 0 ? (
        <View style={styles.empty}>
          <Text style={styles.emptyTitle}>{t.itineraries.listEmptyTitle}</Text>
          <Text style={styles.emptyBody}>{t.itineraries.listEmptyBody}</Text>
        </View>
      ) : (
        <FlatList
          data={itineraries}
          keyExtractor={(item) => item.id}
          contentContainerStyle={styles.list}
          renderItem={({ item }) => (
            <Pressable
              style={styles.card}
              onPress={() => navigation.navigate("ItineraryDetail", { itineraryId: item.id })}
            >
              <Text style={styles.cardTitle}>{item.title}</Text>
              <Text style={styles.cardMeta}>
                {formatDuration(item.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(item.placeCount))}
              </Text>
            </Pressable>
          )}
        />
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", gap: 14, paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  h1: { fontFamily: fonts.display, fontSize: 22, color: colors.ink },
  createBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    backgroundColor: colors.terracotta,
    borderRadius: radii.md,
    marginHorizontal: 20,
    marginTop: 18,
    paddingVertical: 14,
  },
  createBtnText: { fontFamily: fonts.bodyBold, fontSize: 15, color: colors.cream },
  loading: { marginTop: 40 },
  empty: { paddingHorizontal: 32, marginTop: 48, alignItems: "center", gap: 8 },
  emptyTitle: { fontFamily: fonts.bodySemiBold, fontSize: 16, color: colors.ink, textAlign: "center" },
  emptyBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
  list: { paddingHorizontal: 20, paddingTop: 18, paddingBottom: 32, gap: 12 },
  card: {
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    padding: 16,
  },
  cardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 16, color: colors.ink, marginBottom: 4 },
  cardMeta: { fontFamily: fonts.body, fontSize: 13, color: colors.inkSoft },
});
```

- [ ] **Step 2: Register the route in `AppNavigator.tsx`**

In `mobile/src/navigation/AppNavigator.tsx`, add the import:

```typescript
import { ItinerariesListScreen } from "../screens/ItinerariesList";
```

and add the screen entry (anywhere in the `Stack.Navigator`, e.g. after `Settings`):

```tsx
      <Stack.Screen name="ItinerariesList" component={ItinerariesListScreen} />
```

- [ ] **Step 3: Add the entry point in `Map.tsx`'s header**

In `mobile/src/screens/Map.tsx`, find the `headerRight` view (containing the offline badge and the
settings gear button) and add a new icon button **before** the existing gear button:

```tsx
          <Pressable style={styles.gearBtn} onPress={() => navigation.navigate("ItinerariesList")}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Circle cx={6} cy={7} r={2} stroke={colors.ink} strokeWidth={2} />
              <Circle cx={18} cy={17} r={2} stroke={colors.ink} strokeWidth={2} />
              <Path d="M8 8.5 16 15.5" stroke={colors.ink} strokeWidth={2} strokeLinecap="round" />
            </Svg>
          </Pressable>
```

(This reuses the existing `gearBtn` style verbatim — same circular icon-button treatment as the
settings gear right next to it, just a different icon: two waypoint dots connected by a line. `Map.tsx`
already imports `Svg, { Circle, Line, Path } from "react-native-svg"` — no import changes needed.)

- [ ] **Step 4: Add the entry point in `Settings.tsx`**

In `mobile/src/screens/Settings.tsx`, find this exact boundary (the end of the "account" section,
right before the "offline data" section starts):

```tsx
            )}
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.offlineDataSection}</Text>
```

Replace it with (inserting a new "itineraries" section between the two, only shown when the user is
logged in — itineraries are an account feature, same gate already used for the account section above
it):

```tsx
            )}
          </View>
        </View>

        {isLoggedIn && (
          <View style={styles.section}>
            <Text style={styles.sectionLabel}>{t.itineraries.listTitle}</Text>
            <View style={styles.group}>
              <Pressable style={[styles.row, styles.rowLast]} onPress={() => navigation.navigate("ItinerariesList")}>
                <Text style={styles.rowLabel}>{t.itineraries.listTitle}</Text>
              </Pressable>
            </View>
          </View>
        )}

        <View style={styles.section}>
          <Text style={styles.sectionLabel}>{t.settings.offlineDataSection}</Text>
```

- [ ] **Step 5: Run tsc and the full test suite**

Run: `cd mobile && npx tsc --noEmit && npx jest`
Expected: both clean/green.

- [ ] **Step 6: Manual verification**

No component test exists for screens in this app (matches convention). Run
`npx expo start --port 19010 --web` from `mobile/`, log in, open the app, and confirm: the new icon
button appears in `Map.tsx`'s header next to the settings gear and navigates to the empty itineraries
list; the same list is reachable from Settings; the empty state renders correctly with no itineraries
yet.

- [ ] **Step 7: Commit**

```bash
cd mobile && git add src/screens/ItinerariesList.tsx src/navigation/AppNavigator.tsx src/screens/Map.tsx src/screens/Settings.tsx
git commit -m "mobile: itineraries list screen, entry points from Map and Settings"
```

---

### Task 3: Chat creation screen

**Files:**
- Create: `mobile/src/screens/ItineraryChat.tsx`
- Modify: `mobile/src/navigation/AppNavigator.tsx`

**Interfaces:**
- Consumes: `createItinerary` (Task 1), `useAuth()`, `useLocale()`, `formatDuration` (Task 1),
  navigation to `"ItineraryDetail"` (Task 1's route, built by Task 4).
- Produces: `ItineraryChatScreen`, registered as route `"ItineraryChat"` — consumed by Task 2's "create"
  button.

- [ ] **Step 1: Write `ItineraryChat.tsx`**

```tsx
// mobile/src/screens/ItineraryChat.tsx
import React, { useState } from "react";
import { View, Text, Pressable, TextInput, ScrollView, StyleSheet, ActivityIndicator, KeyboardAvoidingView, Platform } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { createItinerary, type Itinerary } from "../data/ItinerariesRepository";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ItineraryChat">;

type Turn = { question: string; result: Itinerary | null };

export function ItineraryChatScreen({ navigation }: Props) {
  const { t } = useLocale();
  const { token } = useAuth();
  const [turns, setTurns] = useState<Turn[]>([]);
  const [pendingQuestion, setPendingQuestion] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function handleSend() {
    const question = input.trim();
    if (!question || !token || pendingQuestion) return;
    setInput("");
    setError(null);
    setPendingQuestion(question);
    try {
      const result = await createItinerary(token, question);
      setTurns((prev) => [...prev, { question, result }]);
    } catch {
      setInput(question);
      setError(t.itineraries.chatSendError);
    } finally {
      setPendingQuestion(null);
    }
  }

  return (
    <KeyboardAvoidingView style={styles.screen} behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <SafeAreaView style={styles.flexOne} edges={["top", "bottom"]}>
        <View style={styles.topbar}>
          <Pressable style={styles.back} onPress={() => navigation.goBack()}>
            <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
              <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
          </Pressable>
        </View>

        <View style={styles.head}>
          <Text style={styles.title}>{t.itineraries.chatTitle}</Text>
          <Text style={styles.subtitle}>{t.itineraries.chatSubtitle}</Text>
        </View>

        <ScrollView style={styles.chat} contentContainerStyle={styles.chatContent}>
          {turns.map((turn, i) => (
            <View key={i}>
              <View style={styles.rowUser}>
                <View style={styles.bubbleUser}>
                  <Text style={styles.bubbleUserText}>{turn.question}</Text>
                </View>
              </View>
              <View style={styles.rowAi}>
                {turn.result ? (
                  <Pressable
                    style={styles.itineraryCard}
                    onPress={() => navigation.navigate("ItineraryDetail", { itineraryId: turn.result!.id })}
                  >
                    <Text style={styles.itineraryCardTitle}>{turn.result.title}</Text>
                    <Text style={styles.itineraryCardMeta}>
                      {formatDuration(turn.result.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(turn.result.placeCount))}
                    </Text>
                    {turn.result.stops.map((stop, si) => (
                      <Text
                        key={si}
                        style={stop.kind === "suggestion" ? styles.stopLineSuggestion : styles.stopLine}
                      >
                        {si + 1}. {stop.label}
                        {stop.kind === "suggestion" ? ` (${t.itineraries.suggestionLabel})` : ""}
                      </Text>
                    ))}
                    <Text style={styles.viewFullLink}>{t.itineraries.viewFullItinerary}</Text>
                  </Pressable>
                ) : (
                  <View style={styles.bubbleAi}>
                    <Text style={styles.bubbleAiText}>{t.itineraries.chatSendError}</Text>
                  </View>
                )}
              </View>
            </View>
          ))}
          {pendingQuestion && (
            <View>
              <View style={styles.rowUser}>
                <View style={styles.bubbleUser}>
                  <Text style={styles.bubbleUserText}>{pendingQuestion}</Text>
                </View>
              </View>
              <View style={styles.rowAi}>
                <View style={styles.bubbleAi}>
                  <ActivityIndicator color={colors.terracotta} />
                </View>
              </View>
            </View>
          )}
        </ScrollView>

        {error && <Text style={styles.errorText}>{error}</Text>}

        <View style={styles.inputBar}>
          <TextInput
            style={styles.input}
            value={input}
            onChangeText={setInput}
            placeholder={t.itineraries.chatInputPlaceholder}
            placeholderTextColor={colors.inkFaint}
            onSubmitEditing={handleSend}
            returnKeyType="send"
          />
          <Pressable style={[styles.sendBtn, !input.trim() && styles.sendBtnDisabled]} disabled={!input.trim() || !!pendingQuestion} onPress={handleSend}>
            <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
              <Path d="M22 2 11 13" stroke={colors.cream} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
              <Path d="M22 2 15 22 11 13 2 9 22 2Z" stroke={colors.cream} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
            </Svg>
          </Pressable>
        </View>
      </SafeAreaView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  flexOne: { flex: 1 },
  topbar: { flexDirection: "row", alignItems: "center", paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  head: { paddingHorizontal: 22, paddingTop: 16 },
  title: { fontFamily: fonts.display, fontSize: 22, color: colors.ink, marginBottom: 6 },
  subtitle: { fontFamily: fonts.body, fontSize: 13.5, lineHeight: 19, color: colors.inkSoft },
  chat: { flex: 1, paddingHorizontal: 20, paddingTop: 18 },
  chatContent: { gap: 14, paddingBottom: 8 },
  rowUser: { flexDirection: "row", justifyContent: "flex-end", marginBottom: 8 },
  bubbleUser: {
    maxWidth: "78%",
    backgroundColor: colors.terracotta,
    borderRadius: 16,
    borderBottomRightRadius: 4,
    paddingVertical: 12,
    paddingHorizontal: 15,
  },
  bubbleUserText: { fontFamily: fonts.body, fontSize: 14.5, lineHeight: 21, color: colors.cream },
  rowAi: { flexDirection: "row", justifyContent: "flex-start" },
  bubbleAi: {
    maxWidth: "84%",
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: 16,
    borderBottomLeftRadius: 4,
    paddingVertical: 12,
    paddingHorizontal: 15,
  },
  bubbleAiText: { fontFamily: fonts.body, fontSize: 14.5, lineHeight: 21, color: colors.ink },
  itineraryCard: {
    maxWidth: "90%",
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: 16,
    borderBottomLeftRadius: 4,
    padding: 15,
    gap: 6,
  },
  itineraryCardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 15.5, color: colors.ink },
  itineraryCardMeta: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginBottom: 4 },
  stopLine: { fontFamily: fonts.body, fontSize: 13.5, color: colors.ink },
  stopLineSuggestion: { fontFamily: fonts.body, fontSize: 13.5, color: colors.inkSoft, fontStyle: "italic" },
  viewFullLink: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.terracotta, marginTop: 6 },
  errorText: { fontFamily: fonts.body, fontSize: 12.5, color: colors.terracottaDark, textAlign: "center", paddingBottom: 6 },
  inputBar: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    margin: 20,
    marginTop: 12,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    borderRadius: radii.md,
    paddingVertical: 8,
    paddingHorizontal: 8,
    paddingLeft: 16,
  },
  input: { flex: 1, fontFamily: fonts.body, fontSize: 14.5, color: colors.ink },
  sendBtn: {
    width: 38,
    height: 38,
    borderRadius: 19,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  sendBtnDisabled: { backgroundColor: "rgba(193,89,46,0.35)" },
});
```

- [ ] **Step 2: Register the route in `AppNavigator.tsx`**

Add the import:

```typescript
import { ItineraryChatScreen } from "../screens/ItineraryChat";
```

and the screen entry:

```tsx
      <Stack.Screen name="ItineraryChat" component={ItineraryChatScreen} />
```

- [ ] **Step 3: Run tsc and the full test suite**

Run: `cd mobile && npx tsc --noEmit && npx jest`
Expected: both clean/green. (`ItineraryDetail` navigation calls type-check fine even though that screen
doesn't exist yet — `AppStackParamList` already declares its param shape from Task 1, which is all
`navigation.navigate` needs.)

- [ ] **Step 4: Manual verification**

Run `npx expo start --port 19010 --web` from `mobile/`, open "Créer un itinéraire" from the list screen,
type a real request (e.g. "1h à Santa Teresa, focus street art"), send it, and confirm an inline
itinerary card appears with a title, duration, stop count, and a distinctly-styled (italic, muted)
suggestion line if the response includes a meal break. Requires the backend to actually reach Claude
(a configured `ANTHROPIC_API_KEY` and available credit) — if that's not available in this environment,
confirm at least that the request is sent and a real error message appears in the failure path instead
of a silent no-op.

- [ ] **Step 5: Commit**

```bash
cd mobile && git add src/screens/ItineraryChat.tsx src/navigation/AppNavigator.tsx
git commit -m "mobile: itinerary chat creation screen"
```

---

### Task 4: Itinerary detail screen (vertical timeline)

**Files:**
- Create: `mobile/src/screens/ItineraryDetail.tsx`
- Modify: `mobile/src/navigation/AppNavigator.tsx`

**Interfaces:**
- Consumes: `getItinerary` (Task 1), `useAuth()`, `useLocale()`, `formatDuration` (Task 1).
- Produces: `ItineraryDetailScreen`, registered as route `"ItineraryDetail"` — this is the last task
  touching `AppStackParamList`'s consumers; no later task depends on this one.

- [ ] **Step 1: Write `ItineraryDetail.tsx`**

```tsx
// mobile/src/screens/ItineraryDetail.tsx
import React, { useEffect, useState } from "react";
import { View, Text, Pressable, ScrollView, StyleSheet, ActivityIndicator } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { getItinerary, type Itinerary } from "../data/ItinerariesRepository";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "ItineraryDetail">;

export function ItineraryDetailScreen({ route, navigation }: Props) {
  const { t } = useLocale();
  const { token } = useAuth();
  const [itinerary, setItinerary] = useState<Itinerary | null>(null);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    getItinerary(token, route.params.itineraryId)
      .then((result) => {
        if (!cancelled) setItinerary(result);
      })
      .catch(() => {
        if (!cancelled) setNotFound(true);
      });
    return () => {
      cancelled = true;
    };
  }, [token, route.params.itineraryId]);

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      {notFound ? (
        <View style={styles.empty}>
          <Text style={styles.emptyBody}>{t.itineraries.detailNotFound}</Text>
        </View>
      ) : !itinerary ? (
        <ActivityIndicator style={styles.loading} color={colors.terracotta} />
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          <Text style={styles.title}>{itinerary.title}</Text>
          <Text style={styles.meta}>
            {formatDuration(itinerary.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(itinerary.placeCount))}
          </Text>

          <View style={styles.timeline}>
            {(() => {
              let placeNumber = 0;
              return itinerary.stops.map((stop, i) => {
                const isSuggestion = stop.kind === "suggestion";
                if (!isSuggestion) placeNumber += 1;
                const isLast = i === itinerary.stops.length - 1;
                return (
                  <View key={i} style={styles.stopRow}>
                    <View style={styles.badgeColumn}>
                      <View style={[styles.badge, isSuggestion && styles.badgeSuggestion]}>
                        {!isSuggestion && <Text style={styles.badgeText}>{placeNumber}</Text>}
                      </View>
                      {!isLast && <View style={styles.connector} />}
                    </View>
                    <View style={styles.stopBody}>
                      <Text style={[styles.stopLabel, isSuggestion && styles.stopLabelSuggestion]}>
                        {stop.label}
                      </Text>
                      {isSuggestion ? (
                        <Text style={styles.suggestionCaption}>{t.itineraries.suggestionLabel}</Text>
                      ) : (
                        <Text style={styles.stopMeta}>{formatDuration(stop.timeOnSiteMinutes)}</Text>
                      )}
                      {!isLast && stop.walkToNextMinutes > 0 && (
                        <Text style={styles.walkMeta}>
                          {t.itineraries.walkToNext.replace("{minutes}", String(stop.walkToNextMinutes))}
                        </Text>
                      )}
                    </View>
                  </View>
                );
              });
            })()}
          </View>
        </ScrollView>
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  loading: { marginTop: 40 },
  empty: { paddingHorizontal: 32, marginTop: 48, alignItems: "center" },
  emptyBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
  content: { paddingHorizontal: 22, paddingTop: 16, paddingBottom: 40 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 6 },
  meta: { fontFamily: fonts.body, fontSize: 13.5, color: colors.inkSoft, marginBottom: 24 },
  timeline: {},
  stopRow: { flexDirection: "row", gap: 14 },
  badgeColumn: { alignItems: "center", width: 28 },
  badge: {
    width: 28,
    height: 28,
    borderRadius: 14,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  badgeSuggestion: {
    backgroundColor: "transparent",
    borderWidth: 1.5,
    borderColor: colors.inkFaint,
    borderStyle: "dashed",
  },
  badgeText: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.cream },
  connector: { width: 2, flex: 1, minHeight: 24, backgroundColor: colors.line, marginTop: 2 },
  stopBody: { flex: 1, paddingBottom: 22 },
  stopLabel: { fontFamily: fonts.bodySemiBold, fontSize: 15.5, color: colors.ink },
  stopLabelSuggestion: { fontStyle: "italic", color: colors.inkSoft },
  stopMeta: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginTop: 2 },
  suggestionCaption: { fontFamily: fonts.body, fontSize: 12, fontStyle: "italic", color: colors.inkFaint, marginTop: 2 },
  walkMeta: { fontFamily: fonts.body, fontSize: 12, color: colors.inkFaint, marginTop: 8 },
});
```

- [ ] **Step 2: Register the route in `AppNavigator.tsx`**

Add the import:

```typescript
import { ItineraryDetailScreen } from "../screens/ItineraryDetail";
```

and the screen entry:

```tsx
      <Stack.Screen name="ItineraryDetail" component={ItineraryDetailScreen} />
```

- [ ] **Step 3: Run tsc and the full test suite**

Run: `cd mobile && npx tsc --noEmit && npx jest`
Expected: both clean/green.

- [ ] **Step 4: Manual verification**

From the list or chat screen, open an itinerary and confirm: numbered solid terracotta badges for real
place stops, a dashed unnumbered badge with italic muted text and the "suggestion de l'IA, non vérifiée"
caption for the meal-break slot (if the generated itinerary included one), and a walking-time line
between consecutive stops.

- [ ] **Step 5: Commit**

```bash
cd mobile && git add src/screens/ItineraryDetail.tsx src/navigation/AppNavigator.tsx
git commit -m "mobile: itinerary detail screen, vertical timeline with the suggestion-slot distinction"
```
