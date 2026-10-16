// internal/adapters/postgres/itinerary_repository_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

// saveUserFixture crée un utilisateur de test avec une adresse email unique
// (passée par l'appelant, en général dérivée d'un ID de fixture déjà créé) --
// comme savePlaceFixture, users.email n'a pas de contrainte d'unicité testée
// ici et les tests ne nettoient pas derrière eux.
func saveUserFixture(t *testing.T, pool *pgxpool.Pool, emailLocalPart string) *domain.User {
	t.Helper()
	email, err := domain.NewEmail(emailLocalPart + "@example.com")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	passwordHash, err := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := NewUserRepository(pool).Save(context.Background(), user); err != nil {
		t.Fatalf("save user fixture: %v", err)
	}
	return user
}

func TestItineraryRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	place := savePlaceFixture(t, NewPlaceRepository(pool), "Escadaria Selarón")
	user := saveUserFixture(t, pool, "julie+"+place.ID())

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

	place := savePlaceFixture(t, NewPlaceRepository(pool), "Parque das Ruínas")
	user := saveUserFixture(t, pool, "julie+"+place.ID())

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

func TestItineraryRepository_SaveAndFindFeatured(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	place := savePlaceFixture(t, NewPlaceRepository(pool), "Paço Imperial")
	featuredUser := saveUserFixture(t, pool, "julie+featured-"+place.ID())

	featuredTitle, _ := domain.NewItineraryTitle("Roteiro do Rio Colonial")
	featuredStop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 20, 5)
	featured, err := domain.NewItinerary(featuredUser.ID(), featuredTitle, []domain.ItineraryStop{featuredStop})
	if err != nil {
		t.Fatalf("build featured fixture itinerary: %v", err)
	}
	featured.MarkFeatured()

	repo := NewItineraryRepository(pool)
	if err := repo.Save(ctx, featured); err != nil {
		t.Fatalf("save featured: %v", err)
	}

	notFeaturedUser := saveUserFixture(t, pool, "julie+notfeatured-"+place.ID())
	notFeaturedTitle, _ := domain.NewItineraryTitle("Une après-midi à Santa Teresa")
	notFeaturedStop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 20, 5)
	notFeatured, err := domain.NewItinerary(notFeaturedUser.ID(), notFeaturedTitle, []domain.ItineraryStop{notFeaturedStop})
	if err != nil {
		t.Fatalf("build non-featured fixture itinerary: %v", err)
	}
	if err := repo.Save(ctx, notFeatured); err != nil {
		t.Fatalf("save non-featured: %v", err)
	}

	found, err := repo.FindFeatured(ctx)
	if err != nil {
		t.Fatalf("find featured: %v", err)
	}

	var sawFeatured, sawNotFeatured bool
	for _, it := range found {
		if it.ID() == featured.ID() {
			sawFeatured = true
			if !it.IsFeatured() {
				t.Fatal("expected the found featured itinerary to have IsFeatured() true")
			}
		}
		if it.ID() == notFeatured.ID() {
			sawNotFeatured = true
		}
	}
	if !sawFeatured {
		t.Fatal("expected FindFeatured to include the itinerary marked featured")
	}
	if sawNotFeatured {
		t.Fatal("expected FindFeatured to exclude the itinerary NOT marked featured")
	}
}

func TestItineraryRepository_IsFeaturedRoundTrips(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	place := savePlaceFixture(t, NewPlaceRepository(pool), "Confeitaria Colombo")
	user := saveUserFixture(t, pool, "julie+"+place.ID())

	title, _ := domain.NewItineraryTitle("Roteiro do Rio Colonial")
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 20, 5)
	it, err := domain.NewItinerary(user.ID(), title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture itinerary: %v", err)
	}
	it.MarkFeatured()

	repo := NewItineraryRepository(pool)
	if err := repo.Save(ctx, it); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindByID(ctx, it.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if !found.IsFeatured() {
		t.Fatal("expected IsFeatured() true after reload from Postgres")
	}
}
