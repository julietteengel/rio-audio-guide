package application

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestRetryAudioGeneration_RequeuesWithSameVoice(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	publisher := &fakePublisher{}
	ctx := context.Background()

	text, _ := domain.NewScriptText("Le Cristo Redentor...")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source text")
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("unexpected error saving fixture: %v", err)
	}

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkFailed("elevenlabs: transient 500")
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("unexpected error saving fixture: %v", err)
	}

	if err := RetryAudioGeneration(ctx, scriptRepo, audioFileRepo, publisher, audioFile.ID()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if saved.Status() != domain.AudioFileStatusQueued {
		t.Fatalf("got status %v, want queued", saved.Status())
	}
	if saved.FailureReason() != "" {
		t.Fatalf("got failure reason %q, want cleared", saved.FailureReason())
	}

	if len(publisher.published) != 1 || publisher.published[0] != audioFile.ID() {
		t.Fatalf("got published=%v, want exactly [%q]", publisher.published, audioFile.ID())
	}
}

func TestRetryAudioGeneration_RejectsNonFailedAudioFile(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	publisher := &fakePublisher{}
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("unexpected error saving fixture: %v", err)
	}

	// Still "queued" -- never failed, nothing to retry.
	if err := RetryAudioGeneration(ctx, scriptRepo, audioFileRepo, publisher, audioFile.ID()); err == nil {
		t.Fatal("expected an error retrying an audio file that was never marked failed")
	}
	if len(publisher.published) != 0 {
		t.Fatalf("got %d published jobs, want 0", len(publisher.published))
	}
}
