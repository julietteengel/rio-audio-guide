package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakeFeaturedPlaceRepo struct {
	places map[string]*domain.Place
}

func (f *fakeFeaturedPlaceRepo) Save(_ context.Context, _ *domain.Place) error { return nil }
func (f *fakeFeaturedPlaceRepo) FindByID(_ context.Context, id string) (*domain.Place, error) {
	p, ok := f.places[id]
	if !ok {
		// Matches the real Postgres-backed PlaceRepository's actual
		// behavior (a plain QueryRow/Scan miss) -- so this fake exercises
		// the same error CreateFeaturedItinerary sees in production,
		// not a generic stand-in.
		return nil, pgx.ErrNoRows
	}
	return p, nil
}
func (f *fakeFeaturedPlaceRepo) FindByName(_ context.Context, _ string) (*domain.Place, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeFeaturedPlaceRepo) FindActiveInBoundingBox(_ context.Context, _, _, _, _ float64) ([]*domain.Place, error) {
	return nil, errors.New("not implemented in fake")
}

func TestCreateFeaturedItinerary_BuildsStopsFromRealPlaces(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Paço Imperial")
	coords, _ := domain.NewCoordinates(-22.9035, -43.1755)
	place := domain.NewPlace(placeName, "historic_site", coords, "", "wikidata", "correct")

	placeRepo := &fakeFeaturedPlaceRepo{places: map[string]*domain.Place{place.ID(): place}}
	itineraryRepo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{}}

	it, err := CreateFeaturedItinerary(context.Background(), placeRepo, itineraryRepo, "admin-1", "Roteiro do Rio Colonial", []FeaturedStopInput{
		{PlaceID: place.ID(), TimeOnSiteMinutes: 20, WalkToNextMinutes: 5},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !it.IsFeatured() {
		t.Fatal("expected the created itinerary to be marked featured")
	}
	stops := it.Stops()
	if len(stops) != 1 {
		t.Fatalf("got %d stops, want 1", len(stops))
	}
	if stops[0].Label() != "Paço Imperial" {
		t.Fatalf("got label %q, want the real place's own name %q", stops[0].Label(), "Paço Imperial")
	}
	if itineraryRepo.saved == nil {
		t.Fatal("expected the itinerary to have been saved")
	}
}

func TestCreateFeaturedItinerary_UnknownPlaceIDFails(t *testing.T) {
	placeRepo := &fakeFeaturedPlaceRepo{places: map[string]*domain.Place{}}
	itineraryRepo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{}}

	_, err := CreateFeaturedItinerary(context.Background(), placeRepo, itineraryRepo, "admin-1", "Roteiro do Rio Colonial", []FeaturedStopInput{
		{PlaceID: "nonexistent", TimeOnSiteMinutes: 20, WalkToNextMinutes: 5},
	})
	// A place that doesn't exist at all propagates the repository's own raw
	// error (pgx.ErrNoRows here) rather than being folded into
	// ErrFeaturedStopPlaceNotFound -- that sentinel is reserved for a place
	// that exists but is removed (see TestCreateFeaturedItinerary_RemovedPlaceFails).
	// The HTTP handler is what maps both cases to the same 422 for the admin.
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("got error %v, want it to wrap pgx.ErrNoRows", err)
	}
}

func TestCreateFeaturedItinerary_RemovedPlaceFails(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Paço Imperial")
	coords, _ := domain.NewCoordinates(-22.9035, -43.1755)
	place := domain.NewPlace(placeName, "historic_site", coords, "", "wikidata", "correct")
	if err := place.Remove("duplicate"); err != nil {
		t.Fatalf("remove fixture place: %v", err)
	}
	placeRepo := &fakeFeaturedPlaceRepo{places: map[string]*domain.Place{place.ID(): place}}
	itineraryRepo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{}}

	_, err := CreateFeaturedItinerary(context.Background(), placeRepo, itineraryRepo, "admin-1", "Roteiro do Rio Colonial", []FeaturedStopInput{
		{PlaceID: place.ID(), TimeOnSiteMinutes: 20, WalkToNextMinutes: 5},
	})
	if !errors.Is(err, ErrFeaturedStopPlaceNotFound) {
		t.Fatalf("got error %v, want ErrFeaturedStopPlaceNotFound for a removed place", err)
	}
}

func TestCreateFeaturedItinerary_SaveFailureWrapsErrSaveFailed(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Paço Imperial")
	coords, _ := domain.NewCoordinates(-22.9035, -43.1755)
	place := domain.NewPlace(placeName, "historic_site", coords, "", "wikidata", "correct")
	placeRepo := &fakeFeaturedPlaceRepo{places: map[string]*domain.Place{place.ID(): place}}
	itineraryRepo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{}, saveErr: errors.New("connection reset")}

	_, err := CreateFeaturedItinerary(context.Background(), placeRepo, itineraryRepo, "admin-1", "Roteiro do Rio Colonial", []FeaturedStopInput{
		{PlaceID: place.ID(), TimeOnSiteMinutes: 20, WalkToNextMinutes: 5},
	})
	if !errors.Is(err, ErrSaveFailed) {
		t.Fatalf("got error %v, want it to wrap ErrSaveFailed", err)
	}
}

func TestCreateFeaturedItinerary_EmptyTitleFails(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Paço Imperial")
	coords, _ := domain.NewCoordinates(-22.9035, -43.1755)
	place := domain.NewPlace(placeName, "historic_site", coords, "", "wikidata", "correct")
	placeRepo := &fakeFeaturedPlaceRepo{places: map[string]*domain.Place{place.ID(): place}}
	itineraryRepo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{}}

	_, err := CreateFeaturedItinerary(context.Background(), placeRepo, itineraryRepo, "admin-1", "", []FeaturedStopInput{
		{PlaceID: place.ID(), TimeOnSiteMinutes: 20, WalkToNextMinutes: 5},
	})
	if !errors.Is(err, domain.ErrItineraryTitleRequired) {
		t.Fatalf("got error %v, want domain.ErrItineraryTitleRequired", err)
	}
}

func TestListFeaturedItineraries_ReturnsOnlyFeatured(t *testing.T) {
	title, _ := domain.NewItineraryTitle("Roteiro do Rio Colonial")
	stop, _ := domain.NewPlaceStop("place-1", "Paço Imperial", 20, 5)
	featured, _ := domain.NewItinerary("admin-1", title, []domain.ItineraryStop{stop})
	featured.MarkFeatured()
	notFeatured, _ := domain.NewItinerary("user-1", title, []domain.ItineraryStop{stop})

	repo := &fakeItineraryRepo{byID: map[string]*domain.Itinerary{
		featured.ID():    featured,
		notFeatured.ID(): notFeatured,
	}}

	got, err := ListFeaturedItineraries(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID() != featured.ID() {
		t.Fatalf("got %d itineraries, want exactly the 1 featured one", len(got))
	}
}

// ports import is used by the fakeFeaturedPlaceRepo type assertion the Go
// compiler performs implicitly when it's passed where a ports.PlaceRepository
// is expected -- keeping the import here documents that requirement even
// though no symbol from it is referenced by name in this file.
var _ ports.PlaceRepository = (*fakeFeaturedPlaceRepo)(nil)
