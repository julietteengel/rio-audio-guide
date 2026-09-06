package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// fakePlaceAssistantHTTP is the zero-value-safe default every OTHER test
// file's NewServer(...) call passes for the new 12th argument -- it never
// needs to be configured because those tests don't exercise the assistant
// route at all.
type fakePlaceAssistantHTTP struct{ result ports.AssistantAnswer }

func (f *fakePlaceAssistantHTTP) Ask(_ context.Context, _ string, _ []ports.ConversationTurn, _ string) (ports.AssistantAnswer, error) {
	return f.result, nil
}

func publishedScriptFixtureHTTP(t *testing.T, placeID, language, text string) *domain.Script {
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

func TestAskAssistant_RequiresAuth(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401 without an Authorization header", rec.Code)
	}
}

func TestAskAssistant_Success(t *testing.T) {
	script := publishedScriptFixtureHTTP(t, "place-1", "fr", "Ce lieu a été construit en 1931.")
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	assistant := &fakePlaceAssistantHTTP{result: ports.AssistantAnswer{Answer: "En 1931.", GroundingLevel: "grounded"}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, assistant)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand a-t-il été construit ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp askAssistantResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Answer != "En 1931." {
		t.Fatalf("got answer %q", resp.Answer)
	}
	if resp.GroundingLevel != "grounded" {
		t.Fatalf("got grounding_level %q, want it to survive to JSON as \"grounded\"", resp.GroundingLevel)
	}
}

func TestAskAssistant_NoPublishedScript_Returns404(t *testing.T) {
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 for a place with no published script", rec.Code)
	}
}

func TestAskAssistant_UnpublishedScript_Returns404(t *testing.T) {
	lang, err := domain.NewLanguage("fr")
	if err != nil {
		t.Fatalf("build fixture language: %v", err)
	}
	text, err := domain.NewScriptText("brouillon")
	if err != nil {
		t.Fatalf("build fixture script text: %v", err)
	}
	draft := domain.NewScript("place-1", lang, text, "source") // never reviewed/published -- stays draft
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{draft.ID(): draft}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": "Quand ?"})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 for an unpublished (draft) script", rec.Code)
	}
}

func TestAskAssistant_EmptyQuestion_Returns400(t *testing.T) {
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{})

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"language": "fr", "question": ""})
	req := httptest.NewRequest(http.MethodPost, "/places/place-1/assistant", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400 for an empty question", rec.Code)
	}
}
