package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakeItineraryRepoHTTP struct {
	byUserID map[string][]*domain.Itinerary
	byID     map[string]*domain.Itinerary
	saved    *domain.Itinerary
}

func (f *fakeItineraryRepoHTTP) Save(_ context.Context, it *domain.Itinerary) error {
	f.saved = it
	return nil
}
func (f *fakeItineraryRepoHTTP) FindByID(_ context.Context, id string) (*domain.Itinerary, error) {
	it, ok := f.byID[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return it, nil
}
func (f *fakeItineraryRepoHTTP) FindByUserID(_ context.Context, userID string) ([]*domain.Itinerary, error) {
	return f.byUserID[userID], nil
}

type fakeGeneratorHTTP struct{ result ports.GeneratedItinerary }

func (f *fakeGeneratorHTTP) Generate(_ context.Context, _ string, _ []ports.CandidatePlace) (ports.GeneratedItinerary, error) {
	return f.result, nil
}

func testPlace(t *testing.T, name string, lat, lon float64) *domain.Place {
	t.Helper()
	placeName, err := domain.NewPlaceName(name)
	if err != nil {
		t.Fatalf("build fixture place name: %v", err)
	}
	coords, err := domain.NewCoordinates(lat, lon)
	if err != nil {
		t.Fatalf("build fixture coordinates: %v", err)
	}
	return domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
}

func TestCreateItinerary_RequiresAuth(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	body, _ := json.Marshal(map[string]string{"request": "1h à Santa Teresa"})
	req := httptest.NewRequest(http.MethodPost, "/itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401 without an Authorization header", rec.Code)
	}
}

func TestCreateItinerary_Success(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	itineraryRepo := &fakeItineraryRepoHTTP{}
	generator := &fakeGeneratorHTTP{result: ports.GeneratedItinerary{
		Title: "Art et rue à Santa Teresa",
		Stops: []ports.GeneratedStop{{PlaceID: place.ID(), Label: place.Name().String(), TimeOnSiteMinutes: 10, WalkToNextMinutes: 0}},
	}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, generator, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"request": "1h à Santa Teresa"})
	req := httptest.NewRequest(http.MethodPost, "/itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got status %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if itineraryRepo.saved == nil {
		t.Fatal("expected the generated itinerary to have been saved")
	}
}

func TestCreateItinerary_ResponseIncludesStopKinds(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	itineraryRepo := &fakeItineraryRepoHTTP{}
	generator := &fakeGeneratorHTTP{result: ports.GeneratedItinerary{
		Title: "Art et rue à Santa Teresa",
		Stops: []ports.GeneratedStop{
			{PlaceID: place.ID(), Label: place.Name().String(), TimeOnSiteMinutes: 10, WalkToNextMinutes: 5},
			{IsSuggestion: true, Label: "Pause déjeuner", WalkToNextMinutes: 0},
		},
	}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, generator, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	body, _ := json.Marshal(map[string]string{"request": "1h à Santa Teresa"})
	req := httptest.NewRequest(http.MethodPost, "/itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got status %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var resp itineraryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Stops) != 2 {
		t.Fatalf("got %d stops, want 2", len(resp.Stops))
	}
	if resp.Stops[0].Kind != "place" || resp.Stops[0].PlaceID == "" {
		t.Fatalf("stop 0: got kind=%q place_id=%q, want kind=place with a place_id", resp.Stops[0].Kind, resp.Stops[0].PlaceID)
	}
	if resp.Stops[1].Kind != "suggestion" || resp.Stops[1].PlaceID != "" {
		t.Fatalf("stop 1: got kind=%q place_id=%q, want kind=suggestion with no place_id", resp.Stops[1].Kind, resp.Stops[1].PlaceID)
	}
}

func TestListItineraries_Success(t *testing.T) {
	title, _ := domain.NewItineraryTitle("Art et rue à Santa Teresa")
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 0)
	existing, err := domain.NewItinerary("test-user-id", title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	itineraryRepo := &fakeItineraryRepoHTTP{byUserID: map[string][]*domain.Itinerary{"test-user-id": {existing}}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/itineraries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestGetItinerary_NotFoundForAnotherUser(t *testing.T) {
	title, _ := domain.NewItineraryTitle("Art et rue à Santa Teresa")
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 0)
	existing, err := domain.NewItinerary("someone-else", title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{existing.ID(): existing}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/itineraries/"+existing.ID(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404 for another user's itinerary: %s", rec.Code, rec.Body.String())
	}
}
