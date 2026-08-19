package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
)

var errNotImplementedInFake = errors.New("not implemented in fake")

type fakePlaceRepo struct{ places []*domain.Place }

func (f *fakePlaceRepo) Save(_ context.Context, _ *domain.Place) error { return nil }
func (f *fakePlaceRepo) FindByID(_ context.Context, id string) (*domain.Place, error) {
	for _, p := range f.places {
		if p.ID() == id {
			return p, nil
		}
	}
	return nil, pgx.ErrNoRows
}
func (f *fakePlaceRepo) FindByName(_ context.Context, _ string) (*domain.Place, error) {
	return nil, errNotImplementedInFake
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
		return nil, errNotImplementedInFake
	}
	return s, nil
}
func (f *fakeScriptRepo) FindByPlaceIDAndLanguage(_ context.Context, placeID, language string) (*domain.Script, error) {
	for _, s := range f.scripts {
		if s.PlaceID() == placeID && s.Language().String() == language {
			return s, nil
		}
	}
	return nil, pgx.ErrNoRows
}

type fakeAudioFileRepo struct{ files map[string]*domain.AudioFile }

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.files[a.ID()] = a
	return nil
}
func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	a, ok := f.files[id]
	if !ok {
		return nil, errNotImplementedInFake
	}
	return a, nil
}
func (f *fakeAudioFileRepo) FindByScriptID(_ context.Context, scriptID string) (*domain.AudioFile, error) {
	for _, a := range f.files {
		if a.ScriptID() == scriptID {
			return a, nil
		}
	}
	return nil, pgx.ErrNoRows
}

type fakeUserRepo struct{ users map[string]*domain.User }

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{users: map[string]*domain.User{}} }

func (f *fakeUserRepo) Save(_ context.Context, u *domain.User) error {
	f.users[u.ID()] = u
	return nil
}
func (f *fakeUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return u, nil
}
func (f *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, u := range f.users {
		if u.Email().String() == email {
			return u, nil
		}
	}
	return nil, pgx.ErrNoRows
}

// fakeTokenIssuer skips real signing entirely -- "userID:role" is opaque
// enough for tests, which only need Issue/Verify to round-trip consistently,
// never real cryptographic guarantees.
type fakeTokenIssuer struct{}

func (fakeTokenIssuer) Issue(userID string, role domain.Role) (string, error) {
	return userID + ":" + role.String(), nil
}

func (fakeTokenIssuer) Verify(token string) (string, domain.Role, error) {
	userID, roleRaw, ok := strings.Cut(token, ":")
	if !ok {
		return "", "", errors.New("invalid fake token")
	}
	role, err := domain.NewRole(roleRaw)
	if err != nil {
		return "", "", err
	}
	return userID, role, nil
}

type fakePublisher struct{ published int }

func (f *fakePublisher) PublishTTSJob(_ context.Context, _, _, _, _, _ string) error {
	f.published++
	return nil
}

type fakeCache struct {
	data map[string]string
	sets int
}

func newFakeCache() *fakeCache { return &fakeCache{data: map[string]string{}} }

func (f *fakeCache) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.data[key] = value
	f.sets++
	return nil
}

type erroringCache struct{}

func (erroringCache) Get(_ context.Context, _ string) (string, bool, error) {
	return "", false, errors.New("redis unavailable")
}
func (erroringCache) Set(_ context.Context, _, _ string, _ time.Duration) error {
	return errors.New("redis unavailable")
}

func TestListPlaces(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

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
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, audioFileRepo, newFakeUserRepo(), publisher, fakeAudioStorage{}, newFakeCache(), tokens)

	token, _ := tokens.Issue("julie", domain.RoleUser)
	body := strings.NewReader(`{"voice_id":"voice-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/scripts/"+script.ID()+"/review", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if publisher.published != 1 {
		t.Fatalf("got %d published jobs, want 1", publisher.published)
	}

	reviewed, _ := scriptRepo.FindByID(context.Background(), script.ID())
	if reviewed.ReviewerID() != "julie" {
		t.Fatalf("got reviewerID %q, want %q (from the authenticated token, not a client-supplied field)", reviewed.ReviewerID(), "julie")
	}
}

func TestReviewScript_RequiresAuth(t *testing.T) {
	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, audioFileRepo, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{})

	body := strings.NewReader(`{"voice_id":"voice-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/scripts/"+script.ID()+"/review", body)
	req.Header.Set("Content-Type", "application/json")
	// Pas d'en-tête Authorization -- doit être rejeté avant même d'atteindre
	// le handler.
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401 (missing Authorization header)", rec.Code)
	}
}

func TestListPlaces_CachesOnSecondCall(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	cache := newFakeCache()
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, cache, fakeTokenIssuer{})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/places", nil)
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

func TestListPlaces_FailsOpenWhenCacheErrors(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakePublisher{}, fakeAudioStorage{}, erroringCache{}, fakeTokenIssuer{})

	req := httptest.NewRequest(http.MethodGet, "/places", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 — a cache error must never fail the request", rec.Code)
	}
}
