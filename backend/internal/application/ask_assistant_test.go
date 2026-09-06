// internal/application/ask_assistant_test.go
package application

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

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
		return nil, pgx.ErrNoRows
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

func TestAskAssistant_PropagatesRepositoryErrorWhenScriptMissing(t *testing.T) {
	repo := &fakeScriptRepoApp{scripts: map[string]*domain.Script{}}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("got %v, want an error wrapping pgx.ErrNoRows (a genuinely missing script propagates the repo's own error now, not ErrNoPublishedScript)", err)
	}
}

func TestAskAssistant_PropagatesUnexpectedRepositoryError(t *testing.T) {
	repo := &fakeScriptRepoAppErroring{err: errors.New("connection reset by peer")}
	assistant := &fakePlaceAssistant{}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", nil, "question")
	if err == nil {
		t.Fatal("expected a real repository failure to propagate, not be swallowed")
	}
	if errors.Is(err, ErrNoPublishedScript) {
		t.Fatal("a real repository failure must never be misreported as ErrNoPublishedScript")
	}
}

// fakeScriptRepoAppErroring simulates a genuine repository failure (a DB
// outage, not a missing row) -- distinct from fakeScriptRepoApp's map-miss
// case, which now returns pgx.ErrNoRows specifically.
type fakeScriptRepoAppErroring struct{ err error }

func (f *fakeScriptRepoAppErroring) Save(context.Context, *domain.Script) error { return nil }
func (f *fakeScriptRepoAppErroring) FindByID(context.Context, string) (*domain.Script, error) {
	return nil, f.err
}
func (f *fakeScriptRepoAppErroring) FindByPlaceIDAndLanguage(context.Context, string, string) (*domain.Script, error) {
	return nil, f.err
}
func (f *fakeScriptRepoAppErroring) FindByPlaceID(context.Context, string) ([]*domain.Script, error) {
	return nil, f.err
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
		history[i] = ports.ConversationTurn{Question: fmt.Sprintf("q%d", i), Answer: fmt.Sprintf("a%d", i)}
	}

	_, err := AskAssistant(context.Background(), assistant, repo, "place-1", "fr", history, "question")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assistant.lastHistory) != 6 {
		t.Fatalf("got %d history turns passed to the assistant, want 6 (truncated from 9)", len(assistant.lastHistory))
	}
	// The LAST 6 of the original 9 (indices 3-8), not just any 6 -- a
	// history[:6] bug would pass the count check above but fail this one.
	for i, turn := range assistant.lastHistory {
		wantIndex := i + 3
		if turn.Question != fmt.Sprintf("q%d", wantIndex) || turn.Answer != fmt.Sprintf("a%d", wantIndex) {
			t.Fatalf("turn %d: got {%q, %q}, want {q%d, a%d} (the truncation must keep the most recent turns)", i, turn.Question, turn.Answer, wantIndex, wantIndex)
		}
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
