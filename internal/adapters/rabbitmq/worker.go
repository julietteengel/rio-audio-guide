package rabbitmq

import (
	"context"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type Worker struct {
	channel       *amqp.Channel
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	storage       ports.AudioStorage
}

func NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, storage ports.AudioStorage) (*Worker, error) {
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &Worker{channel: channel, scriptRepo: scriptRepo, audioFileRepo: audioFileRepo, storage: storage}, nil
}

// Run consomme tts_jobs jusqu'à annulation du ctx. Bloquant — à lancer dans
// sa propre goroutine ou son propre binaire (cmd/worker).
func (w *Worker) Run(ctx context.Context) error {
	msgs, err := w.channel.Consume(TTSJobQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			w.handle(ctx, msg)
		}
	}
}

func (w *Worker) handle(ctx context.Context, msg amqp.Delivery) {
	var job ttsJobMessage
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		log.Printf("tts worker: bad message, dropping: %v", err)
		_ = msg.Nack(false, false) // malformé — jamais requeue, boucle infinie sinon
		return
	}

	if err := application.StartAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID); err != nil {
		log.Printf("tts worker: start generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true) // requeue — probablement transitoire
		return
	}

	audioBytes, duration := generateAudioStub(job.Text)
	storageURL, err := w.storage.Upload(ctx, job.AudioFileID+".mp3", audioBytes, "audio/mpeg")
	if err != nil {
		log.Printf("tts worker: upload failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	if err := application.CompleteAudioGeneration(ctx, w.scriptRepo, w.audioFileRepo, job.AudioFileID, storageURL, "", duration); err != nil {
		log.Printf("tts worker: complete generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	_ = msg.Ack(false)
}

// generateAudioStub remplace le vrai appel TTS (ElevenLabs) — pas construit ici,
// nécessiterait une clé API et sa propre conception. Renvoie un résultat
// plausible mais factice pour exercer réellement le reste du pipeline (upload,
// Postgres, transition Script→published).
func generateAudioStub(text string) (audioBytes []byte, duration time.Duration) {
	wordCount := len(text) / 5
	duration = time.Duration(wordCount) * 400 * time.Millisecond
	return []byte("STUB-AUDIO:" + text), duration
}
