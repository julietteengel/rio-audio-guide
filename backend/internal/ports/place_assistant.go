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
