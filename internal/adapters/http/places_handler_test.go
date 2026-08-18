package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func newPublishedScript(placeID string, language domain.Language, text, source string) *domain.Script {
	scriptText, _ := domain.NewScriptText(text)
	script := domain.NewScript(placeID, language, scriptText, source)
	_ = script.MarkReviewed("reviewer")
	_ = script.Publish()
	return script
}

func TestGetPlaceDetail_ReturnsPublishedNarration(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")
	script := newPublishedScript(place.ID(), domain.LanguageFR, "Inaugurée en 1931...", "wikipedia extract")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	server := NewServer(placeRepo, scriptRepo, &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}},
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Inaugurée en 1931") {
		t.Fatalf("expected response to contain narration text, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("expected response to contain place name, got %s", rec.Body.String())
	}
}

func TestGetPlaceDetail_NotFoundWhenPlaceMissing(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/does-not-exist?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPlaceDetail_NotFoundWhenNoScriptForLanguage(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPlaceDetail_AcceptedWhenScriptNotPublished(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	scriptText, _ := domain.NewScriptText("Brouillon pas encore relu")
	script := domain.NewScript(place.ID(), domain.LanguageFR, scriptText, "source") // reste Draft

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	server := NewServer(placeRepo, scriptRepo, &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}},
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202 (narration not yet published): %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Brouillon") {
		t.Fatalf("draft narration text must never be exposed, got %s", rec.Body.String())
	}
}

func TestListPlaces_FiltersByQueryParam(t *testing.T) {
	christName, _ := domain.NewPlaceName("Cristo Redentor")
	christCoords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	christ := domain.NewPlace(christName, "monument", christCoords, "", "wikidata", "rich")

	stairsName, _ := domain.NewPlaceName("Escadaria Selarón")
	stairsCoords, _ := domain.NewCoordinates(-22.9153, -43.1811)
	stairs := domain.NewPlace(stairsName, "landmark", stairsCoords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{christ, stairs}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places?q=cristo", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("expected matching place in results, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Escadaria") {
		t.Fatalf("non-matching place must be filtered out, got %s", rec.Body.String())
	}
}

func TestGetPlaceDetail_RequiresLanguageParam(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/some-id", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}
