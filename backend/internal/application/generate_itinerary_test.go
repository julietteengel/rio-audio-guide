package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakeItineraryRepo struct {
	saved    *domain.Itinerary
	byID     map[string]*domain.Itinerary
	byUserID map[string][]*domain.Itinerary
	saveErr  error
}

func (f *fakeItineraryRepo) Save(_ context.Context, itinerary *domain.Itinerary) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = itinerary
	return nil
}
func (f *fakeItineraryRepo) FindByID(_ context.Context, id string) (*domain.Itinerary, error) {
	it, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return it, nil
}
func (f *fakeItineraryRepo) FindByUserID(_ context.Context, userID string) ([]*domain.Itinerary, error) {
	return f.byUserID[userID], nil
}

type fakeGenerator struct {
	result ports.GeneratedItinerary
	err    error
}

func (f *fakeGenerator) Generate(_ context.Context, _ string, _ []ports.CandidatePlace) (ports.GeneratedItinerary, error) {
	return f.result, f.err
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

func TestGenerateItinerary_BuildsAndSavesADomainItinerary(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{result: ports.GeneratedItinerary{
		Title: "Art et rue à Santa Teresa",
		Stops: []ports.GeneratedStop{
			{PlaceID: place.ID(), Label: "Escadaria Selarón", TimeOnSiteMinutes: 10, WalkToNextMinutes: 5},
			{IsSuggestion: true, Label: "Pause déjeuner", WalkToNextMinutes: 5},
		},
	}}

	it, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if it.UserID() != "user-1" || it.Title().String() != "Art et rue à Santa Teresa" {
		t.Fatalf("got userID=%q title=%q", it.UserID(), it.Title())
	}
	if len(it.Stops()) != 2 {
		t.Fatalf("got %d stops, want 2", len(it.Stops()))
	}
	if repo.saved == nil || repo.saved.ID() != it.ID() {
		t.Fatal("expected the generated itinerary to have been saved")
	}
}

func TestGenerateItinerary_PropagatesGeneratorError(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{err: errors.New("claude: no candidates")}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if err == nil {
		t.Fatal("expected the generator's error to propagate")
	}
}

func TestGenerateItinerary_RejectsAnEmptyGeneratedItinerary(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{result: ports.GeneratedItinerary{Title: "Vide", Stops: nil}}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if err == nil {
		t.Fatal("expected an error for a zero-stop generated itinerary (domain.NewItinerary rejects it)")
	}
}

func TestGenerateItinerary_RejectsEmptyCandidateList(t *testing.T) {
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{result: ports.GeneratedItinerary{Title: "Vide"}}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", nil)
	if !errors.Is(err, ErrNoCandidatePlaces) {
		t.Fatalf("got %v, want ErrNoCandidatePlaces", err)
	}
}

func TestGenerateItinerary_RejectsAPlaceIDNotInCandidates(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{result: ports.GeneratedItinerary{
		Title: "Art et rue à Santa Teresa",
		Stops: []ports.GeneratedStop{{PlaceID: "not-a-real-place", Label: "Ghost", TimeOnSiteMinutes: 10, WalkToNextMinutes: 0}},
	}}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if err == nil {
		t.Fatal("expected an error for a place id absent from the candidate list")
	}
}

func TestGenerateItinerary_WrapsGeneratorError(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{}
	gen := &fakeGenerator{err: errors.New("claude: rate limited")}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if !errors.Is(err, ErrGenerationFailed) {
		t.Fatalf("got %v, want an error wrapping ErrGenerationFailed", err)
	}
}

func TestGenerateItinerary_WrapsSaveError(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	repo := &fakeItineraryRepo{saveErr: errors.New("connection reset")}
	gen := &fakeGenerator{result: ports.GeneratedItinerary{
		Title: "Art et rue à Santa Teresa",
		Stops: []ports.GeneratedStop{{PlaceID: place.ID(), Label: "Escadaria Selarón", TimeOnSiteMinutes: 10, WalkToNextMinutes: 0}},
	}}

	_, err := GenerateItinerary(context.Background(), gen, repo, "user-1", "1h à Santa Teresa", []*domain.Place{place})
	if !errors.Is(err, ErrSaveFailed) {
		t.Fatalf("got %v, want an error wrapping ErrSaveFailed", err)
	}
}

func TestListItineraries(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	title, _ := domain.NewItineraryTitle("Art et rue à Santa Teresa")
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 0)
	existing, err := domain.NewItinerary("user-1", title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	repo := &fakeItineraryRepo{byUserID: map[string][]*domain.Itinerary{"user-1": {existing}}}

	got, err := ListItineraries(context.Background(), repo, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID() != existing.ID() {
		t.Fatalf("got %d itineraries, want exactly the fixture one", len(got))
	}
}

func TestGetItinerary(t *testing.T) {
	place := testPlace(t, "Escadaria Selarón", -22.9147, -43.1806)
	title, _ := domain.NewItineraryTitle("Art et rue à Santa Teresa")
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 0)
	existing, err := domain.NewItinerary("user-1", title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	repo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{existing.ID(): existing}}

	got, err := GetItinerary(context.Background(), repo, existing.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID() != existing.ID() {
		t.Fatalf("got id %q, want %q", got.ID(), existing.ID())
	}
}
