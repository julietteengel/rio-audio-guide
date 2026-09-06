// internal/application/ask_assistant.go
package application

import (
	"context"
	"errors"
	"fmt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrNoPublishedScript is returned when the place's script exists but isn't
// published yet -- a genuinely-missing script (no row at all) is NOT this
// error; that case propagates as the repository's own raw error (wrapped),
// so the HTTP layer can apply this project's usual pgx.ErrNoRows-vs-anything-
// else distinction (see internal/adapters/http/place_assistant_handler.go),
// the same as every other route that calls FindByPlaceIDAndLanguage.
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
		return ports.AssistantAnswer{}, fmt.Errorf("application: script lookup for assistant: %w", err)
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
