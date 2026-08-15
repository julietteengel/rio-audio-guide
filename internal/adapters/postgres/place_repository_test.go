//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

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
