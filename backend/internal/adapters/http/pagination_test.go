package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func serverWithPlaces(places []*domain.Place) *Server {
	return NewServer(&fakePlaceRepo{places: places}, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, newFakeUserRepo(), &fakeItineraryRepoHTTP{},
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)
}

func serverWithNPlaces(t *testing.T, n int) *Server {
	t.Helper()
	places := make([]*domain.Place, 0, n)
	for i := 0; i < n; i++ {
		name, _ := domain.NewPlaceName(fmt.Sprintf("Place %03d", i))
		coords, _ := domain.NewCoordinates(-22.9, -43.2)
		places = append(places, domain.NewPlace(name, "monument", coords, "", "wikidata", "rich"))
	}
	return serverWithPlaces(places)
}

func getPlaces(t *testing.T, s *Server, query string) (int, []placeResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/places"+query, nil)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	var got []placeResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("bad body: %v: %s", err, rec.Body.String())
		}
	}
	return rec.Code, got
}

func TestListPlaces_WithoutParamsReturnsEverythingUnderTheCap(t *testing.T) {
	code, got := getPlaces(t, serverWithNPlaces(t, 30), "")
	if code != http.StatusOK || len(got) != 30 {
		t.Fatalf("got %d places (status %d), want all 30", len(got), code)
	}
}

func TestListPlaces_DefaultCapStopsAnUnboundedResponse(t *testing.T) {
	_, got := getPlaces(t, serverWithNPlaces(t, maxPlacesPageSize+25), "")
	if len(got) != maxPlacesPageSize {
		t.Fatalf("got %d places, want the default cap of %d", len(got), maxPlacesPageSize)
	}
}

func TestListPlaces_LimitAndOffsetSelectAWindow(t *testing.T) {
	s := serverWithNPlaces(t, 30)
	_, all := getPlaces(t, s, "")
	_, page := getPlaces(t, s, "?limit=10&offset=5")
	if len(page) != 10 {
		t.Fatalf("got %d places, want 10", len(page))
	}
	if page[0].ID != all[5].ID || page[9].ID != all[14].ID {
		t.Fatalf("window does not match items 5..14 of the full list")
	}
}

func TestListPlaces_OffsetPastTheEndIsAnEmptyList(t *testing.T) {
	code, got := getPlaces(t, serverWithNPlaces(t, 5), "?offset=50")
	if code != http.StatusOK || len(got) != 0 {
		t.Fatalf("got %d places (status %d), want an empty 200", len(got), code)
	}
}

func TestListPlaces_RejectsBadPaginationParams(t *testing.T) {
	s := serverWithNPlaces(t, 5)
	for _, q := range []string{"?limit=0", "?limit=-1", "?limit=abc", "?offset=-3", "?limit=100000"} {
		if code, _ := getPlaces(t, s, q); code != http.StatusBadRequest {
			t.Errorf("%s: got status %d, want 400", q, code)
		}
	}
}

func TestListPlaces_DifferentPagesAreNotServedFromOneCacheEntry(t *testing.T) {
	s := serverWithNPlaces(t, 30)
	_, first := getPlaces(t, s, "?limit=5&offset=0")
	_, second := getPlaces(t, s, "?limit=5&offset=5")
	if first[0].ID == second[0].ID {
		t.Fatal("page 2 returned page 1's cached body")
	}
}

func serverWithNItineraries(t *testing.T, n int) (*Server, string) {
	t.Helper()
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 0)
	var mine []*domain.Itinerary
	for i := 0; i < n; i++ {
		title, _ := domain.NewItineraryTitle(fmt.Sprintf("Roteiro %03d", i))
		it, err := domain.NewItinerary("test-user-id", title, []domain.ItineraryStop{stop})
		if err != nil {
			t.Fatalf("build fixture: %v", err)
		}
		mine = append(mine, it)
	}
	tokens := fakeTokenIssuer{}
	s := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(),
		&fakeItineraryRepoHTTP{byUserID: map[string][]*domain.Itinerary{"test-user-id": mine}},
		&fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)
	token, _ := tokens.Issue("test-user-id", domain.RoleUser)
	return s, token
}

func getItineraries(s *Server, token, query string) (int, []itineraryResponse) {
	req := httptest.NewRequest(http.MethodGet, "/itineraries"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.echo.ServeHTTP(rec, req)
	var got []itineraryResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return rec.Code, got
}

func TestListItineraries_DefaultCapAndWindow(t *testing.T) {
	s, token := serverWithNItineraries(t, maxItinerariesPerPage+10)
	_, capped := getItineraries(s, token, "")
	if len(capped) != maxItinerariesPerPage {
		t.Fatalf("got %d, want default cap %d", len(capped), maxItinerariesPerPage)
	}
	_, window := getItineraries(s, token, "?limit=5&offset=98")
	if len(window) != 5 || window[0].ID != capped[98].ID {
		t.Fatalf("window wrong: %d items", len(window))
	}
}

func TestListItineraries_RejectsBadPagination(t *testing.T) {
	s, token := serverWithNItineraries(t, 3)
	if code, _ := getItineraries(s, token, "?limit=0"); code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}
