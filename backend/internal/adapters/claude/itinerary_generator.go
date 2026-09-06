// internal/adapters/claude/itinerary_generator.go
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"rioaudioguide/backend/internal/ports"
)

// messagesAPI n'expose que la méthode utilisée ici -- *anthropic.MessageService
// (client.Messages) la satisfait déjà structurellement, permettant de tester
// avec un faux client Go plutôt que de simuler la requête HTTP réelle du SDK,
// même convention que pollyAPI dans internal/adapters/awspolly.
type messagesAPI interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

type ItineraryGenerator struct {
	client messagesAPI
}

func NewItineraryGenerator(client messagesAPI) *ItineraryGenerator {
	return &ItineraryGenerator{client: client}
}

const systemPrompt = `You compose short walking itineraries from Rio de Janeiro's cultural sites for a tourist audio-guide app.

Rules:
- You may ONLY reference places from the candidate list given to you, by their exact place_id. Never invent a place, never reference a place not in the list.
- You may optionally add ONE additional stop with is_suggestion=true for a meal break, drawn from your own general knowledge of the area -- this is the only stop allowed without a real place_id. Never add more than one.
- Respect the user's stated time budget, theme, and neighborhood.
- Always call propose_itinerary with your answer -- never answer in plain text.`

var proposeItineraryTool = anthropic.ToolParam{
	Name:        "propose_itinerary",
	Description: anthropic.String("Propose a walking itinerary built only from the given candidate places, plus at most one optional meal-break suggestion."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"title": map[string]any{"type": "string"},
			"stops": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"is_suggestion":        map[string]any{"type": "boolean"},
						"place_id":             map[string]any{"type": "string"},
						"label":                map[string]any{"type": "string"},
						"time_on_site_minutes": map[string]any{"type": "integer"},
						"walk_to_next_minutes": map[string]any{"type": "integer"},
					},
					"required": []string{"is_suggestion", "label", "walk_to_next_minutes"},
				},
			},
		},
		Required: []string{"title", "stops"},
	},
}

type toolStopInput struct {
	IsSuggestion      bool   `json:"is_suggestion"`
	PlaceID           string `json:"place_id"`
	Label             string `json:"label"`
	TimeOnSiteMinutes int    `json:"time_on_site_minutes"`
	WalkToNextMinutes int    `json:"walk_to_next_minutes"`
}

type toolInput struct {
	Title string          `json:"title"`
	Stops []toolStopInput `json:"stops"`
}

func (g *ItineraryGenerator) Generate(ctx context.Context, request string, candidates []ports.CandidatePlace) (ports.GeneratedItinerary, error) {
	candidateLines := make([]string, 0, len(candidates))
	validPlaceIDs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		candidateLines = append(candidateLines, fmt.Sprintf("- %s (%s): %.5f,%.5f, category=%s", c.ID, c.Name, c.Lat, c.Lon, c.Category))
		validPlaceIDs[c.ID] = true
	}
	userMessage := fmt.Sprintf("Request: %s\n\nCandidate places:\n%s", request, joinLines(candidateLines))

	resp, err := g.client.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-opus-5",
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &proposeItineraryTool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "propose_itinerary"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
	})
	if err != nil {
		return ports.GeneratedItinerary{}, fmt.Errorf("claude: generate itinerary: %w", err)
	}

	var parsed *toolInput
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok && variant.Name == "propose_itinerary" {
			var in toolInput
			if err := json.Unmarshal(variant.Input, &in); err != nil {
				return ports.GeneratedItinerary{}, fmt.Errorf("claude: parse propose_itinerary input: %w", err)
			}
			parsed = &in
			break
		}
	}
	if parsed == nil {
		return ports.GeneratedItinerary{}, fmt.Errorf("claude: response contained no propose_itinerary tool call")
	}

	// Defense in depth: internal/application/generate_itinerary.go independently
	// validates every place_id against its own candidate list and rejects more
	// than one suggestion stop too. This adapter-level check is not the only
	// place this happens anymore -- it catches the same problems earlier,
	// right where the LLM's raw response is parsed, and fails fast before any
	// round-trip back through the application layer.
	suggestionCount := 0
	stops := make([]ports.GeneratedStop, 0, len(parsed.Stops))
	for _, s := range parsed.Stops {
		if s.IsSuggestion {
			suggestionCount++
			if suggestionCount > 1 {
				return ports.GeneratedItinerary{}, fmt.Errorf("claude: more than one suggestion stop is not allowed")
			}
		} else if !validPlaceIDs[s.PlaceID] {
			return ports.GeneratedItinerary{}, fmt.Errorf("claude: place id %q is not in the candidate list", s.PlaceID)
		}
		stops = append(stops, ports.GeneratedStop{
			IsSuggestion:      s.IsSuggestion,
			PlaceID:           s.PlaceID,
			Label:             s.Label,
			TimeOnSiteMinutes: s.TimeOnSiteMinutes,
			WalkToNextMinutes: s.WalkToNextMinutes,
		})
	}

	return ports.GeneratedItinerary{Title: parsed.Title, Stops: stops}, nil
}

func joinLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l)
	}
	return b.String()
}
