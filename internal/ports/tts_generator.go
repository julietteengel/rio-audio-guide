package ports

import (
	"context"
	"fmt"
	"time"
)

// TTSGenerator is the outbound port to a text-to-speech provider — implemented
// by internal/adapters/elevenlabs.
type TTSGenerator interface {
	Generate(ctx context.Context, text, language, voiceID string) (audioBytes []byte, duration time.Duration, err error)
}

// PermanentError indicates the TTS provider rejected the request in a way
// retrying the same message won't fix (bad API key, invalid text/voice_id).
// The RabbitMQ worker uses this to stop requeueing instead of looping
// forever on an unrecoverable message.
type PermanentError struct {
	StatusCode int
	Body       string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent error (status %d): %s", e.StatusCode, e.Body)
}
