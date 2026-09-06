# Place assistant (real per-place Q&A)

**Status:** design approved, spec pending implementation plan.

## Why

The `Assistant` screen (pushed from `PlaceDetail`'s "Poser une question" row) has always been a
non-functional "roadmap" teaser — disabled input, a fixed example exchange, a "pas encore construit"
badge (`docs/superpowers/specs/2026-08-18-mobile-app-design.md`). This reverses that: the founder
explicitly chose to build it for real, overriding the standing scope decision in `mission.md` ("Not a
live RAG chat / research copilot — explicitly considered and rejected as feature creep... Reconsiderable
in Phase 2 if a real user need emerges"). `mission.md` gets updated to reflect this once this spec is
approved — same pattern as the 2026-09-06 domain/ports authorship change earlier this project.

This is a **per-place** assistant (distinct from the itineraries feature's own free-text chat, which
composes routes across places — the two were deliberately kept separate when the itineraries feature was
scoped).

## Scope

**In scope (v1):**
- Real question-answering on a single place's already-grounded content (the same narration text and
  sources used for audio narration — no separate corpus, no new grounding work).
- A general-knowledge fallback for practical questions the grounded text doesn't cover (opening hours,
  access, etc.), clearly and honestly distinguished from verified content, never given the same trust
  signal as grounded narration — mirrors the itineraries feature's one sanctioned unverified slot (the
  meal-break suggestion), applied here per-answer instead of per-stop.
- Multi-turn conversation within a single screen visit (follow-up questions can build on the prior
  exchange).
- Ephemeral only — nothing is persisted server-side. A conversation is lost when the screen is left,
  same as it would be for any transient client-side state.

**Explicitly out of scope for v1 (deferred, not decided now):**
- Persisting conversations (server-side, per user/per place) for later retrieval.
- Any place without a **published** script in the user's current language — the assistant is simply
  unavailable there, matching this project's standing narration rule ("a place without real grounding
  gets no narration — never invent facts to fill a gap"), not offered in a fully-unverified mode.
- Cross-place reasoning ("which of these three museums is closest") — that already belongs to the
  itineraries feature, not this one.
- Voice input/output for the assistant itself (text only, same as the current teaser).

## Backend

Same hexagonal layering as the AI Itineraries feature (`ports` → `application` → `adapters/claude` +
`adapters/http`), but **no new `domain` aggregate** — unlike an `Itinerary`, nothing here is persisted, so
there is no entity to model or reconstruct. `internal/domain/script.go`'s existing `Script` type (already
used for narration) is read, not extended.

**New port** (`internal/ports/place_assistant.go`):

```go
type ConversationTurn struct {
	Question string
	Answer   string
}

type AssistantAnswer struct {
	Answer         string
	GroundingLevel string // "grounded" | "mixed" | "general"
}

// PlaceAssistant answers a free-text question about one place, using that
// place's own already-grounded narration text as its primary source.
type PlaceAssistant interface {
	Ask(ctx context.Context, groundedText string, history []ConversationTurn, question string) (AssistantAnswer, error)
}
```

**New adapter** (`internal/adapters/claude/place_assistant.go`) — same shape as
`internal/adapters/claude/itinerary_generator.go`: a system prompt instructing the model to answer
primarily from the given grounded text, and to supplement with its own general knowledge only when the
grounded text doesn't cover the question, then to self-report which of the three `GroundingLevel` values
the **whole answer** falls under. Forced tool-use (`ToolChoiceUnionParam`), same pattern as
`propose_itinerary` — the model cannot answer in free text without also declaring its grounding level, so
there is no separate classification step that could drift from what the model actually did.

**New application function** (`internal/application/ask_assistant.go`):

```go
func AskAssistant(ctx context.Context, assistant ports.PlaceAssistant, scriptRepo ports.ScriptRepository, placeID, language string, history []ports.ConversationTurn, question string) (ports.AssistantAnswer, error)
```

- Loads the place's script via `scriptRepo.FindByPlaceIDAndLanguage(ctx, placeID, language)`; if none
  exists, or it exists but `Status() != domain.ScriptStatusPublished`, returns a dedicated
  `ErrNoPublishedScript` (mirrors the narration/audio-availability gate already used by
  `getPlaceDetail`/`getPlaceAudio`).
- Truncates `history` to the last **6** turns *before* building the prompt (not after) — an explicit,
  from-day-one safeguard against unbounded token growth in a long conversation, the exact class of issue
  the itineraries feature's final review had to add after the fact (uncapped candidate list). Six turns
  is a deliberate, generous-but-bounded choice: enough for a real back-and-forth, small enough that cost
  per request can never grow unbounded regardless of how long a user keeps chatting.
- Calls `assistant.Ask(...)`, wraps any error the same way `application.GenerateItinerary` wraps generator
  failures (an `ErrAssistantFailed` sentinel, `%w`-wrapped) so the HTTP layer can classify it as upstream
  (502) rather than a client-side rejection (422) — the itineraries fix wave's error-classification
  pattern, applied here from the start rather than added after a review finds it missing.
- No `Save` call, no repository write of any kind — this function's only side effect is the outbound
  Claude call.

**New route** (`internal/adapters/http/place_assistant_handler.go`, registered in `server.go` next to the
other `/places/:id/...` routes, auth required):

```
POST /places/:id/assistant
Body:     {"language": "fr", "question": "...", "history": [{"question": "...", "answer": "..."}]}
Response: {"answer": "...", "grounding_level": "grounded"}
```

- 400 if `question` is empty (mirrors `createItinerary`'s empty-`request` check).
- 404/422 if `errors.Is(err, application.ErrNoPublishedScript)` — a clear "no narration for this
  place/language yet" message, not a raw error.
- 502 + generic message + server-side `log.Printf` if `errors.Is(err, application.ErrAssistantFailed)`
  (upstream Claude failure) — same classification the itineraries fix wave introduced for
  `ErrGenerationFailed`.
- 500 + generic message for anything else unexpected.

## Mobile (`mobile/src/screens/Assistant.tsx`)

- The fixed example exchange and the "Roadmap · pas encore construit" badge are removed. The screen
  starts empty, with a short static invitation line in place of the old example bubbles until the first
  real question is sent.
- The input bar (currently `pointerEvents="none"`, permanently dimmed) becomes a real controlled
  `TextInput` plus a send button/icon.
- Conversation state is a plain in-memory array of `{question, answer, groundingLevel}` held in the
  screen's own React state — nothing external, nothing persisted, gone the moment the screen unmounts.
  Sending a question appends the user's bubble immediately, shows a loading indicator in place of the AI
  bubble while the request is in flight, then replaces it with the real answer once the response arrives.
- The existing `sourceChip` becomes a `groundingLevel`-driven badge with three visual states:
  - `grounded` — keeps today's exact "✓ [source]" chip styling (already implemented, already correct).
  - `mixed` — same chip shape, amber/warning tone, label along the lines of "en partie non vérifié".
  - `general` — same chip shape, a more clearly "caution" tone, label along the lines of "information
    générale, non vérifiée" — deliberately never using the checkmark icon the grounded state uses, so a
    user cannot mistake one for the other at a glance.
- `PlaceDetail`'s "Poser une question" row stays exactly where it is, but only navigates to `Assistant`
  when the place has a published script in the current locale; otherwise it's disabled (or the
  `Assistant` screen itself renders a "pas encore disponible pour ce lieu" state instead of the chat —
  final choice left to the implementation plan, whichever reuses more of the existing
  `narrationUnavailable`-style plumbing already in `PlaceDetail.tsx`).

## Testing

- Backend: `internal/application/ask_assistant_test.go` (fake `PlaceAssistant` + fake script repo,
  mirroring `generate_itinerary_test.go`'s fake-based style, including a test that a request is rejected
  before ever calling the assistant port when no published script exists, and a test that history longer
  than 6 turns is truncated before being handed to the port); `internal/adapters/claude/place_assistant_test.go`
  (fake Anthropic client, tool-use parsing, mirroring `itinerary_generator_test.go`); HTTP handler tests
  mirroring `itinerary_handler_test.go` — critically, **from the first commit**, a test asserting
  `grounding_level` round-trips into the JSON response body (the itineraries feature only got this test
  after a final review caught its absence; here it's part of the initial task, not a fix-wave addition).
- Mobile: no component test for the screen itself (matches every other screen in this app); any pure
  logic extracted (e.g. client-side conversation-array helpers, if any end up existing outside the
  component) gets Jest coverage per the project's existing convention.

## After this plan (explicitly deferred)

- Persisting conversations server-side.
- Making unpublished/ungrounded places usable in a fully-general-knowledge mode.
- Any cross-place reasoning (stays the itineraries feature's territory).
- Voice input/output for the assistant.
