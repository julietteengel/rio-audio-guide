package rabbitmq

import (
	"context"
	"encoding/json"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AudioJobPublisher struct {
	channel *amqp.Channel
	mu      sync.Mutex
}

const TTSJobQueue = "tts_jobs"

type ttsJobMessage struct {
	AudioFileID string `json:"audio_file_id"`
	ScriptID    string `json:"script_id"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	VoiceID     string `json:"voice_id"`
}

func NewAudioJobPublisher(channel *amqp.Channel) (*AudioJobPublisher, error) {
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &AudioJobPublisher{channel: channel}, nil
}

func (p *AudioJobPublisher) PublishTTSJob(ctx context.Context, audioFileID, scriptID, text, language, voiceID string) error {
	body, err := json.Marshal(ttsJobMessage{
		AudioFileID: audioFileID,
		ScriptID:    scriptID,
		Text:        text,
		Language:    language,
		VoiceID:     voiceID,
	})
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.channel.PublishWithContext(ctx, "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}
