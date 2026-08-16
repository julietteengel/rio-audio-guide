package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

type fakePlaceRepo struct{ places []*domain.Place }

func (f *fakePlaceRepo) Save(_ context.Context, _ *domain.Place) error { return nil }
func (f *fakePlaceRepo) FindByID(_ context.Context, _ string) (*domain.Place, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakePlaceRepo) FindActiveInBoundingBox(_ context.Context, _, _, _, _ float64) ([]*domain.Place, error) {
	return f.places, nil
}

type fakeScriptRepo struct{ scripts map[string]*domain.Script }

func (f *fakeScriptRepo) Save(_ context.Context, s *domain.Script) error {
	f.scripts[s.ID()] = s
	return nil
}
func (f *fakeScriptRepo) FindByID(_ context.Context, id string) (*domain.Script, error) {
	s, ok := f.scripts[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}

type fakeAudioFileRepo struct{ files map[string]*domain.AudioFile }

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.files[a.ID()] = a
	return nil
}
func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	a, ok := f.files[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

type fakePublisher struct{ published int }

func (f *fakePublisher) PublishTTSJob(_ context.Context, _, _, _, _, _ string) error {
	f.published++
	return nil
}

func TestListPlaces(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{})

	req := httptest.NewRequest(http.MethodGet, "/places", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("expected response to contain place name, got %s", rec.Body.String())
	}
}

func TestReviewScript(t *testing.T) {
	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
	publisher := &fakePublisher{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, audioFileRepo, publisher)

	body := strings.NewReader(`{"reviewer":"julie","voice_id":"voice-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/scripts/"+script.ID()+"/review", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if publisher.published != 1 {
		t.Fatalf("got %d published jobs, want 1", publisher.published)
	}
}
