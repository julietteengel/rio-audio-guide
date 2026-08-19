package application

import (
	"context"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestStartAudioGeneration(t *testing.T) {
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	_ = audioFileRepo.Save(ctx, audioFile)

	if err := StartAudioGeneration(ctx, audioFileRepo, audioFile.ID()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if saved.Status() != domain.AudioFileStatusGenerating {
		t.Fatalf("got status %v, want generating", saved.Status())
	}
}

// Un message RabbitMQ redelivré rejoue StartAudioGeneration sur un AudioFile
// déjà "generating" : ce doit être un no-op, pas une erreur — sinon le worker
// nack/requeue en boucle et le fichier reste bloqué à jamais.
func TestStartAudioGeneration_IsIdempotentWhenAlreadyGenerating(t *testing.T) {
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	_ = audioFileRepo.Save(ctx, audioFile)

	if err := StartAudioGeneration(ctx, audioFileRepo, audioFile.ID()); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if err := StartAudioGeneration(ctx, audioFileRepo, audioFile.ID()); err != nil {
		t.Fatalf("unexpected error on redelivery: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if saved.Status() != domain.AudioFileStatusGenerating {
		t.Fatalf("got status %v, want generating", saved.Status())
	}
}

func TestCompleteAudioGeneration(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	text, _ := domain.NewScriptText("Text")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(ctx, script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFileRepo.Save(ctx, audioFile)

	err := CompleteAudioGeneration(ctx, scriptRepo, audioFileRepo, audioFile.ID(), "s3://bucket/a.mp3", "s3://bucket/a.json", 42*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	savedAudio, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if savedAudio.Status() != domain.AudioFileStatusReady {
		t.Fatalf("got audio status %v, want ready", savedAudio.Status())
	}

	savedScript, _ := scriptRepo.FindByID(ctx, script.ID())
	if savedScript.Status() != domain.ScriptStatusPublished {
		t.Fatalf("got script status %v, want published", savedScript.Status())
	}
}

func TestFailAudioGeneration(t *testing.T) {
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFileRepo.Save(ctx, audioFile)

	if err := FailAudioGeneration(ctx, audioFileRepo, audioFile.ID(), "TTS quota exceeded"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if saved.Status() != domain.AudioFileStatusFailed {
		t.Fatalf("got status %v, want failed", saved.Status())
	}
	if saved.FailureReason() != "TTS quota exceeded" {
		t.Fatalf("got failure reason %q, want %q", saved.FailureReason(), "TTS quota exceeded")
	}
}
