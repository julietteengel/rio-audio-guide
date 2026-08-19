package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type manifestFixture struct {
	place      *domain.Place
	script     *domain.Script
	audioFile  *domain.AudioFile
	placeRepo  *fakePlaceRepo
	scriptRepo *fakeScriptRepo
	audioRepo  *fakeAudioFileRepo
}

// newReadyManifestFixture builds one place with a Published FR script and a
// Ready audio file — the "fully downloadable" case the manifest is for.
func newReadyManifestFixture(t *testing.T) manifestFixture {
	t.Helper()
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")
	script := newPublishedScript(place.ID(), domain.LanguageFR, "Inaugurée en 1931...", "wikipedia extract")

	audioFile, err := domain.NewAudioFile(script.ID(), "voice-1")
	if err != nil {
		t.Fatalf("NewAudioFile: %v", err)
	}
	if err := audioFile.MarkGenerating(); err != nil {
		t.Fatalf("MarkGenerating: %v", err)
	}
	audio, err := domain.NewGeneratedAudio("s3://bucket/audio.mp3", "", 120*time.Second)
	if err != nil {
		t.Fatalf("NewGeneratedAudio: %v", err)
	}
	if err := audioFile.MarkReady(audio); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}

	return manifestFixture{
		place:      place,
		script:     script,
		audioFile:  audioFile,
		placeRepo:  &fakePlaceRepo{places: []*domain.Place{place}},
		scriptRepo: &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}},
		audioRepo:  &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}},
	}
}

func TestGetCityManifest_IncludesFullyReadyPlace(t *testing.T) {
	fx := newReadyManifestFixture(t)
	server := NewServer(fx.placeRepo, fx.scriptRepo, fx.audioRepo, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/cities/rio/manifest?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("expected manifest to contain the ready place, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Inaugurée en 1931") {
		t.Fatalf("expected manifest to contain narration text, got %s", rec.Body.String())
	}
}

func TestGetCityManifest_OmitsPlaceWithUnpublishedScript(t *testing.T) {
	name, _ := domain.NewPlaceName("Escadaria Selarón")
	coords, _ := domain.NewCoordinates(-22.9153, -43.1811)
	place := domain.NewPlace(name, "landmark", coords, "", "wikidata", "rich")
	scriptText, _ := domain.NewScriptText("Pas encore relu")
	script := domain.NewScript(place.ID(), domain.LanguageFR, scriptText, "source") // reste Draft

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	server := NewServer(placeRepo, scriptRepo, &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(),
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/cities/rio/manifest?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Escadaria Selarón") {
		t.Fatalf("place with unpublished script must be omitted, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Pas encore relu") {
		t.Fatalf("draft narration must never leak into the manifest, got %s", rec.Body.String())
	}
}

func TestGetCityManifest_OmitsPlaceWithNoScriptForLanguage(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/cities/rio/manifest?language=es", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("place with no script in the requested language must be omitted, got %s", rec.Body.String())
	}
}

func TestGetCityManifest_UnknownCityIs404(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/cities/sao-paulo/manifest?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", rec.Code)
	}
}

func TestGetCityManifest_RequiresLanguageParam(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/cities/rio/manifest", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
}

func TestGetCityManifest_CachesOnSecondCall(t *testing.T) {
	fx := newReadyManifestFixture(t)
	cache := newFakeCache()
	server := NewServer(fx.placeRepo, fx.scriptRepo, fx.audioRepo, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, cache, fakeTokenIssuer{})

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/cities/rio/manifest?language=fr", nil)
		rec := httptest.NewRecorder()
		server.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: got status %d, want 200", i, rec.Code)
		}
	}

	if cache.sets != 1 {
		t.Fatalf("got %d cache writes, want exactly 1 (second call should have hit the cache)", cache.sets)
	}
}
