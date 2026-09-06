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
