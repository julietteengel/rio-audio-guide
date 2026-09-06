// internal/application/ask_assistant_test.go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakePlaceAssistant struct {
	result       ports.AssistantAnswer
	err          error
	lastHistory  []ports.ConversationTurn
	lastQuestion string
}

func (f *fakePlaceAssistant) Ask(_ context.Context, _ string, history []ports.ConversationTurn, question string) (ports.AssistantAnswer, error) {
	f.lastHistory = history
	f.lastQuestion = question
	return f.result, f.err
}

func publishedScriptFixture(t *testing.T, placeID, language, text string) *domain.Script {
	t.Helper()
	lang, err := domain.NewLanguage(language)
	if err != nil {
		t.Fatalf("build fixture language: %v", err)
	}
	scriptText, err := domain.NewScriptText(text)
	if err != nil {
		t.Fatalf("build fixture script text: %v", err)
	}
	script := domain.NewScript(placeID, lang, scriptText, "source")
	if err := script.MarkReviewed("reviewer-1"); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}
	if err := script.Publish(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return script
}

type fakeScriptRepoApp struct {
	scripts map[string]*domain.Script // keyed by placeID+"|"+language
}

func (f *fakeScriptRepoApp) Save(context.Context, *domain.Script) error { return nil }
func (f *fakeScriptRepoApp) FindByID(context.Context, string) (*domain.Script, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeScriptRepoApp) FindByPlaceIDAndLanguage(_ context.Context, placeID, language string) (*domain.Script, error) {
	s, ok := f.scripts[placeID+"|"+language]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}
func (f *fakeScriptRepoApp) FindByPlaceID(context.Context, string) ([]*domain.Script, error) {
	return nil, errors.New("not implemented in fake")
}

func TestAskAssistant_AsksWithTheScriptsGroundedText(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "Ce lieu a été construit en 1931.")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{result: ports.AssistantAnswer{Answer: "En 1931.", GroundingLevel: "grounded"}}

	got, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "Quand a-t-il été construit ?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Answer != "En 1931." || got.GroundingLevel != "grounded" {
		t.Fatalf("got %+v", got)
	}
	if assistant.lastQuestion != "Quand a-t-il été construit ?" {
		t.Fatalf("got question %q passed to assistant", assistant.lastQuestion)
	}
}

func TestAskAssistant_RejectsAPlaceWithNoPublishedScript(t *testing.T) {
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{}}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrNoPublishedScript) {
		t.Fatalf("got %v, want ErrNoPublishedScript", err)
	}
}

func TestAskAssistant_RejectsAnUnpublishedScript(t *testing.T) {
	lang, _ := domain.NewLanguage("fr")
	text, _ := domain.NewScriptText("brouillon")
	draft := domain.NewScript("place-1", lang, text, "source") // never reviewed/published -- stays draft
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": draft}}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrNoPublishedScript) {
		t.Fatalf("got %v, want ErrNoPublishedScript", err)
	}
}

func TestAskAssistant_TruncatesHistoryToLastSixTurns(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "texte")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{result: ports.AssistantAnswer{Answer: "ok", GroundingLevel: "grounded"}}

	history := make([]ports.ConversationTurn, 9)
	for i := range history {
		history[i] = ports.ConversationTurn{Question: "q", Answer: "a"}
	}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", history, "question")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assistant.lastHistory) != 6 {
		t.Fatalf("got %d history turns passed to the assistant, want 6 (truncated from 9)", len(assistant.lastHistory))
	}
}

func TestAskAssistant_WrapsAssistantError(t *testing.T) {
	script := publishedScriptFixture(t, "place-1", "fr", "texte")
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{"place-1|fr": script}}
	assistant := &fakePlaceAssistant{err: errors.New("rate limited")}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, ErrAssistantFailed) {
		t.Fatalf("got %v, want an error wrapping ErrAssistantFailed", err)
	}
}
