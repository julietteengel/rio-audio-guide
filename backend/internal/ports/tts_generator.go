package ports

import (
	"context"
	"fmt"
	"time"
)

// TTSGenerator is the outbound port to a text-to-speech provider — implemented
// by internal/adapters/awspolly. Generate is expected to complete the ENTIRE
// synthesis (including any provider-side async polling) and return final,
// already-uploaded storage locations -- not raw bytes. Amazon Polly's
// long-text API writes directly to S3 itself; there is no upload step left
// for the worker to perform.
type TTSGenerator interface {
	Generate(ctx context.Context, text, language, voiceID string) (storageURL, timestampsURL string, duration time.Duration, err error)
}

// PermanentError indicates the TTS provider rejected the request in a way
// retrying the same message won't fix (bad API key, invalid text/voice_id,
// unsupported language, task failed for a non-transient reason). The
// RabbitMQ worker uses this to stop requeueing instead of looping forever
// on an unrecoverable message.
type PermanentError struct {
	StatusCode int
	Body       string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent error (status %d): %s", e.StatusCode, e.Body)
}
