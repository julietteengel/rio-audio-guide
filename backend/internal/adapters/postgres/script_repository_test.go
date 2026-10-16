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

	// reviewer_id référence users(id) (schema.sql) -- il faut un vrai compte
	// pour que MarkReviewed("...") passe la contrainte de clé étrangère,
	// pas juste une chaîne arbitraire.
	email, _ := domain.NewEmail("julie+" + place.ID() + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	reviewer := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := NewUserRepository(pool).Save(ctx, reviewer); err != nil {
		t.Fatalf("save reviewer fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	text, err := domain.NewScriptText("Voici l'escadaria...")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source text")
	if err := script.MarkReviewed(reviewer.ID()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := scriptRepo.FindByID(ctx, script.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status() != domain.ScriptStatusReviewed || found.ReviewerID() != reviewer.ID() {
		t.Fatalf("got status=%v reviewerID=%v, want reviewed by %v", found.Status(), found.ReviewerID(), reviewer.ID())
	}
	if found.ReviewedAt().IsZero() {
		t.Fatal("expected ReviewedAt to be set after round-trip")
	}
}

func TestScriptRepository_FindByPlaceID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Confeitaria Colombo")
	coords, _ := domain.NewCoordinates(-22.9068, -43.1765)
	place := domain.NewPlace(placeName, "historic_site", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	for _, lang := range []domain.Language{domain.LanguageFR, domain.LanguageEN} {
		text, _ := domain.NewScriptText("Texte")
		script := domain.NewScript(place.ID(), lang, text, "source")
		if err := scriptRepo.Save(ctx, script); err != nil {
			t.Fatalf("save script fixture (%s): %v", lang, err)
		}
	}

	found, err := scriptRepo.FindByPlaceID(ctx, place.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("got %d scripts, want 2", len(found))
	}
	languages := map[string]bool{}
	for _, s := range found {
		languages[s.Language().String()] = true
	}
	if !languages["fr"] || !languages["en"] {
		t.Fatalf("got languages %v, want fr and en", languages)
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

func TestScriptRepository_FindAllDraft(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	place := savePlaceFixture(t, NewPlaceRepository(pool), "Pedra do Sal")
	scriptRepo := NewScriptRepository(pool)

	draftText, _ := domain.NewScriptText("Un lieu historique de samba...")
	draft := domain.NewScript(place.ID(), domain.LanguageFR, draftText, "source text")
	if err := scriptRepo.Save(ctx, draft); err != nil {
		t.Fatalf("save draft fixture: %v", err)
	}

	email, _ := domain.NewEmail("julie+" + place.ID() + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	reviewer := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := NewUserRepository(pool).Save(ctx, reviewer); err != nil {
		t.Fatalf("save reviewer fixture: %v", err)
	}
	reviewedText, _ := domain.NewScriptText("Un autre lieu, déjà reviewé...")
	reviewed := domain.NewScript(place.ID(), domain.LanguageEN, reviewedText, "source text")
	if err := reviewed.MarkReviewed(reviewer.ID()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := scriptRepo.Save(ctx, reviewed); err != nil {
		t.Fatalf("save reviewed fixture: %v", err)
	}

	found, err := scriptRepo.FindAllDraft(ctx)
	if err != nil {
		t.Fatalf("find all draft: %v", err)
	}

	var sawDraft, sawReviewed bool
	for _, s := range found {
		if s.ID() == draft.ID() {
			sawDraft = true
			if s.Status() != domain.ScriptStatusDraft {
				t.Fatalf("got status %v for the draft fixture, want draft", s.Status())
			}
		}
		if s.ID() == reviewed.ID() {
			sawReviewed = true
		}
	}
	if !sawDraft {
		t.Fatal("expected FindAllDraft to include the draft script")
	}
	if sawReviewed {
		t.Fatal("expected FindAllDraft to exclude the already-reviewed script")
	}
}
