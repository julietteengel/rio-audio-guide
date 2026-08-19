package application

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func newPublishedScript(t *testing.T, placeID string, lang domain.Language) *domain.Script {
	t.Helper()
	text, err := domain.NewScriptText("Texte")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	s := domain.NewScript(placeID, lang, text, "source")
	if err := s.MarkReviewed("admin"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Publish(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return s
}

func TestMissingLanguages_NoneMissingWhenAllFourPublished(t *testing.T) {
	repo := newFakeScriptRepo()
	for _, lang := range allLanguages {
		s := newPublishedScript(t, "place-1", lang)
		_ = repo.Save(context.Background(), s)
	}

	missing, err := MissingLanguages(context.Background(), repo, "place-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("got missing=%v, want none", missing)
	}
}

func TestMissingLanguages_ReportsUnwrittenAndUnpublishedLanguages(t *testing.T) {
	repo := newFakeScriptRepo()
	// fr: published -- counts as done.
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-1", domain.LanguageFR))
	// en: exists but only reviewed (audio requested, not confirmed ready) --
	// must still count as missing, same as GetPlaceAudio's own rule for an
	// unpublished script.
	text, _ := domain.NewScriptText("Texte")
	enScript := domain.NewScript("place-1", domain.LanguageEN, text, "source")
	_ = enScript.MarkReviewed("admin")
	_ = repo.Save(context.Background(), enScript)
	// es, pt: no script row at all yet.

	missing, err := MissingLanguages(context.Background(), repo, "place-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[domain.Language]bool{domain.LanguageEN: true, domain.LanguageES: true, domain.LanguagePT: true}
	if len(missing) != len(want) {
		t.Fatalf("got missing=%v, want exactly %v", missing, want)
	}
	for _, lang := range missing {
		if !want[lang] {
			t.Fatalf("got unexpected missing language %q", lang)
		}
	}
}

func TestMissingLanguages_UnrelatedPlaceScriptsDontCount(t *testing.T) {
	repo := newFakeScriptRepo()
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-1", domain.LanguageFR))
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-2", domain.LanguageFR))
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-2", domain.LanguageEN))
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-2", domain.LanguageES))
	_ = repo.Save(context.Background(), newPublishedScript(t, "place-2", domain.LanguagePT))

	missing, err := MissingLanguages(context.Background(), repo, "place-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[domain.Language]bool{domain.LanguageEN: true, domain.LanguageES: true, domain.LanguagePT: true}
	if len(missing) != len(want) {
		t.Fatalf("got missing=%v (place-2's scripts must not count toward place-1), want %v", missing, want)
	}
}
