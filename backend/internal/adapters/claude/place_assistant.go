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
