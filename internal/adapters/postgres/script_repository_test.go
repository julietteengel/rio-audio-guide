//go:build integration

package postgres

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestScriptRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Escadaria Selarón")
	coords, _ := domain.NewCoordinates(-22.9147, -43.1806)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	text, err := domain.NewScriptText("Voici l'escadaria...")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source text")
	if err := script.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := scriptRepo.FindByID(ctx, script.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status() != domain.ScriptStatusReviewed || found.Reviewer() != "julie" {
		t.Fatalf("got status=%v reviewer=%v, want reviewed by julie", found.Status(), found.Reviewer())
	}
	if found.ReviewedAt().IsZero() {
		t.Fatal("expected ReviewedAt to be set after round-trip")
	}
}

func TestScriptRepository_FindByPlaceIDAndLanguage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Theatro Municipal")
	coords, _ := domain.NewCoordinates(-22.9105, -43.1765)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	text, _ := domain.NewScriptText("Voici le Theatro Municipal...")
	script := domain.NewScript(place.ID(), domain.LanguagePT, text, "source text")
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save script fixture: %v", err)
	}

	found, err := scriptRepo.FindByPlaceIDAndLanguage(ctx, place.ID(), "pt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != script.ID() {
		t.Fatalf("got ID %q, want %q", found.ID(), script.ID())
	}

	if _, err := scriptRepo.FindByPlaceIDAndLanguage(ctx, place.ID(), "es"); err == nil {
		t.Fatal("expected an error for a language with no script, got nil")
	}
}
