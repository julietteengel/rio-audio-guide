// internal/adapters/claude/itinerary_generator_test.go
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

// fakeMessagesAPI simule client.Messages.New sans toucher le réseau --
// renvoie directement le *anthropic.Message construit par le test.
type fakeMessagesAPI struct {
	response *anthropic.Message
	err      error
}

func (f *fakeMessagesAPI) New(_ context.Context, _ anthropic.MessageNewParams, _ ...option.RequestOption) (*anthropic.Message, error) {
	return f.response, f.err
}

func toolUseResponse(t *testing.T, input any) *anthropic.Message {
	t.Helper()
	rawInput, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal fixture input: %v", err)
	}
	// ContentBlockUnion.AsAny() reads from an unexported raw-JSON cache that
	// only gets populated by the SDK's own UnmarshalJSON -- building the
	// struct directly (rather than round-tripping through JSON) leaves that
	// cache empty and AsAny() silently returns a zero-value block, so the
	// fixture must go through json.Unmarshal like a real API response would.
	rawMessage, err := json.Marshal(map[string]any{
		"content": []map[string]any{
			{"type": "tool_use", "id": "toolu_1", "name": "propose_itinerary", "input": json.RawMessage(rawInput)},
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

var candidates = []ports.CandidatePlace{
	{ID: "place-1", Name: "Escadaria Selarón", Category: "monument", Lat: -22.9147, Lon: -43.1806},
	{ID: "place-2", Name: "Parque das Ruínas", Category: "monument", Lat: -22.9207, Lon: -43.1876},
}

func TestItineraryGenerator_Generate_ParsesToolUseResponse(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": false, "place_id": "place-1", "label": "Escadaria Selarón", "time_on_site_minutes": 10, "walk_to_next_minutes": 8},
			{"is_suggestion": true, "label": "Pause déjeuner", "walk_to_next_minutes": 5},
			{"is_suggestion": false, "place_id": "place-2", "label": "Parque das Ruínas", "time_on_site_minutes": 20, "walk_to_next_minutes": 0},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	got, err := gen.Generate(context.Background(), "1h à Santa Teresa, focus street art, avec pause déjeuner", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Art et rue à Santa Teresa" {
		t.Fatalf("got title %q", got.Title)
	}
	if len(got.Stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(got.Stops))
	}
	if got.Stops[0].PlaceID != "place-1" || got.Stops[0].IsSuggestion {
		t.Fatalf("stop 0 should be place-1, got %+v", got.Stops[0])
	}
	if !got.Stops[1].IsSuggestion || got.Stops[1].Label != "Pause déjeuner" {
		t.Fatalf("stop 1 should be the suggestion, got %+v", got.Stops[1])
	}
}

func TestItineraryGenerator_Generate_RejectsUnknownPlaceID(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": false, "place_id": "place-999-not-a-real-candidate", "label": "Lieu inventé", "time_on_site_minutes": 10, "walk_to_next_minutes": 0},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error for a place id outside the candidate list, got nil")
	}
}

func TestItineraryGenerator_Generate_RejectsMoreThanOneSuggestion(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": true, "label": "Pause déjeuner", "walk_to_next_minutes": 5},
			{"is_suggestion": true, "label": "Pause café", "walk_to_next_minutes": 5},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error for more than one suggestion stop, got nil")
	}
}

func TestItineraryGenerator_Generate_PropagatesAPIError(t *testing.T) {
	fake := &fakeMessagesAPI{err: errors.New("connection reset")}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected the underlying API error to propagate")
	}
}

func TestItineraryGenerator_Generate_ErrorsWhenNoToolUseBlock(t *testing.T) {
	fake := &fakeMessagesAPI{response: &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "I couldn't come up with an itinerary."}},
	}}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error when Claude didn't call the tool")
	}
}

var _ ports.ItineraryGenerator = (*ItineraryGenerator)(nil)
