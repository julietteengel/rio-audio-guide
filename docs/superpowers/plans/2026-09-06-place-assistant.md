# Place Assistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the `Assistant` screen from a non-functional "roadmap" teaser into a real per-place Q&A
assistant, grounded in that place's own published narration text, with an honestly-marked
general-knowledge fallback for questions the narration doesn't cover.

**Architecture:** Same hexagonal layering as the AI Itineraries feature (`ports` → `application` →
`adapters/claude` + `adapters/http`), but no new `domain` aggregate — nothing here is persisted.
Mobile: the existing `Assistant.tsx` screen becomes a real chat wired to a new backend route, with
conversation state held in memory only (React state), never saved server-side.

**Tech Stack:** Go backend (Echo, `pgx`, Anthropic Go SDK — already pinned in `go.mod`,
`github.com/anthropics/anthropic-sdk-go` v1.71.0), React Native/Expo mobile (existing i18n/theme system).

**Spec:** `docs/superpowers/specs/2026-09-06-place-assistant-design.md`

## Global Constraints

- Conversation history is capped at **6** prior turns before being sent to the LLM, truncated in
  `internal/application`, never in the adapter — an explicit, from-day-one safeguard against unbounded
  token growth (the itineraries feature only added its equivalent cap, `maxCandidatePlaces`, after a
  final review caught its absence — this plan does not repeat that gap).
  Model: `claude-opus-5`, `MaxTokens: 1024` (a short conversational answer, not a generated document).
- Errors from the assistant's own port/adapter are wrapped and classified by origin at the HTTP layer —
  never collapsed to a single generic status. The itineraries fix wave established this pattern
  (`ErrGenerationFailed` → 502, `ErrSaveFailed` → 500, everything else → its own status); this plan uses
  the same shape from the start: `ErrAssistantFailed` → 502, `ErrNoPublishedScript` → 404, bad request →
  400.
- The `grounding_level` field (`"grounded" | "mixed" | "general"`) must round-trip into the HTTP JSON
  response, verified by a dedicated test in the task that adds the route — not added after the fact.
- No new `internal/domain` type. `internal/ports/place_assistant.go`'s `AssistantAnswer`/
  `ConversationTurn` are plain `ports`-level structs (mirrors `ports.GeneratedItinerary`/`GeneratedStop`
  for the itineraries feature, which also aren't `domain` types).
- Mobile: nothing is persisted client-side either. Conversation state lives in the `Assistant` screen's
  own React state and is gone when the screen unmounts.
- i18n: every new dictionary key is added to **all four** locale blocks (`fr`, `en`, `pt`, `es`) in
  `mobile/src/i18n/dictionary.ts` — there is no compile-time check that the locales stay in sync
  (`Dictionary` is `typeof dictionary.en` only), so a key added to `en` alone compiles fine and silently
  renders `undefined` for the other three languages at runtime. Each task below gives the exact text for
  all four; do not paraphrase or add a key to only one locale.

---

### Task 1: Backend port + Claude adapter

**Files:**
- Create: `backend/internal/ports/place_assistant.go`
- Create: `backend/internal/adapters/claude/place_assistant.go`
- Test: `backend/internal/adapters/claude/place_assistant_test.go`

**Interfaces:**
- Consumes: `messagesAPI` (already declared in `backend/internal/adapters/claude/itinerary_generator.go`,
  same package — do **not** redeclare it, this task's new file is in the same `claude` package and can
  reference it directly).
- Produces: `ports.ConversationTurn{Question, Answer string}`, `ports.AssistantAnswer{Answer,
  GroundingLevel string}`, `ports.PlaceAssistant` interface with method
  `Ask(ctx context.Context, groundedText string, history []ConversationTurn, question string)
  (AssistantAnswer, error)`, and `claude.NewPlaceAssistant(client messagesAPI) *PlaceAssistant` — Task 3
  wires this into `cmd/api/main.go` and `internal/adapters/http/server.go`; Task 2 consumes the
  `ports.PlaceAssistant` interface.

- [ ] **Step 1: Write `ports/place_assistant.go`**

```go
// internal/ports/place_assistant.go
package ports

import "context"

// ConversationTurn is one already-completed question/answer exchange, sent
// back on each new request so the assistant can follow up on it -- nothing
// server-side remembers a conversation between requests (see
// internal/application/ask_assistant.go), so the caller carries this state.
type ConversationTurn struct {
	Question string
	Answer   string
}

// AssistantAnswer is the assistant's answer to one question, plus its own
// declared GroundingLevel -- one of "grounded" (entirely from the place's
// own narration text), "mixed" (partly from it, partly the assistant's own
// general knowledge), or "general" (the narration text didn't cover the
// question at all). The mobile client renders each level with a distinct,
// honest trust signal; it never shows the same badge for a "general" or
// "mixed" answer that it shows for "grounded".
type AssistantAnswer struct {
	Answer         string
	GroundingLevel string
}

// PlaceAssistant answers a free-text question about one place, given that
// place's own already-published narration text as its primary source.
// Implemented by internal/adapters/claude.
type PlaceAssistant interface {
	Ask(ctx context.Context, groundedText string, history []ConversationTurn, question string) (AssistantAnswer, error)
}
```

- [ ] **Step 2: Write the failing adapter test**

```go
// internal/adapters/claude/place_assistant_test.go
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"rioaudioguide/backend/internal/ports"
)

// answerToolUseResponse mirrors toolUseResponse (itinerary_generator_test.go,
// same package) but for the answer_question tool -- not reused directly
// because that helper hardcodes the "propose_itinerary" tool name.
func answerToolUseResponse(t *testing.T, input any) *anthropic.Message {
	t.Helper()
	rawInput, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal fixture input: %v", err)
	}
	rawMessage, err := json.Marshal(map[string]any{
		"content": []map[string]any{
			{"type": "tool_use", "id": "toolu_1", "name": "answer_question", "input": json.RawMessage(rawInput)},
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture message: %v", err)
	}
	var msg anthropic.Message
	if err := json.Unmarshal(rawMessage, &msg); err != nil {
		t.Fatalf("unmarshal fixture message: %v", err)
	}
	return &msg
}

func TestPlaceAssistant_Ask_ParsesToolUseResponse(t *testing.T) {
	input := map[string]any{
		"answer":          "The site was chosen for its 360° view of the city.",
		"grounding_level": "grounded",
	}
	fake := &fakeMessagesAPI{response: answerToolUseResponse(t, input)}
	assistant := NewPlaceAssistant(fake)

	got, err := assistant.Ask(context.Background(), "Cristo Redentor narration text...", nil, "Why was it built here?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Answer != "The site was chosen for its 360° view of the city." {
		t.Fatalf("got answer %q", got.Answer)
	}
	if got.GroundingLevel != "grounded" {
		t.Fatalf("got grounding level %q, want grounded", got.GroundingLevel)
	}
}

func TestPlaceAssistant_Ask_SendsHistoryAsPriorMessages(t *testing.T) {
	var captured []anthropic.MessageParam
	fake := &fakeMessagesAPI{response: answerToolUseResponse(t, map[string]any{"answer": "42", "grounding_level": "general"})}
	// Wrap fake to capture the params New() was called with.
	capturing := &capturingMessagesAPI{inner: fake, onCall: func(p anthropic.MessageNewParams) { captured = p.Messages }}
	assistant := NewPlaceAssistant(capturing)

	history := []ports.ConversationTurn{{Question: "What is this place?", Answer: "A statue."}}
	_, err := assistant.Ask(context.Background(), "narration text", history, "How tall is it?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 prior user turn + 1 prior assistant turn + the new question = 3 messages.
	if len(captured) != 3 {
		t.Fatalf("got %d messages sent to Claude, want 3 (1 history pair + 1 new question)", len(captured))
	}
}

func TestPlaceAssistant_Ask_WrapsClientError(t *testing.T) {
	fake := &fakeMessagesAPI{err: errors.New("connection reset")}
	assistant := NewPlaceAssistant(fake)

	_, err := assistant.Ask(context.Background(), "narration text", nil, "question")
	if err == nil {
		t.Fatal("expected an error to propagate")
	}
}

// capturingMessagesAPI wraps another messagesAPI and records the params of
// the last call, so a test can assert on what was actually sent to Claude
// without needing a real network round-trip.
type capturingMessagesAPI struct {
	inner  messagesAPI
	onCall func(anthropic.MessageNewParams)
}

func (c *capturingMessagesAPI) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	c.onCall(params)
	return c.inner.New(ctx, params, opts...)
}
```

This test file needs one more import alongside the others already at its top:
`"github.com/anthropics/anthropic-sdk-go/option"` (the same import `itinerary_generator.go` already
uses for the real `messagesAPI` interface's method signature).

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/claude/... -run PlaceAssistant -v`
Expected: FAIL (build error — `NewPlaceAssistant` doesn't exist yet).

- [ ] **Step 4: Write the adapter**

```go
// internal/adapters/claude/place_assistant.go
package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"rioaudioguide/backend/internal/ports"
)

// PlaceAssistant answers a free-text question about one place, given that
// place's own narration text -- implements ports.PlaceAssistant.
type PlaceAssistant struct {
	client messagesAPI
}

func NewPlaceAssistant(client messagesAPI) *PlaceAssistant {
	return &PlaceAssistant{client: client}
}

const assistantSystemPromptPrefix = `You are a knowledgeable, friendly local guide answering a tourist's question about ONE specific place in Rio de Janeiro.

You are given that place's own verified narration text. Rules:
- Answer primarily from the given narration text.
- If the narration text doesn't cover the question, you may supplement with your own general knowledge of the place/area -- but you must always call answer_question and honestly set grounding_level to reflect what you actually did:
  - "grounded": your whole answer came from the narration text.
  - "mixed": part of your answer came from the narration text, part from your own general knowledge.
  - "general": the narration text didn't help at all -- your whole answer is your own general knowledge.
- Never claim grounding_level "grounded" for anything you didn't actually find in the narration text.
- Always call answer_question -- never answer in plain text.

Narration text for this place:
`

var answerQuestionTool = anthropic.ToolParam{
	Name:        "answer_question",
	Description: anthropic.String("Answer the tourist's question about this place, honestly declaring how much of the answer came from the place's own verified narration text."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"answer":          map[string]any{"type": "string"},
			"grounding_level": map[string]any{"type": "string", "enum": []string{"grounded", "mixed", "general"}},
		},
		Required: []string{"answer", "grounding_level"},
	},
}

type answerToolInput struct {
	Answer         string `json:"answer"`
	GroundingLevel string `json:"grounding_level"`
}

func (a *PlaceAssistant) Ask(ctx context.Context, groundedText string, history []ports.ConversationTurn, question string) (ports.AssistantAnswer, error) {
	messages := make([]anthropic.MessageParam, 0, len(history)*2+1)
	for _, turn := range history {
		messages = append(messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock(turn.Question)),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock(turn.Answer)),
		)
	}
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(question)))

	resp, err := a.client.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-opus-5",
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: assistantSystemPromptPrefix + groundedText}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &answerQuestionTool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "answer_question"},
		},
		Messages: messages,
	})
	if err != nil {
		return ports.AssistantAnswer{}, fmt.Errorf("claude: ask assistant: %w", err)
	}

	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok && variant.Name == "answer_question" {
			var in answerToolInput
			if err := json.Unmarshal(variant.Input, &in); err != nil {
				return ports.AssistantAnswer{}, fmt.Errorf("claude: parse answer_question input: %w", err)
			}
			return ports.AssistantAnswer{Answer: in.Answer, GroundingLevel: in.GroundingLevel}, nil
		}
	}
	return ports.AssistantAnswer{}, fmt.Errorf("claude: response contained no answer_question tool call")
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/adapters/claude/... -v`
Expected: PASS (all tests in the package, including the pre-existing itinerary generator tests).

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/ports/place_assistant.go internal/adapters/claude/place_assistant.go internal/adapters/claude/place_assistant_test.go
git commit -m "claude: PlaceAssistant adapter, answers grounded in a place's narration text"
```

---

### Task 2: Application orchestration

**Files:**
- Create: `backend/internal/application/ask_assistant.go`
- Test: `backend/internal/application/ask_assistant_test.go`

**Interfaces:**
- Consumes: `ports.PlaceAssistant.Ask(...)` (Task 1), `ports.ScriptRepository.FindByPlaceIDAndLanguage(ctx,
  placeID, language string) (*domain.Script, error)` (already exists), `domain.Script.Status()` /
  `domain.ScriptStatusPublished` / `domain.Script.Text().String()` (already exist).
- Produces: `application.AskAssistant(ctx, assistant ports.PlaceAssistant, scriptRepo
  ports.ScriptRepository, placeID, language string, history []ports.ConversationTurn, question string)
  (ports.AssistantAnswer, error)`, `application.ErrNoPublishedScript`, `application.ErrAssistantFailed` —
  Task 3 consumes all three.

- [ ] **Step 1: Write the failing test**

```go
// internal/application/ask_assistant_test.go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakePlaceAssistant struct {
	result       ports.AssistantAnswer
	err          error
	lastHistory  []ports.ConversationTurn
	lastQuestion string
}

func (f *fakePlaceAssistant) Ask(_ context.Context, _ string, history []ports.ConversationTurn, question string) (ports.AssistantAnswer, error) {
	f.lastHistory = history
	f.lastQuestion = question
	return f.result, f.err
}

func publishedScriptFixture(t *testing.T, placeID, language, text string) *domain.Script {
	t.Helper()
	lang, err := domain.NewLanguage(language)
	if err != nil {
		t.Fatalf("build fixture language: %v", err)
	}
	scriptText, err := domain.NewScriptText(text)
	if err != nil {
		t.Fatalf("build fixture script text: %v", err)
	}
	script := domain.NewScript(placeID, lang, scriptText, "source")
	if err := script.MarkReviewed("reviewer-1"); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	if err := script.Publish(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return script
}

type fakeScriptRepoApp struct {
	scripts map[string]*domain.Script // keyed by placeID+"|"+language
}

func (f *fakeScriptRepoApp) Save(context.Context, *domain.Script) error { return nil }
func (f *fakeScriptRepoApp) FindByID(context.Context, string) (*domain.Script, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeScriptRepoApp) FindByPlaceIDAndLanguage(_ context.Context, placeID, language string) (*domain.Script, error) {
	s, ok := f.scripts[placeID+"|"+language]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}
func (f *fakeScriptRepoApp) FindByPlaceID(context.Context, string) ([]*domain.Script, error) {
	return nil, errors.New("not implemented in fake")
}

func TestAskAssistant_AsksWithTheScriptsGroundedText(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "Ce lieu a été construit en 1931.")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{result: ports.AssistantAnswer{Answer: "En 1931.", GroundingLevel: "grounded"}}

	got, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "Quand a-t-il été construit ?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Answer != "En 1931." || got.GroundingLevel != "grounded" {
		t.Fatalf("got %+v", got)
	}
	if assistant.lastQuestion != "Quand a-t-il été construit ?" {
		t.Fatalf("got question %q passed to assistant", assistant.lastQuestion)
	}
}

func TestAskAssistant_RejectsAPlaceWithNoPublishedScript(t *testing.T) {
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{}}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrNoPublishedScript) {
		t.Fatalf("got %v, want ErrNoPublishedScript", err)
	}
}

func TestAskAssistant_RejectsAnUnpublishedScript(t *testing.T) {
	lang, _ := domain.NewLanguage("fr")
	text, _ := domain.NewScriptText("brouillon")
	draft := domain.NewScript("place-1", lang, text, "source") // never reviewed/published -- stays draft
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": draft}}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrNoPublishedScript) {
		t.Fatalf("got %v, want ErrNoPublishedScript", err)
	}
}

func TestAskAssistant_TruncatesHistoryToLastSixTurns(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "texte")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{result: ports.AssistantAnswer{Answer: "ok", GroundingLevel: "grounded"}}

	history := make([]ports.ConversationTurn, 9)
	for i := range history {
		history[i] = ports.ConversationTurn{Question: "q", Answer: "a"}
	}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", history, "question")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assistant.lastHistory) != 6 {
		t.Fatalf("got %d history turns passed to the assistant, want 6 (truncated from 9)", len(assistant.lastHistory))
	}
}

func TestAskAssistant_WrapsAssistantError(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "texte")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{err: errors.New("rate limited")}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrAssistantFailed) {
		t.Fatalf("got %v, want an error wrapping ErrAssistantFailed", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/application/... -run AskAssistant -v`
Expected: FAIL (build error — `AskAssistant` doesn't exist yet).

- [ ] **Step 3: Write the application function**

```go
// internal/application/ask_assistant.go
package application

import (
	"context"
	"errors"
	"fmt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrNoPublishedScript is returned when the place has no published script in
// the requested language -- the same "no source, no content" gate the
// narration/audio routes already enforce (getPlaceDetail/getPlaceAudio in
// internal/adapters/http), applied here so the assistant can never answer
// about a place it has no verified text for.
var ErrNoPublishedScript = errors.New("application: no published script for this place/language")

// ErrAssistantFailed wraps any error from the assistant port (the Claude API
// being down, rate-limited, or returning a malformed response) -- an
// upstream failure, not a rejection of the caller's request. HTTP handlers
// map errors.Is(err, ErrAssistantFailed) to a 5xx, not a 422/404 -- the same
// error-classification pattern internal/application/generate_itinerary.go
// already uses for ErrGenerationFailed.
var ErrAssistantFailed = errors.New("application: assistant failed to answer")

// maxHistoryTurns caps how many prior exchanges are sent to the assistant on
// each request -- an interim safeguard against unbounded token growth in a
// long-running conversation, applied from the start (see
// internal/application/generate_itinerary.go's maxCandidatePlaces for why
// this class of bug is worth avoiding up front rather than fixing later).
const maxHistoryTurns = 6

func AskAssistant(ctx context.Context, assistant ports.PlaceAssistant, scriptRepo ports.ScriptRepository, placeID, language string, history []ports.ConversationTurn, question string) (ports.AssistantAnswer, error) {
	script, err := scriptRepo.FindByPlaceIDAndLanguage(ctx, placeID, language)
	if err != nil {
		return ports.AssistantAnswer{}, ErrNoPublishedScript
	}
	if script.Status() != domain.ScriptStatusPublished {
		return ports.AssistantAnswer{}, ErrNoPublishedScript
	}

	if len(history) > maxHistoryTurns {
		history = history[len(history)-maxHistoryTurns:]
	}

	answer, err := assistant.Ask(ctx, script.Text().String(), history, question)
	if err != nil {
		return ports.AssistantAnswer{}, fmt.Errorf("%w: %v", ErrAssistantFailed, err)
	}
	return answer, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/application/... -v`
Expected: PASS (all tests in the package, including the pre-existing itinerary application tests).

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/application/ask_assistant.go internal/application/ask_assistant_test.go
git commit -m "application: AskAssistant orchestration, gates on published script, caps history"
```

---

### Task 3: HTTP route + wiring

**Files:**
- Create: `backend/internal/adapters/http/place_assistant_handler.go`
- Create: `backend/internal/adapters/http/place_assistant_handler_test.go`
- Modify: `backend/internal/adapters/http/server.go`
- Modify: `backend/cmd/api/main.go`
- Modify (append the 12th constructor arg to every `NewServer(...)` call): `backend/internal/adapters/http/server_test.go`,
  `backend/internal/adapters/http/audio_handler_test.go`, `backend/internal/adapters/http/manifest_handler_test.go`,
  `backend/internal/adapters/http/places_handler_test.go`, `backend/internal/adapters/http/itinerary_handler_test.go`

**Interfaces:**
- Consumes: `application.AskAssistant`, `application.ErrNoPublishedScript`, `application.ErrAssistantFailed`
  (Task 2), `ports.PlaceAssistant`, `ports.ConversationTurn` (Task 1).
- Produces: `POST /places/:id/assistant` — `{"language", "question", "history":
  [{"question","answer"}]}` → `{"answer", "grounding_level"}`. `Server.assistant` field,
  `NewServer(...)`'s 12th parameter (`assistant ports.PlaceAssistant`) — nothing later consumes this;
  it's the last task touching `server.go`/`main.go` in this plan.

- [ ] **Step 1: Write the failing handler test**

```go
// internal/adapters/http/place_assistant_handler_test.go
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// fakePlaceAssistantHTTP is the zero-value-safe default every OTHER test
// file's NewServer(...) call passes for the new 12th argument -- it never
// needs to be configured because those tests don't exercise the assistant
// route at all.
type fakePlaceAssistantHTTP struct{ result ports.AssistantAnswer }

func (f *fakePlaceAssistantHTTP) Ask(_ context.Context, _ string, _ []ports.ConversationTurn, _ string) (ports.AssistantAnswer, error) {
	return f.result, nil
}

func publishedScriptFixtureHTTP(t *testing.T, placeID, language, text string) *domain.Script {
	t.Helper()
	lang, err := domain.NewLanguage(language)
	if err != nil {
		t.Fatalf("build fixture language: %v", err)
	}
	scriptText, err := domain.NewScriptText(text)
	if err != nil {
		t.Fatalf("build fixture script text: %v", err)
	}
	script := domain.NewScript(placeID, lang, scriptText, "source")
	if err := script.MarkReviewed("reviewer-1"); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	if err := script.Publish(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return script
}

func TestAskAssistant_RequiresAuth(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401 without an Authorization header", rec.Code)
	}
}

func TestAskAssistant_Success(t *testing.T) {
	script := publishedScriptFixtureHTTP(t, "place-1", "fr", "Ce lieu a été construit en 1931.")
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	assistant := &fakePlaceAssistantHTTP{result: ports.AssistantAnswer{Answer: "En 1931.", GroundingLevel: "grounded"}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, assistant)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand a-t-il été construit ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp askAssistantResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Answer != "En 1931." {
		t.Fatalf("got answer %q", resp.Answer)
	}
	if resp.GroundingLevel != "grounded" {
		t.Fatalf("got grounding_level %q, want it to survive to JSON as \"grounded\"", resp.GroundingLevel)
	}
}

func TestAskAssistant_NoPublishedScript_Returns404(t *testing.T) {
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 for a place with no published script", rec.Code)
	}
}

func TestAskAssistant_EmptyQuestion_Returns400(t *testing.T) {
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": ""})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400 for an empty question", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/http/... -run AskAssistant -v`
Expected: FAIL to compile (`askAssistant`/`askAssistantResponse` don't exist yet, `NewServer` doesn't
accept a 12th argument yet).

- [ ] **Step 3: Write the handler**

```go
// internal/adapters/http/place_assistant_handler.go
package http

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type conversationTurnRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type askAssistantRequest struct {
	Language string                    `json:"language"`
	Question string                    `json:"question"`
	History  []conversationTurnRequest `json:"history"`
}

type askAssistantResponse struct {
	Answer         string `json:"answer"`
	GroundingLevel string `json:"grounding_level"`
}

func (s *Server) askAssistant(c echo.Context) error {
	var req askAssistantRequest
	if err := c.Bind(&req); err != nil || req.Question == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"question\" field is required"})
	}
	if req.Language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"language\" field is required"})
	}

	history := make([]ports.ConversationTurn, len(req.History))
	for i, turn := range req.History {
		history[i] = ports.ConversationTurn{Question: turn.Question, Answer: turn.Answer}
	}

	answer, err := application.AskAssistant(c.Request().Context(), s.assistant, s.scriptRepo, c.Param("id"), req.Language, history, req.Question)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrNoPublishedScript):
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no narration for this place/language yet"})
		case errors.Is(err, application.ErrAssistantFailed):
			log.Printf("assistant: failed to answer: %v", err)
			return c.JSON(http.StatusBadGateway, echo.Map{"error": "the assistant is temporarily unavailable, please try again"})
		default:
			log.Printf("assistant: unexpected error: %v", err)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "unexpected error"})
		}
	}
	return c.JSON(http.StatusOK, askAssistantResponse{Answer: answer.Answer, GroundingLevel: answer.GroundingLevel})
}
```

- [ ] **Step 4: Wire `server.go`**

In `internal/adapters/http/server.go`, add a field to `Server`:

```go
	generator     ports.ItineraryGenerator
	assistant     ports.PlaceAssistant
```

Change `NewServer`'s signature to append a 12th parameter and assign it:

```go
func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, userRepo ports.UserRepository, itineraryRepo ports.ItineraryRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache, tokens ports.TokenIssuer, generator ports.ItineraryGenerator, assistant ports.PlaceAssistant) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		userRepo:      userRepo,
		itineraryRepo: itineraryRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
		tokens:        tokens,
		generator:     generator,
		assistant:     assistant,
	}
```

Add the route, next to the itineraries routes:

```go
	s.echo.POST("/places/:id/assistant", s.askAssistant, auth)
```

- [ ] **Step 5: Fix every other `NewServer(...)` call site**

Every existing `NewServer(...)` call across the whole `internal/adapters/http` package currently ends
with a 10-argument call whose last argument is `&fakeGeneratorHTTP{}` (either that literal or a local
variable holding one) — because the itineraries feature already appended its own 10th argument the same
way. Add `, &fakePlaceAssistantHTTP{}` immediately before the final closing `)` of **every** `NewServer(`
call in:
- `internal/adapters/http/server_test.go` (9 call sites)
- `internal/adapters/http/audio_handler_test.go` (7 call sites)
- `internal/adapters/http/manifest_handler_test.go` (6 call sites)
- `internal/adapters/http/places_handler_test.go` (6 call sites)
- `internal/adapters/http/itinerary_handler_test.go` (4 call sites — this file's own tests build their
  own `generator`/`&fakeGeneratorHTTP{}` value as the 11th argument; append the 12th
  `&fakePlaceAssistantHTTP{}` after whatever that 11th argument already is)

This is mechanical, but do not do it by blind find-and-replace across the whole repository — edit each
call site in each file individually. After editing, run `go build ./... && go vet ./...`: any missed
call site shows up as a compiler error naming its exact file:line, so use that to confirm nothing was
missed rather than re-grepping by hand.

- [ ] **Step 6: Wire `cmd/api/main.go`**

Find where `itineraryGenerator` is constructed and `NewServer(...)` is called (near the end of `main.go`).
Add, right after the `itineraryGenerator` line:

```go
	placeAssistant := claude.NewPlaceAssistant(&anthropicClient.Messages)
```

Then append `placeAssistant` as the 12th argument to the existing `NewServer(...)` call.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd backend && go build ./... && go vet ./... && go test ./... -v`
Expected: PASS across the whole module (every package, including every pre-existing test file touched
in Step 5).

- [ ] **Step 8: Commit**

```bash
cd backend && git add internal/adapters/http/place_assistant_handler.go internal/adapters/http/place_assistant_handler_test.go internal/adapters/http/server.go internal/adapters/http/server_test.go internal/adapters/http/audio_handler_test.go internal/adapters/http/manifest_handler_test.go internal/adapters/http/places_handler_test.go internal/adapters/http/itinerary_handler_test.go cmd/api/main.go
git commit -m "http: POST /places/:id/assistant, wired to the Claude place assistant"
```

---

### Task 4: Mobile — real chat screen

**Files:**
- Create: `mobile/src/data/AssistantRepository.ts`
- Test: `mobile/src/data/__tests__/AssistantRepository.test.ts`
- Modify: `mobile/src/i18n/dictionary.ts`
- Modify: `mobile/src/screens/Assistant.tsx`
- Modify: `mobile/src/screens/PlaceDetail.tsx`

**Interfaces:**
- Consumes: `POST /places/:id/assistant` (Task 3), `useAuth()` (`mobile/src/auth/AuthContext.tsx`,
  already exists — returns `{ token, ... }`), `useLocale()` (already exists — returns `{ t, locale }`),
  `placesRepository.getById(placeId)` (already exists, returns a `Place` with `narrationStatus`).
- Produces: `askAssistant(token, placeId, language, question, history)` and the `ConversationTurn`/
  `AssistantAnswer` types from `AssistantRepository.ts` — used only by `Assistant.tsx` in this plan.

- [ ] **Step 1: Write `AssistantRepository.ts`**

```typescript
// mobile/src/data/AssistantRepository.ts
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";

export type ConversationTurn = { question: string; answer: string };

export type GroundingLevel = "grounded" | "mixed" | "general";

export type AssistantAnswer = {
  answer: string;
  groundingLevel: GroundingLevel;
};

export class AssistantApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

export function toGroundingLevel(raw: string): GroundingLevel {
  return raw === "grounded" || raw === "mixed" || raw === "general" ? raw : "general";
}

export async function askAssistant(
  token: string,
  placeId: string,
  language: Locale,
  question: string,
  history: ConversationTurn[],
): Promise<AssistantAnswer> {
  const res = await fetch(`${API_BASE_URL}/places/${encodeURIComponent(placeId)}/assistant`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ language, question, history }),
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
    throw new AssistantApiError(message, res.status);
  }

  const parsed = body as { answer: string; grounding_level: string };
  return { answer: parsed.answer, groundingLevel: toGroundingLevel(parsed.grounding_level) };
}
```

- [ ] **Step 2: Write a Jest test for the one pure helper, `toGroundingLevel`**

```typescript
// mobile/src/data/__tests__/AssistantRepository.test.ts
import { toGroundingLevel } from "../AssistantRepository";

test("passes through each known grounding level unchanged", () => {
  expect(toGroundingLevel("grounded")).toBe("grounded");
  expect(toGroundingLevel("mixed")).toBe("mixed");
  expect(toGroundingLevel("general")).toBe("general");
});

test("falls back to general for anything unrecognized, rather than crashing the UI", () => {
  expect(toGroundingLevel("")).toBe("general");
  expect(toGroundingLevel("unexpected-value")).toBe("general");
});
```

Run: `cd mobile && npx jest src/data/__tests__/AssistantRepository.test.ts`
Expected: PASS.

- [ ] **Step 3: Update `dictionary.ts`**

In the `fr` locale's `assistant` block, replace:

```typescript
    assistant: {
      roadmapBadge: "Roadmap · pas encore construit",
      title: "Assistant du guide",
      subtitle: "Pose une question sur le lieu. Réponse sourcée sur le contenu vérifié du guide.",
      exampleQuestion: "Pourquoi construit ici, sur le Corcovado ?",
      exampleAnswer:
        "Le site fut choisi pour sa vue à 360° sur la ville. Le projet a été lancé en 1921 pour marquer le centenaire de l'indépendance du Brésil.",
      exampleSource: "Source : script §2",
      inputPlaceholder: "Poser une question…",
    },
```

with:

```typescript
    assistant: {
      title: "Assistant du guide",
      subtitle: "Pose une question sur le lieu. Réponse sourcée sur le contenu vérifié du guide.",
      inputPlaceholder: "Poser une question…",
      emptyStateInvite: "Pose ta première question sur ce lieu.",
      groundedBadge: "✓ sources vérifiées",
      mixedBadge: "⚠ en partie non vérifié",
      generalBadge: "⚠ information générale, non vérifiée",
      unavailableTitle: "Pas encore disponible",
      unavailableBody: "L'assistant a besoin d'une narration vérifiée pour ce lieu, pas encore publiée dans cette langue.",
      sendError: "Une erreur est survenue. Réessaie.",
    },
```

In the `en` locale's `assistant` block, replace:

```typescript
    assistant: {
      roadmapBadge: "Roadmap · not built yet",
      title: "Guide assistant",
      subtitle: "Ask a question about the place. Answers are sourced from the guide's verified content.",
      exampleQuestion: "Why was it built here, on Corcovado?",
      exampleAnswer:
        "The site was chosen for its 360° view of the city. The project launched in 1921 to mark Brazil's independence centennial.",
      exampleSource: "Source: script §2",
      inputPlaceholder: "Ask a question…",
    },
```

with:

```typescript
    assistant: {
      title: "Guide assistant",
      subtitle: "Ask a question about the place. Answers are sourced from the guide's verified content.",
      inputPlaceholder: "Ask a question…",
      emptyStateInvite: "Ask your first question about this place.",
      groundedBadge: "✓ verified sources",
      mixedBadge: "⚠ partly unverified",
      generalBadge: "⚠ general information, unverified",
      unavailableTitle: "Not available yet",
      unavailableBody: "The assistant needs verified narration for this place, not yet published in this language.",
      sendError: "Something went wrong. Please try again.",
    },
```

In the `pt` locale's `assistant` block, replace:

```typescript
    assistant: {
      roadmapBadge: "Roadmap · ainda não construído",
      title: "Assistente do guia",
      subtitle: "Faça uma pergunta sobre o lugar. Respostas com fontes do conteúdo verificado do guia.",
      exampleQuestion: "Por que foi construído aqui, no Corcovado?",
      exampleAnswer:
        "O local foi escolhido pela vista de 360° da cidade. O projeto foi lançado em 1921 para marcar o centenário da independência do Brasil.",
      exampleSource: "Fonte: roteiro §2",
      inputPlaceholder: "Fazer uma pergunta…",
    },
```

with:

```typescript
    assistant: {
      title: "Assistente do guia",
      subtitle: "Faça uma pergunta sobre o lugar. Respostas com fontes do conteúdo verificado do guia.",
      inputPlaceholder: "Fazer uma pergunta…",
      emptyStateInvite: "Faça sua primeira pergunta sobre este lugar.",
      groundedBadge: "✓ fontes verificadas",
      mixedBadge: "⚠ parcialmente não verificado",
      generalBadge: "⚠ informação geral, não verificada",
      unavailableTitle: "Ainda não disponível",
      unavailableBody: "O assistente precisa de uma narração verificada para este lugar, ainda não publicada neste idioma.",
      sendError: "Ocorreu um erro. Tente novamente.",
    },
```

In the `es` locale's `assistant` block, replace:

```typescript
    assistant: {
      roadmapBadge: "Roadmap · aún no construido",
      title: "Asistente de la guía",
      subtitle: "Haz una pregunta sobre el lugar. Respuestas con fuentes del contenido verificado de la guía.",
      exampleQuestion: "¿Por qué se construyó aquí, en el Corcovado?",
      exampleAnswer:
        "El lugar fue elegido por su vista de 360° de la ciudad. El proyecto se lanzó en 1921 para conmemorar el centenario de la independencia de Brasil.",
      exampleSource: "Fuente: guion §2",
      inputPlaceholder: "Hacer una pregunta…",
    },
```

with:

```typescript
    assistant: {
      title: "Asistente de la guía",
      subtitle: "Haz una pregunta sobre el lugar. Respuestas con fuentes del contenido verificado de la guía.",
      inputPlaceholder: "Hacer una pregunta…",
      emptyStateInvite: "Haz tu primera pregunta sobre este lugar.",
      groundedBadge: "✓ fuentes verificadas",
      mixedBadge: "⚠ parcialmente no verificado",
      generalBadge: "⚠ información general, no verificada",
      unavailableTitle: "Aún no disponible",
      unavailableBody: "El asistente necesita una narración verificada para este lugar, aún no publicada en este idioma.",
      sendError: "Ocurrió un error. Inténtalo de nuevo.",
    },
```

- [ ] **Step 4: Verify all four locale blocks still declare the same key set**

Run: `cd mobile && grep -A 10 '    assistant: {' src/i18n/dictionary.ts`

Confirm all four blocks show exactly these 10 keys (order doesn't matter, presence does): `title`,
`subtitle`, `inputPlaceholder`, `emptyStateInvite`, `groundedBadge`, `mixedBadge`, `generalBadge`,
`unavailableTitle`, `unavailableBody`, `sendError`. No
`roadmapBadge`/`exampleQuestion`/`exampleAnswer`/`exampleSource` should remain anywhere in the file.

- [ ] **Step 5: Rewrite `Assistant.tsx`**

```tsx
// mobile/src/screens/Assistant.tsx
import React, { useEffect, useState } from "react";
import {
  View,
  Text,
  Pressable,
  TextInput,
  StyleSheet,
  ScrollView,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { useAuth } from "../auth/AuthContext";
import { placesRepository } from "../data/PlacesRepository";
import { askAssistant, type ConversationTurn, type GroundingLevel } from "../data/AssistantRepository";
import { colors, fonts, radii } from "../theme/tokens";
import type { Dictionary } from "../i18n/dictionary";

type Props = NativeStackScreenProps<AppStackParamList, "Assistant">;

type Turn = { question: string; answer: string; groundingLevel: GroundingLevel };

function GroundingBadge({ level, t }: { level: GroundingLevel; t: Dictionary }) {
  const isGrounded = level === "grounded";
  const bg = isGrounded ? colors.groundBg : colors.roadmapBg;
  const text = isGrounded ? colors.groundText : colors.roadmapText;
  const label = level === "grounded" ? t.assistant.groundedBadge : level === "mixed" ? t.assistant.mixedBadge : t.assistant.generalBadge;
  return (
    <View style={[styles.sourceChip, { backgroundColor: bg }]}>
      {isGrounded ? (
        <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
          <Polyline points="5 13 10 18 19 7" stroke={text} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" />
        </Svg>
      ) : (
        <Svg width={11} height={11} viewBox="0 0 24 24" fill="none">
          <Path d="M12 9v4M12 17h.01" stroke={text} strokeWidth={2.4} strokeLinecap="round" />
          <Path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" stroke={text} strokeWidth={1.8} strokeLinejoin="round" />
        </Svg>
      )}
      <Text style={[styles.sourceChipText, { color: text }]}>{label}</Text>
    </View>
  );
}

export function AssistantScreen({ route, navigation }: Props) {
  const { t, locale } = useLocale();
  const { token } = useAuth();
  const [available, setAvailable] = useState<boolean | null>(null);
  const [turns, setTurns] = useState<Turn[]>([]);
  const [pendingQuestion, setPendingQuestion] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    placesRepository.getById(route.params.placeId).then((place) => {
      if (!cancelled) setAvailable(place?.narrationStatus === "ready");
    });
    return () => {
      cancelled = true;
    };
  }, [route.params.placeId, locale]);

  async function handleSend() {
    const question = input.trim();
    if (!question || !token || pendingQuestion) return;
    setInput("");
    setError(null);
    setPendingQuestion(question);
    try {
      const history: ConversationTurn[] = turns.map(({ question, answer }) => ({ question, answer }));
      const result = await askAssistant(token, route.params.placeId, locale, question, history);
      setTurns((prev) => [...prev, { question, answer: result.answer, groundingLevel: result.groundingLevel }]);
    } catch {
      // Put the question back in the input rather than silently losing it --
      // the user can retry without retyping it.
      setInput(question);
      setError(t.assistant.sendError);
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

        {available === false ? (
          <View style={styles.unavailable}>
            <Text style={styles.unavailableTitle}>{t.assistant.unavailableTitle}</Text>
            <Text style={styles.unavailableBody}>{t.assistant.unavailableBody}</Text>
          </View>
        ) : (
          <>
            <View style={styles.head}>
              <Text style={styles.title}>{t.assistant.title}</Text>
              <Text style={styles.subtitle}>{t.assistant.subtitle}</Text>
            </View>

            <ScrollView style={styles.chat} contentContainerStyle={styles.chatContent}>
              {turns.length === 0 && !pendingQuestion && <Text style={styles.invite}>{t.assistant.emptyStateInvite}</Text>}
              {turns.map((turn, i) => (
                <View key={i}>
                  <View style={styles.rowUser}>
                    <View style={styles.bubbleUser}>
                      <Text style={styles.bubbleUserText}>{turn.question}</Text>
                    </View>
                  </View>
                  <View style={styles.rowAi}>
                    <View style={styles.bubbleAi}>
                      <Text style={styles.bubbleAiText}>{turn.answer}</Text>
                      <GroundingBadge level={turn.groundingLevel} t={t} />
                    </View>
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
                placeholder={t.assistant.inputPlaceholder}
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
          </>
        )}
      </SafeAreaView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  flexOne: { flex: 1 },
  topbar: { flexDirection: "row", alignItems: "center", gap: 12, paddingHorizontal: 20, paddingTop: 8 },
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
  head: { paddingHorizontal: 22, paddingTop: 20 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 8 },
  subtitle: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, maxWidth: 320 },
  chat: { flex: 1, paddingHorizontal: 20, paddingTop: 22 },
  chatContent: { gap: 14, paddingBottom: 8 },
  invite: { fontFamily: fonts.body, fontSize: 14.5, color: colors.inkFaint, textAlign: "center", marginTop: 40 },
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
  sourceChip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 5,
    marginTop: 10,
    borderRadius: radii.pill,
    paddingVertical: 5,
    paddingHorizontal: 10,
    alignSelf: "flex-start",
  },
  sourceChipText: { fontFamily: fonts.bodyBold, fontSize: 11.5 },
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
  unavailable: { flex: 1, paddingHorizontal: 32, justifyContent: "center", alignItems: "center", gap: 10 },
  unavailableTitle: { fontFamily: fonts.display, fontSize: 20, color: colors.ink, textAlign: "center" },
  unavailableBody: { fontFamily: fonts.body, fontSize: 14, lineHeight: 21, color: colors.inkSoft, textAlign: "center" },
});
```

- [ ] **Step 6: Gate the entry point in `PlaceDetail.tsx`**

In `mobile/src/screens/PlaceDetail.tsx`, find the "ask" `Pressable` (navigates to `Assistant`):

```tsx
        <Pressable
          style={styles.ask}
          onPress={() => navigation.navigate("Assistant", { placeId: place.id })}
        >
```

Replace with:

```tsx
        <Pressable
          style={[styles.ask, place.narrationStatus !== "ready" && styles.askDisabled]}
          disabled={place.narrationStatus !== "ready"}
          onPress={() => navigation.navigate("Assistant", { placeId: place.id })}
        >
```

And add one new style entry to that file's `StyleSheet.create({...})` call, next to the existing `ask:`
entry:

```typescript
  askDisabled: { opacity: 0.4 },
```

- [ ] **Step 7: Manual verification**

There is no component-test convention for screens in this app (matches every other screen), so verify by
running the app:

```bash
cd mobile && npx expo start --port 19010 --web
```

Open a place with a published script in the current locale (Cristo Redentor, if its scripts are still
published in your local Postgres — see the `docker exec ... psql` check from this session's own manual
testing), tap "Poser une question", send a real question, and confirm: a user bubble appears
immediately, a loading indicator appears in the AI bubble slot, then a real answer with the correct
`GroundingBadge` variant replaces it. Then open a place with no published script in the current locale
and confirm the "pas encore disponible" state renders instead of a chat.

- [ ] **Step 8: Commit**

```bash
cd mobile && git add src/data/AssistantRepository.ts src/data/__tests__/AssistantRepository.test.ts src/i18n/dictionary.ts src/screens/Assistant.tsx src/screens/PlaceDetail.tsx
git commit -m "mobile: real per-place assistant chat, replaces the roadmap teaser"
```
