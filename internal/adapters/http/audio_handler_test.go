package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type fakeAudioStorage struct{}

func (fakeAudioStorage) Upload(_ context.Context, _ string, _ []byte, _ string) (string, error) {
	return "", errNotImplementedInFake
}
func (fakeAudioStorage) PresignURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://presigned.example.com/" + key + "?X-Amz-Signature=fake", nil
}

func TestGetPlaceAudio_Ready(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(placeName, "monument", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")

	audio, _ := domain.NewGeneratedAudio("s3://rio-audio-guide/abc123.mp3", "", 30*time.Second)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady(audio)

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo,
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "presigned.example.com/abc123.mp3") {
		t.Fatalf("expected a presigned URL in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_FailsOpenWhenCacheErrors(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(placeName, "monument", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")

	audio, _ := domain.NewGeneratedAudio("s3://rio-audio-guide/abc123.mp3", "", 30*time.Second)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady(audio)

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo,
		&fakePublisher{}, fakeAudioStorage{}, erroringCache{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 — a cache error must never fail the request: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "presigned.example.com/abc123.mp3") {
		t.Fatalf("expected a presigned URL in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_NotReadyYet(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cais do Valongo")
	coords, _ := domain.NewCoordinates(-22.8966, -43.1871)
	place := domain.NewPlace(placeName, "landmark", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo,
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "generating") {
		t.Fatalf("expected status \"generating\" in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_NoScriptForLanguage(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Quinta da Boa Vista")
	coords, _ := domain.NewCoordinates(-22.9058, -43.2244)
	place := domain.NewPlace(placeName, "park", coords, "", "overture", "correct")

	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}},
		&fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache())

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
