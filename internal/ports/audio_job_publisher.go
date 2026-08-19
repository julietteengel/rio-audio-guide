package ports

import "context"

// AudioJobPublisher is the outbound port to the TTS job queue (RabbitMQ, Task 10).
type AudioJobPublisher interface {
	PublishTTSJob(ctx context.Context, audioFileID, scriptID, text, language, voiceID string) error
}
