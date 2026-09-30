package application

import (
	"context"
	"errors"
	"testing"

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
		return nil, errors.New("not found")
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
	if !errors.Is(err, ErrFeaturedStopPlaceNotFound) {
		t.Fatalf("got error %v, want ErrFeaturedStopPlaceNotFound", err)
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
