//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

// testPool reads TEST_DATABASE_URL so the port is overridable — useful when a
// local, non-Docker Postgres is already listening on 5432 (common on dev
// machines), and required for CI where the service container's port varies.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5432/postgres"
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPlaceRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	name, err := domain.NewPlaceName("Cristo Redentor")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	coords, err := domain.NewCoordinates(-22.9519, -43.2105)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	qid, err := domain.NewWikidataQID("Q1963380")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	place := domain.NewPlace(name, "monument", coords, qid, "wikidata", "rich")

	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindByID(ctx, place.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Name() != place.Name() || found.WikidataQID() != place.WikidataQID() {
		t.Fatalf("got %+v, want %+v", found, place)
	}

	if err := place.Remove("test cleanup"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save after remove: %v", err)
	}
	found, err = repo.FindByID(ctx, place.ID())
	if err != nil {
		t.Fatalf("find by id after remove: %v", err)
	}
	if found.Status() != domain.PlaceStatusRemoved {
		t.Fatalf("got status %v, want removed", found.Status())
	}
}

func TestPlaceRepository_FindActiveInBoundingBox(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	name, _ := domain.NewPlaceName("Escadaria Selarón")
	coords, _ := domain.NewCoordinates(-22.9147, -43.1806)
	place := domain.NewPlace(name, "monument", coords, "", "overture", "correct")
	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindActiveInBoundingBox(ctx, -23.0, -43.3, -22.8, -43.0)
	if err != nil {
		t.Fatalf("find active in bounding box: %v", err)
	}
	var seen bool
	for _, p := range found {
		if p.ID() == place.ID() {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("expected place %q in bounding box results, got %d results", place.ID(), len(found))
	}
}

// savePlaceFixture insère un lieu au nom unique. places.name n'a pas de
// contrainte d'unicité et les tests ne nettoient pas derrière eux : réutiliser
// un nom fixe ferait dépendre le résultat des exécutions précédentes.
func savePlaceFixture(t *testing.T, repo *PlaceRepository, rawName string) *domain.Place {
	t.Helper()
	name, err := domain.NewPlaceName(rawName)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	coords, err := domain.NewCoordinates(-22.9147, -43.1806)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	place := domain.NewPlace(name, "monument", coords, "", "overture", "correct")
	if err := repo.Save(context.Background(), place); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	return place
}

func TestPlaceRepository_FindByName(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	uniqueName := fmt.Sprintf("Theatro Municipal %d", time.Now().UnixNano())
	place := savePlaceFixture(t, repo, uniqueName)

	found, err := repo.FindByName(ctx, uniqueName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != place.ID() {
		t.Fatalf("got ID %q, want %q", found.ID(), place.ID())
	}

	if _, err := repo.FindByName(ctx, "does not exist"); err == nil {
		t.Fatal("expected an error for an unknown name, got nil")
	}
}

// Un lieu retiré ne doit pas ressortir de FindByName : sinon cmd/import le
// prendrait pour un lieu existant et rattacherait des scripts à un lieu que le
// domaine considère comme supprimé.
func TestPlaceRepository_FindByName_IgnoresRemovedPlaces(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	uniqueName := fmt.Sprintf("Lieu retiré %d", time.Now().UnixNano())
	place := savePlaceFixture(t, repo, uniqueName)
	if err := place.Remove("doublon"); err != nil {
		t.Fatalf("remove place: %v", err)
	}
	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save removed place: %v", err)
	}

	if _, err := repo.FindByName(ctx, uniqueName); err == nil {
		t.Fatal("expected an error for a removed place, got nil")
	}

	// Même nom, mais actif cette fois : c'est celui-là qu'on doit retrouver.
	active := savePlaceFixture(t, repo, uniqueName)
	found, err := repo.FindByName(ctx, uniqueName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != active.ID() {
		t.Fatalf("got ID %q, want the active place %q", found.ID(), active.ID())
	}
}
