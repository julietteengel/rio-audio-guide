// internal/adapters/postgres/itinerary_repository_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestItineraryRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Escadaria Selarón")
	coords, _ := domain.NewCoordinates(-22.9147, -43.1806)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	if err := NewPlaceRepository(pool).Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	email, _ := domain.NewEmail("julie+" + place.ID() + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := NewUserRepository(pool).Save(ctx, user); err != nil {
		t.Fatalf("save user fixture: %v", err)
	}

	title, _ := domain.NewItineraryTitle("Art et rue à Santa Teresa")
	placeStop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 10, 8)
	lunchStop, _ := domain.NewSuggestionStop("Pause déjeuner", 5)
	itinerary, err := domain.NewItinerary(user.ID(), title, []domain.ItineraryStop{placeStop, lunchStop})
	if err != nil {
		t.Fatalf("build fixture itinerary: %v", err)
	}

	repo := NewItineraryRepository(pool)
	if err := repo.Save(ctx, itinerary); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.FindByID(ctx, itinerary.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if got.UserID() != user.ID() || got.Title() != title {
		t.Fatalf("got userID=%q title=%q, want userID=%q title=%q", got.UserID(), got.Title(), user.ID(), title)
	}
	if len(got.Stops()) != 2 {
		t.Fatalf("got %d stops, want 2", len(got.Stops()))
	}
	// Order must survive the round trip -- the UI renders stops as a
	// numbered sequence, so a shuffled read-back would silently misorder
	// an itinerary that was generated correctly.
	if got.Stops()[0].Kind() != domain.ItineraryStopKindPlace || got.Stops()[0].PlaceID() != place.ID() {
		t.Fatalf("stop 0 should be the place stop, got kind=%q placeID=%q", got.Stops()[0].Kind(), got.Stops()[0].PlaceID())
	}
	if got.Stops()[1].Kind() != domain.ItineraryStopKindSuggestion || got.Stops()[1].Label() != "Pause déjeuner" {
		t.Fatalf("stop 1 should be the suggestion stop, got kind=%q label=%q", got.Stops()[1].Kind(), got.Stops()[1].Label())
	}
}

func TestItineraryRepository_FindByUserID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Parque das Ruínas")
	coords, _ := domain.NewCoordinates(-22.9207, -43.1876)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	if err := NewPlaceRepository(pool).Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}
	email, _ := domain.NewEmail("julie+" + place.ID() + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := NewUserRepository(pool).Save(ctx, user); err != nil {
		t.Fatalf("save user fixture: %v", err)
	}

	title, _ := domain.NewItineraryTitle("Matinée coloniale au Centro")
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 20, 0)
	itinerary, err := domain.NewItinerary(user.ID(), title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture itinerary: %v", err)
	}
	repo := NewItineraryRepository(pool)
	if err := repo.Save(ctx, itinerary); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.FindByUserID(ctx, user.ID())
	if err != nil {
		t.Fatalf("find by user id: %v", err)
	}
	if len(got) != 1 || got[0].ID() != itinerary.ID() {
		t.Fatalf("got %d itineraries, want exactly the one just saved", len(got))
	}
}
