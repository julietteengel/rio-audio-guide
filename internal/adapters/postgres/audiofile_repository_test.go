//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestAudioFileRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Museu Nacional")
	coords, _ := domain.NewCoordinates(-22.9058, -43.2256)
	place := domain.NewPlace(placeName, "museum", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptText, _ := domain.NewScriptText("The National Museum...")
	script := domain.NewScript(place.ID(), domain.LanguageEN, scriptText, "source text")
	scriptRepo := NewScriptRepository(pool)
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save script fixture: %v", err)
	}

	audioRepo := NewAudioFileRepository(pool)
	audioFile, err := domain.NewAudioFile(script.ID(), "voice-1")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	if err := audioFile.MarkGenerating(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	audio, err := domain.NewGeneratedAudio("s3://bucket/a.mp3", "s3://bucket/a.json", 37*time.Second)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	if err := audioFile.MarkReady(audio); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := audioRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := audioRepo.FindByID(ctx, audioFile.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status() != domain.AudioFileStatusReady || found.Audio().StorageURL() != audioFile.Audio().StorageURL() {
		t.Fatalf("got %+v, want %+v", found, audioFile)
	}
	if found.Audio().Duration() != 37*time.Second {
		t.Fatalf("got duration %v, want 37s", found.Audio().Duration())
	}
}

func TestAudioFileRepository_QueuedHasNoAudioYet(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Test Place")
	coords, _ := domain.NewCoordinates(-22.9, -43.2)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	_ = placeRepo.Save(ctx, place)

	scriptText, _ := domain.NewScriptText("Text")
	script := domain.NewScript(place.ID(), domain.LanguageEN, scriptText, "source")
	scriptRepo := NewScriptRepository(pool)
	_ = scriptRepo.Save(ctx, script)

	audioRepo := NewAudioFileRepository(pool)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	if err := audioRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := audioRepo.FindByID(ctx, audioFile.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status() != domain.AudioFileStatusQueued {
		t.Fatalf("got status %v, want queued", found.Status())
	}
	if found.Audio().StorageURL() != "" {
		t.Fatalf("expected no storage URL yet, got %q", found.Audio().StorageURL())
	}
}
