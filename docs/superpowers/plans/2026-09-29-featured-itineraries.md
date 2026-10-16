# Featured Itineraries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin publish a hand-curated, publicly-viewable itinerary (e.g. "Roteiro do Rio
Colonial") that any user — including someone without an account — can discover and follow, distinct
from the personal, LLM-generated itineraries the app already supports.

**Architecture:** Reuses the existing `Itinerary` domain entity and stop model as-is, adding a single
`isFeatured bool` flag rather than introducing a new entity. A new admin-only creation route builds
stops directly from given place IDs (no LLM call); a new public, unauthenticated route lists every
featured itinerary. Mobile gets a new "Discover" section on the map screen and a read-only detail
screen that never depends on a login token.

**Tech Stack:** Go (hexagonal/DDD backend), Postgres, React Native/Expo mobile — no new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-29-featured-itineraries-design.md`

## Global Constraints

- `MarkFeatured()` is one-directional — no un-feature route in this plan (direct-database operation if
  ever needed, per the spec's explicit scope decision).
- `GET /featured-itineraries` must have **zero** auth middleware — not just tolerant of a missing/bad
  token, genuinely unauthenticated, same posture as `GET /places`.
- `POST /featured-itineraries` is **admin-only** (`auth, adminOnly` — the same `requireRole(domain.RoleAdmin)`
  middleware already used for `POST /scripts/:id/review`).
- A featured itinerary's stop labels are always the real place's own name (via `placeRepo.FindByID`),
  never a caller-supplied label — no second, possibly-inconsistent copy of a place's display name.
- The mobile `FeaturedItineraryDetail` screen must render correctly for a logged-out visitor — no
  `token`/`useAuth()` dependency anywhere in it.

---

### Task 1: Domain & Ports (Itinerary.isFeatured, MarkFeatured, ItineraryRepository.FindFeatured)

**Files:**
- Modify: `backend/internal/domain/itinerary.go`
- Modify: `backend/internal/domain/itinerary_test.go`
- Modify: `backend/internal/ports/itinerary_repository.go`

**Interfaces:**
- Produces: `Itinerary.IsFeatured() bool`, `Itinerary.MarkFeatured()`, `ReconstructItinerary(id, userID
  string, title ItineraryTitle, stops []ItineraryStop, createdAt time.Time, isFeatured bool)
  *Itinerary` (signature change — gains a 6th parameter), `ports.ItineraryRepository.FindFeatured(ctx
  context.Context) ([]*domain.Itinerary, error)` (new interface method).
- This task's `ReconstructItinerary` signature change breaks `internal/adapters/postgres`'s one call
  site, and the new `FindFeatured` interface method breaks every fake implementing
  `ports.ItineraryRepository` (`internal/application`'s `fakeItineraryRepo`,
  `internal/adapters/http`'s `fakeItineraryRepoHTTP`) — **this is expected and deferred to Task 2**
  (Postgres) and **Task 3/Task 4** (application/HTTP) respectively, the same deferred-breakage pattern
  already used on this branch's prior features. Verify success for *this* task by building only
  `internal/domain` (Step 6 below).

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/domain/itinerary_test.go` (after the existing `TestItinerary_TotalMinutesAndPlaceCount`):

```go
func TestItinerary_MarkFeatured(t *testing.T) {
	title, _ := NewItineraryTitle("Roteiro do Rio Colonial")
	stop, _ := NewPlaceStop("place-1", "Paço Imperial", 20, 5)
	it, err := NewItinerary("admin-1", title, []ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	if it.IsFeatured() {
		t.Fatal("a freshly created itinerary must not start featured")
	}

	it.MarkFeatured()
	if !it.IsFeatured() {
		t.Fatal("expected IsFeatured() true after MarkFeatured()")
	}

	// Idempotent: calling it twice must not error or toggle back off.
	it.MarkFeatured()
	if !it.IsFeatured() {
		t.Fatal("expected IsFeatured() to remain true after a second MarkFeatured() call")
	}
}

func TestReconstructItinerary_PreservesIsFeatured(t *testing.T) {
	title, _ := NewItineraryTitle("Roteiro do Rio Colonial")
	stop := ReconstructPlaceStop("stop-1", "place-1", "Paço Imperial", 20, 5)

	featured := ReconstructItinerary("it-1", "admin-1", title, []ItineraryStop{stop}, time.Now(), true)
	if !featured.IsFeatured() {
		t.Fatal("expected IsFeatured() true when reconstructed with isFeatured=true")
	}

	notFeatured := ReconstructItinerary("it-2", "user-1", title, []ItineraryStop{stop}, time.Now(), false)
	if notFeatured.IsFeatured() {
		t.Fatal("expected IsFeatured() false when reconstructed with isFeatured=false")
	}
}
```

Add `"time"` to this file's imports (currently just `"testing"`):

```go
import (
	"testing"
	"time"
)
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/domain/... -run "TestItinerary_MarkFeatured|TestReconstructItinerary_PreservesIsFeatured" -v`
Expected: FAIL to compile — `IsFeatured`/`MarkFeatured` undefined, `ReconstructItinerary` called with
too many arguments.

- [ ] **Step 3: Add the `isFeatured` field, `MarkFeatured`, and `IsFeatured` to the `Itinerary` entity**

In `backend/internal/domain/itinerary.go`, update the `Itinerary` struct:

```go
type Itinerary struct {
	id         string
	userID     string
	title      ItineraryTitle
	stops      []ItineraryStop
	createdAt  time.Time
	isFeatured bool
}
```

`NewItinerary`'s body and signature are unchanged — a freshly constructed `Itinerary{}` literal already
zero-values `isFeatured` to `false`, matching the spec's "defaults false, flipped explicitly" design; no
line in `NewItinerary` needs editing.

Add, right after `PlaceCount()`:

```go
// MarkFeatured flips isFeatured to true -- one-directional (no
// UnmarkFeatured), no validation, no error return: unlike
// User.MarkEmailVerified there is no "deleted" concept on Itinerary to
// guard against. Idempotent by construction.
func (i *Itinerary) MarkFeatured() {
	i.isFeatured = true
}

func (i *Itinerary) IsFeatured() bool { return i.isFeatured }
```

- [ ] **Step 4: Update `ReconstructItinerary`**

Replace the function:

```go
// ReconstructItinerary rebâtit un Itinerary depuis des données déjà valides
// (des lignes Postgres) -- préserve l'ID et createdAt donnés, ne revalide rien.
func ReconstructItinerary(id, userID string, title ItineraryTitle, stops []ItineraryStop, createdAt time.Time, isFeatured bool) *Itinerary {
	return &Itinerary{id: id, userID: userID, title: title, stops: stops, createdAt: createdAt, isFeatured: isFeatured}
}
```

- [ ] **Step 5: Add `FindFeatured` to `ports.ItineraryRepository`**

In `backend/internal/ports/itinerary_repository.go`:

```go
// internal/ports/itinerary_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type ItineraryRepository interface {
	Save(ctx context.Context, itinerary *domain.Itinerary) error
	FindByID(ctx context.Context, id string) (*domain.Itinerary, error)
	FindByUserID(ctx context.Context, userID string) ([]*domain.Itinerary, error)
	// FindFeatured returns every itinerary with IsFeatured() true, most
	// recently created first (same ordering as FindByUserID) -- no
	// pagination yet, the founder curates these by hand and a few dozen at
	// most is the realistic scale for the foreseeable future.
	FindFeatured(ctx context.Context) ([]*domain.Itinerary, error)
}
```

- [ ] **Step 6: Run the tests to verify they pass, and confirm this package builds on its own**

Run: `cd backend && go build ./internal/domain/... && go test ./internal/domain/... -v`
Expected: clean build, all tests pass (the 2 new ones plus every pre-existing test in this package).

Do **not** run `go build ./...` yet as a pass/fail gate for this task — `internal/adapters/postgres`,
`internal/application`, and `internal/adapters/http` are expected to fail to compile until Tasks 2-4 fix
their own fakes/call sites (see this task's Interfaces note above).

- [ ] **Step 7: Commit**

```bash
git add internal/domain/itinerary.go internal/domain/itinerary_test.go internal/ports/itinerary_repository.go
git commit -m "domain: Itinerary.isFeatured, ports.ItineraryRepository.FindFeatured"
```

---

### Task 2: Postgres adapter (is_featured column, FindFeatured, integration tests)

**Files:**
- Modify: `backend/internal/adapters/postgres/schema.sql`
- Modify: `backend/internal/adapters/postgres/itinerary_repository.go`
- Modify: `backend/internal/adapters/postgres/itinerary_repository_test.go` (create if it doesn't exist
  yet — check first; if there's no existing itinerary integration test file, create it following this
  package's established `//go:build integration` + `testPool(t)` pattern, e.g.
  `internal/adapters/postgres/user_repository_test.go`)

**Interfaces:**
- Consumes: Task 1's `Itinerary.IsFeatured()`, `ReconstructItinerary`'s new 6-arg signature,
  `ports.ItineraryRepository.FindFeatured`.
- Produces: `(*ItineraryRepository).FindFeatured(ctx) ([]*domain.Itinerary, error)` — a real
  implementation of Task 1's new port method. Consumed by Task 3.
- This task fixes `internal/adapters/postgres`'s compilation (broken by Task 1's `ReconstructItinerary`
  signature change) but leaves `internal/application` and `internal/adapters/http` broken until Tasks
  3-4 fix their own fakes.

- [ ] **Step 1: Write the failing integration tests**

First check whether `backend/internal/adapters/postgres/itinerary_repository_test.go` already exists
(`ls internal/adapters/postgres/*itinerary*`). If it exists, append these tests to it, matching whatever
existing fixture-building helpers it already has. If it does not exist, create it:

```go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func testItineraryFixture(t *testing.T, userID, titleStr string) *domain.Itinerary {
	t.Helper()
	title, err := domain.NewItineraryTitle(titleStr)
	if err != nil {
		t.Fatalf("build fixture title: %v", err)
	}
	stop, err := domain.NewPlaceStop("place-fixture-1", "Paço Imperial", 20, 5)
	if err != nil {
		t.Fatalf("build fixture stop: %v", err)
	}
	it, err := domain.NewItinerary(userID, title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture itinerary: %v", err)
	}
	return it
}

func TestItineraryRepository_SaveAndFindFeatured(t *testing.T) {
	pool := testPool(t)
	repo := NewItineraryRepository(pool)
	ctx := context.Background()

	featured := testItineraryFixture(t, "admin-1", "Roteiro do Rio Colonial")
	featured.MarkFeatured()
	if err := repo.Save(ctx, featured); err != nil {
		t.Fatalf("save featured: %v", err)
	}

	notFeatured := testItineraryFixture(t, "user-1", "Une après-midi à Santa Teresa")
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
	repo := NewItineraryRepository(pool)
	ctx := context.Background()

	it := testItineraryFixture(t, "admin-1", "Roteiro do Rio Colonial")
	it.MarkFeatured()
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test -tags integration ./internal/adapters/postgres/... -run "TestItineraryRepository_SaveAndFindFeatured|TestItineraryRepository_IsFeaturedRoundTrips" -v`
Expected: FAIL to compile — `repo.FindFeatured` undefined, and/or `ReconstructItinerary` call site in
this package still on the old 5-arg signature.

- [ ] **Step 3: Add the column to `schema.sql`**

In `backend/internal/adapters/postgres/schema.sql`, the `itineraries` table currently ends with
`created_at TIMESTAMPTZ NOT NULL DEFAULT now()`. Add the new column:

```sql
CREATE TABLE itineraries (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    title       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_featured BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX itineraries_user_id_idx ON itineraries (user_id);
```

- [ ] **Step 4: Update `itinerary_repository.go`**

Update `upsertItinerarySQL`:

```go
const upsertItinerarySQL = `
	INSERT INTO itineraries (id, user_id, title, created_at, is_featured)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, is_featured = EXCLUDED.is_featured
`
```

Update `Save`'s exec call:

```go
	if _, err := tx.Exec(ctx, upsertItinerarySQL, itinerary.ID(), itinerary.UserID(), itinerary.Title().String(), itinerary.CreatedAt(), itinerary.IsFeatured()); err != nil {
		return err
	}
```

Update `selectItineraryByIDSQL` and `FindByID`:

```go
const selectItineraryByIDSQL = `SELECT id, user_id, title, created_at, is_featured FROM itineraries WHERE id = $1`

func (r *ItineraryRepository) FindByID(ctx context.Context, id string) (*domain.Itinerary, error) {
	var (
		itinID, userID, titleStr string
		createdAt                time.Time
		isFeatured                bool
	)
	row := r.pool.QueryRow(ctx, selectItineraryByIDSQL, id)
	if err := row.Scan(&itinID, &userID, &titleStr, &createdAt, &isFeatured); err != nil {
		return nil, err
	}
	stops, err := r.loadStops(ctx, itinID)
	if err != nil {
		return nil, err
	}
	title, err := domain.NewItineraryTitle(titleStr)
	if err != nil {
		return nil, err
	}
	return domain.ReconstructItinerary(itinID, userID, title, stops, createdAt, isFeatured), nil
}
```

Update `selectItinerariesByUserIDSQL` and `FindByUserID`:

```go
const selectItinerariesByUserIDSQL = `
	SELECT id, user_id, title, created_at, is_featured
	FROM itineraries
	WHERE user_id = $1
	ORDER BY created_at DESC
`

func (r *ItineraryRepository) FindByUserID(ctx context.Context, userID string) ([]*domain.Itinerary, error) {
	rows, err := r.pool.Query(ctx, selectItinerariesByUserIDSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itineraries []*domain.Itinerary
	for rows.Next() {
		var id, uid, titleStr string
		var createdAt time.Time
		var isFeatured bool
		if err := rows.Scan(&id, &uid, &titleStr, &createdAt, &isFeatured); err != nil {
			return nil, err
		}
		stops, err := r.loadStops(ctx, id)
		if err != nil {
			return nil, err
		}
		title, err := domain.NewItineraryTitle(titleStr)
		if err != nil {
			return nil, err
		}
		itineraries = append(itineraries, domain.ReconstructItinerary(id, uid, title, stops, createdAt, isFeatured))
	}
	return itineraries, rows.Err()
}
```

Add `FindFeatured`, right after `FindByUserID`:

```go
const selectFeaturedItinerariesSQL = `
	SELECT id, user_id, title, created_at, is_featured
	FROM itineraries
	WHERE is_featured = true
	ORDER BY created_at DESC
`

func (r *ItineraryRepository) FindFeatured(ctx context.Context) ([]*domain.Itinerary, error) {
	rows, err := r.pool.Query(ctx, selectFeaturedItinerariesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var itineraries []*domain.Itinerary
	for rows.Next() {
		var id, uid, titleStr string
		var createdAt time.Time
		var isFeatured bool
		if err := rows.Scan(&id, &uid, &titleStr, &createdAt, &isFeatured); err != nil {
			return nil, err
		}
		stops, err := r.loadStops(ctx, id)
		if err != nil {
			return nil, err
		}
		title, err := domain.NewItineraryTitle(titleStr)
		if err != nil {
			return nil, err
		}
		itineraries = append(itineraries, domain.ReconstructItinerary(id, uid, title, stops, createdAt, isFeatured))
	}
	return itineraries, rows.Err()
}
```

- [ ] **Step 5: Run the integration tests to verify they pass**

Requires a real Postgres reachable via `TEST_DATABASE_URL` (see `testPool` in this package), with
`schema.sql` already applied (re-apply after Step 3's edit if testing against an existing local
container: `cat internal/adapters/postgres/schema.sql | docker exec -i <your-postgres-container> psql -U
postgres -d postgres`, or `ALTER TABLE itineraries ADD COLUMN IF NOT EXISTS is_featured BOOLEAN NOT
NULL DEFAULT false;` against an already-populated database).

Run: `cd backend && go test -tags integration ./internal/adapters/postgres/... -v`
Expected: PASS, every test in the package (the new ones plus every pre-existing one).

- [ ] **Step 6: Confirm this package builds on its own**

Run: `cd backend && go build ./internal/adapters/postgres/...`
Expected: clean. Still do not run `go build ./...` as a gate — `internal/application` and
`internal/adapters/http` remain broken until Tasks 3-4.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/postgres/schema.sql internal/adapters/postgres/itinerary_repository.go internal/adapters/postgres/itinerary_repository_test.go
git commit -m "postgres: itineraries.is_featured column, FindFeatured"
```

---

### Task 3: Application layer (CreateFeaturedItinerary, ListFeaturedItineraries)

**Files:**
- Create: `backend/internal/application/create_featured_itinerary.go`
- Create: `backend/internal/application/create_featured_itinerary_test.go`
- Modify: `backend/internal/application/generate_itinerary_test.go` (fix `fakeItineraryRepo` to
  implement Task 1's new `FindFeatured` method — mechanical, no behavior change to existing tests)

**Interfaces:**
- Consumes: Task 1's `Itinerary.MarkFeatured()`, `ports.ItineraryRepository.FindFeatured`; existing
  `ports.PlaceRepository.FindByID`; existing `domain.NewItineraryTitle`, `domain.NewPlaceStop`,
  `domain.NewItinerary`.
- Produces: `application.CreateFeaturedItinerary(ctx, placeRepo ports.PlaceRepository, itineraryRepo
  ports.ItineraryRepository, adminUserID, titleStr string, stopInputs []FeaturedStopInput)
  (*domain.Itinerary, error)`, `application.FeaturedStopInput{PlaceID string, TimeOnSiteMinutes int,
  WalkToNextMinutes int}`, `application.ListFeaturedItineraries(ctx, repo ports.ItineraryRepository)
  ([]*domain.Itinerary, error)`, `application.ErrFeaturedStopPlaceNotFound`. Consumed by Task 4's HTTP
  handlers.
- This task fixes `internal/application`'s compilation (broken by Task 1's new `FindFeatured` port
  method) but leaves `internal/adapters/http` broken until Task 4 fixes its own fake.

- [ ] **Step 1: Write the failing tests**

First, fix the existing `fakeItineraryRepo` in `backend/internal/application/generate_itinerary_test.go`
so the whole package compiles again — add this method right after the existing `FindByUserID`:

```go
func (f *fakeItineraryRepo) FindFeatured(_ context.Context) ([]*domain.Itinerary, error) {
	var featured []*domain.Itinerary
	for _, it := range f.byID {
		if it.IsFeatured() {
			featured = append(featured, it)
		}
	}
	return featured, nil
}
```

(`fakeItineraryRepo`'s `byID` field already exists in this file; this reuses it rather than adding a new
backing field. If `f.byID` is `nil` when this runs — e.g. a test that only ever called `Save`, never
populated `byID` directly — ranging over a nil map is safe in Go and yields zero iterations, so no
nil-check is needed.)

Create `backend/internal/application/create_featured_itinerary_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/application/... -run "TestCreateFeaturedItinerary_|TestListFeaturedItineraries_" -v`
Expected: FAIL to compile — `CreateFeaturedItinerary`/`FeaturedStopInput`/`ListFeaturedItineraries`/
`ErrFeaturedStopPlaceNotFound` undefined.

- [ ] **Step 3: Write `create_featured_itinerary.go`**

```go
package application

import (
	"context"
	"errors"
	"fmt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrFeaturedStopPlaceNotFound is returned when a stop names a place_id that
// doesn't exist -- a featured itinerary is hand-curated by an admin, but the
// actual place lookup still goes through the real repository rather than
// trusting the given ID blindly, same "never trust an ID that merely looks
// real" posture GenerateItinerary already applies to LLM-generated stops.
var ErrFeaturedStopPlaceNotFound = errors.New("application: one of the featured itinerary's places was not found")

type FeaturedStopInput struct {
	PlaceID           string
	TimeOnSiteMinutes int
	WalkToNextMinutes int
}

// CreateFeaturedItinerary builds each stop's label from the place's own real
// name (via placeRepo), rather than accepting a label in the request -- the
// admin picks which places and in what order, not a second, possibly-
// inconsistent copy of each place's display name.
func CreateFeaturedItinerary(ctx context.Context, placeRepo ports.PlaceRepository, itineraryRepo ports.ItineraryRepository, adminUserID, titleStr string, stopInputs []FeaturedStopInput) (*domain.Itinerary, error) {
	title, err := domain.NewItineraryTitle(titleStr)
	if err != nil {
		return nil, err
	}

	stops := make([]domain.ItineraryStop, 0, len(stopInputs))
	for _, in := range stopInputs {
		place, err := placeRepo.FindByID(ctx, in.PlaceID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrFeaturedStopPlaceNotFound, in.PlaceID)
		}
		stop, err := domain.NewPlaceStop(place.ID(), place.Name().String(), in.TimeOnSiteMinutes, in.WalkToNextMinutes)
		if err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}

	itinerary, err := domain.NewItinerary(adminUserID, title, stops)
	if err != nil {
		return nil, err
	}
	itinerary.MarkFeatured()
	if err := itineraryRepo.Save(ctx, itinerary); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}
	return itinerary, nil
}

func ListFeaturedItineraries(ctx context.Context, repo ports.ItineraryRepository) ([]*domain.Itinerary, error) {
	return repo.FindFeatured(ctx)
}
```

(`ErrSaveFailed` already exists in `generate_itinerary.go`, same package — reused as-is, not redefined.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/application/... -run "TestCreateFeaturedItinerary_|TestListFeaturedItineraries_" -v`
Expected: PASS, all 4 new tests.

- [ ] **Step 5: Run the full application package test suite to confirm nothing else broke**

Run: `cd backend && go test ./internal/application/... -v`
Expected: PASS, every test in the package, including the existing `GenerateItinerary`/`ListItineraries`
tests (untouched by this task beyond the `fakeItineraryRepo.FindFeatured` stub added in Step 1).

- [ ] **Step 6: Commit**

```bash
git add internal/application/create_featured_itinerary.go internal/application/create_featured_itinerary_test.go internal/application/generate_itinerary_test.go
git commit -m "application: CreateFeaturedItinerary, ListFeaturedItineraries"
```

---

### Task 4: HTTP routes (POST /featured-itineraries admin-only, GET /featured-itineraries public)

**Files:**
- Modify: `backend/internal/adapters/http/itinerary_handler.go`
- Modify: `backend/internal/adapters/http/server.go`
- Modify: `backend/internal/adapters/http/itinerary_handler_test.go`

**Interfaces:**
- Consumes: Task 3's `application.CreateFeaturedItinerary`, `FeaturedStopInput`,
  `ListFeaturedItineraries`, `ErrFeaturedStopPlaceNotFound`.
- Produces: `POST /featured-itineraries` (admin-only, `{title, stops: [{place_id,
  time_on_site_minutes, walk_to_next_minutes}]}` → `201`/`422`/`403`/`500`), `GET
  /featured-itineraries` (public, no auth, → `200` always, empty array if none exist). Consumed by
  Task 5's mobile `ItinerariesRepository.ts`.
- This task fixes the last remaining compile break — after this task, `go build ./...` and `go test
  ./...` are green across the whole backend again.

- [ ] **Step 1: Write the failing HTTP handler tests**

First, fix the existing `fakeItineraryRepoHTTP` in `backend/internal/adapters/http/itinerary_handler_test.go`
so the whole package compiles again — add right after the existing `FindByUserID`:

```go
func (f *fakeItineraryRepoHTTP) FindFeatured(_ context.Context) ([]*domain.Itinerary, error) {
	var featured []*domain.Itinerary
	for _, it := range f.byID {
		if it.IsFeatured() {
			featured = append(featured, it)
		}
	}
	return featured, nil
}
```

Add these new tests to the same file:

```go
func TestCreateFeaturedItinerary_RequiresAdmin(t *testing.T) {
	place := testPlace(t, "Paço Imperial", -22.9035, -43.1755)
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("someone-else", domain.RoleUser)
	body, _ := json.Marshal(map[string]any{
		"title": "Roteiro do Rio Colonial",
		"stops": []map[string]any{{"place_id": place.ID(), "time_on_site_minutes": 20, "walk_to_next_minutes": 5}},
	})
	req := httptest.NewRequest(http.MethodPost, "/featured-itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 403 for a non-admin caller: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateFeaturedItinerary_Success(t *testing.T) {
	place := testPlace(t, "Paço Imperial", -22.9035, -43.1755)
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("admin-1", domain.RoleAdmin)
	body, _ := json.Marshal(map[string]any{
		"title": "Roteiro do Rio Colonial",
		"stops": []map[string]any{{"place_id": place.ID(), "time_on_site_minutes": 20, "walk_to_next_minutes": 5}},
	})
	req := httptest.NewRequest(http.MethodPost, "/featured-itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got status %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if itineraryRepo.saved == nil || !itineraryRepo.saved.IsFeatured() {
		t.Fatal("expected the saved itinerary to be marked featured")
	}
}

func TestCreateFeaturedItinerary_UnknownPlaceReturns422(t *testing.T) {
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{}}
	tokens := fakeTokenIssuer{}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), tokens, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	token, _ := tokens.Issue("admin-1", domain.RoleAdmin)
	body, _ := json.Marshal(map[string]any{
		"title": "Roteiro do Rio Colonial",
		"stops": []map[string]any{{"place_id": "nonexistent", "time_on_site_minutes": 20, "walk_to_next_minutes": 5}},
	})
	req := httptest.NewRequest(http.MethodPost, "/featured-itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got status %d, want 422: %s", rec.Code, rec.Body.String())
	}
}

func TestListFeaturedItineraries_NoAuthRequired(t *testing.T) {
	title, _ := domain.NewItineraryTitle("Roteiro do Rio Colonial")
	place := testPlace(t, "Paço Imperial", -22.9035, -43.1755)
	stop, _ := domain.NewPlaceStop(place.ID(), place.Name().String(), 20, 5)
	featured, err := domain.NewItinerary("admin-1", title, []domain.ItineraryStop{stop})
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	featured.MarkFeatured()
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{featured.ID(): featured}}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	// No Authorization header at all -- proves this route is genuinely
	// unauthenticated, not just tolerant of a bad token.
	req := httptest.NewRequest(http.MethodGet, "/featured-itineraries", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 with no Authorization header: %s", rec.Code, rec.Body.String())
	}
	var resp []itineraryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 || resp[0].ID != featured.ID() {
		t.Fatalf("got %d itineraries, want exactly the 1 featured one", len(resp))
	}
}

func TestListFeaturedItineraries_EmptyArrayWhenNoneExist(t *testing.T) {
	itineraryRepo := &fakeItineraryRepoHTTP{byID: map[string]*domain.Itinerary{}}
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{}, &fakePlaceAssistantHTTP{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/featured-itineraries", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp []itineraryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("got %d itineraries, want 0", len(resp))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/adapters/http/... -run "TestCreateFeaturedItinerary_|TestListFeaturedItineraries_" -v`
Expected: FAIL to compile — this package has been left broken since Task 1's `FindFeatured` addition to
the interface (deferred on purpose, see this task's Interfaces note). Step 1 above fixes
`fakeItineraryRepoHTTP`'s own signature, but the `/featured-itineraries` routes and handlers don't exist
yet — Step 3 below fixes that.

- [ ] **Step 3: Add the handlers to `itinerary_handler.go`**

Add, at the end of the file:

```go
type createFeaturedItineraryStopRequest struct {
	PlaceID           string `json:"place_id"`
	TimeOnSiteMinutes int    `json:"time_on_site_minutes"`
	WalkToNextMinutes int    `json:"walk_to_next_minutes"`
}

type createFeaturedItineraryRequest struct {
	Title string                                `json:"title"`
	Stops []createFeaturedItineraryStopRequest `json:"stops"`
}

func (s *Server) createFeaturedItinerary(c echo.Context) error {
	var req createFeaturedItineraryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}

	stopInputs := make([]application.FeaturedStopInput, len(req.Stops))
	for i, s := range req.Stops {
		stopInputs[i] = application.FeaturedStopInput{
			PlaceID:           s.PlaceID,
			TimeOnSiteMinutes: s.TimeOnSiteMinutes,
			WalkToNextMinutes: s.WalkToNextMinutes,
		}
	}

	itinerary, err := application.CreateFeaturedItinerary(c.Request().Context(), s.placeRepo, s.itineraryRepo, contextUserID(c), req.Title, stopInputs)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrSaveFailed):
			log.Printf("createFeaturedItinerary: save failed: %v", err)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "could not save the featured itinerary"})
		default:
			return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
		}
	}
	return c.JSON(http.StatusCreated, toItineraryResponse(itinerary))
}

func (s *Server) listFeaturedItineraries(c echo.Context) error {
	itineraries, err := application.ListFeaturedItineraries(c.Request().Context(), s.itineraryRepo)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	responses := make([]itineraryResponse, len(itineraries))
	for i, it := range itineraries {
		responses[i] = toItineraryResponse(it)
	}
	return c.JSON(http.StatusOK, responses)
}
```

- [ ] **Step 4: Register the two new routes**

In `backend/internal/adapters/http/server.go`, add right after the existing itinerary route
registrations (`s.echo.GET("/itineraries/:id", s.getItinerary, auth)`):

```go
	s.echo.POST("/itineraries", s.createItinerary, auth)
	s.echo.GET("/itineraries", s.listItineraries, auth)
	s.echo.GET("/itineraries/:id", s.getItinerary, auth)
	s.echo.POST("/featured-itineraries", s.createFeaturedItinerary, auth, adminOnly)
	s.echo.GET("/featured-itineraries", s.listFeaturedItineraries)
```

(`GET /featured-itineraries` deliberately has no middleware at all — the third and fourth positional
arguments other routes pass, like `auth` or `auth, adminOnly`, are simply omitted here.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd backend && go test ./internal/adapters/http/... -v`
Expected: PASS, every test in the package (the new ones plus every pre-existing one — no regressions).

- [ ] **Step 6: Build the whole backend to confirm everything compiles together**

Run: `cd backend && go build ./...`
Expected: no output, exit 0. This is the first point since Task 1 where the whole backend is expected to
compile clean again.

- [ ] **Step 7: Run the full backend test suite**

Run: `cd backend && go test ./...`
Expected: PASS across every package.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/http/itinerary_handler.go internal/adapters/http/server.go internal/adapters/http/itinerary_handler_test.go
git commit -m "http: POST /featured-itineraries (admin), GET /featured-itineraries (public)"
```

---

### Task 5: Mobile — Discover section, featured itinerary detail screen

**Files:**
- Modify: `mobile/src/data/ItinerariesRepository.ts`
- Modify: `mobile/src/screens/Map.tsx`
- Create: `mobile/src/screens/FeaturedItineraryDetail.tsx`
- Modify: `mobile/src/navigation/types.ts`
- Modify: `mobile/src/navigation/AppNavigator.tsx`
- Modify: `mobile/src/i18n/dictionary.ts`

**Interfaces:**
- Consumes: Task 4's `GET /featured-itineraries` (public, `200` always, `itineraryResponse[]`).
- Produces: `listFeaturedItineraries(): Promise<Itinerary[]>` (mobile), `AppStackParamList`'s new
  `FeaturedItineraryDetail: { itinerary: Itinerary }` route.

- [ ] **Step 1: Add `listFeaturedItineraries` to `ItinerariesRepository.ts`**

Add at the end of `mobile/src/data/ItinerariesRepository.ts`:

```ts
// Deliberately does NOT go through itinerariesFetch -- every other function
// in this file requires a token (Authorization header always sent);
// featured itineraries are public, so this is a plain fetch with no auth at
// all, proving out the same "works while logged out" contract the backend
// route itself guarantees.
export async function listFeaturedItineraries(): Promise<Itinerary[]> {
  const res = await fetch(`${API_BASE_URL}/featured-itineraries`);
  if (!res.ok) throw new ItinerariesApiError("request failed", res.status);
  const wire = (await res.json()) as WireItinerary[];
  return wire.map(fromWire);
}
```

- [ ] **Step 2: Add the "Discover" section to `Map.tsx`**

Add a new state and effect, alongside the existing `places`/`offlineCount` state (near the top of
`MapScreen`, after the existing `useEffect` that calls `placesRepository.listNearby()`):

```tsx
  const [featuredItineraries, setFeaturedItineraries] = useState<Itinerary[]>([]);

  useEffect(() => {
    listFeaturedItineraries()
      .then(setFeaturedItineraries)
      .catch(() => {
        // A failed fetch here just means the Discover section stays empty
        // -- it's a bonus surface, not core map functionality, so this
        // never blocks or degrades the rest of the screen.
      });
  }, []);
```

Add the imports this needs, alongside the existing ones:

```tsx
import { listFeaturedItineraries, type Itinerary } from "../data/ItinerariesRepository";
```

Add the section itself in the JSX, between the closing `</ScrollView>` of the existing filter row and
the `<View style={styles.map}>` block:

```tsx
      {featuredItineraries.length > 0 ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          style={styles.discoverRow}
          contentContainerStyle={styles.discoverRowContent}
        >
          <Text style={styles.discoverSectionTitle}>{t.map.discoverTitle}</Text>
          {featuredItineraries.map((it) => (
            <Pressable
              key={it.id}
              style={styles.discoverCard}
              onPress={() => navigation.navigate("FeaturedItineraryDetail", { itinerary: it })}
            >
              <Text style={styles.discoverCardTitle}>{it.title}</Text>
              <Text style={styles.discoverCardMeta}>
                {formatDuration(it.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(it.placeCount))}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
      ) : null}
```

(The section title is rendered as the first item in the same horizontal scroll as the cards -- matching
how `map.allCategories` sits as the first "chip" in the existing filter row above it, rather than
introducing a new layout pattern for a single-purpose header.)

Add the `formatDuration` import (not currently imported in this file):

```tsx
import { formatDuration } from "../utils/itineraryFormat";
```

Add these styles to the `StyleSheet.create` call, after the existing `filterRowContent` entry:

```tsx
  discoverRow: {
    flexGrow: 0,
    flexShrink: 0,
    backgroundColor: colors.cream,
    borderBottomWidth: 1,
    borderBottomColor: colors.line,
  },
  discoverRowContent: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    paddingHorizontal: 18,
    paddingVertical: 12,
  },
  discoverSectionTitle: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.inkSoft, marginRight: 4 },
  discoverCard: {
    backgroundColor: colors.white,
    borderRadius: radii.md,
    borderWidth: 1,
    borderColor: colors.line,
    paddingVertical: 10,
    paddingHorizontal: 14,
    minWidth: 160,
  },
  discoverCardTitle: { fontFamily: fonts.bodySemiBold, fontSize: 13.5, color: colors.ink, marginBottom: 2 },
  discoverCardMeta: { fontFamily: fonts.body, fontSize: 11.5, color: colors.inkSoft },
```

- [ ] **Step 3: Write `FeaturedItineraryDetail.tsx`**

This is a read-only sibling of `ItineraryDetail.tsx` — same visual layout, but it receives the full
`Itinerary` object via route params (no fetch, no `token`, no loading/not-found states) since
`GET /featured-itineraries` already returned complete stop data when `Map.tsx` fetched the list:

```tsx
// mobile/src/screens/FeaturedItineraryDetail.tsx
import React from "react";
import { View, Text, Pressable, ScrollView, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Svg, { Polyline, Path } from "react-native-svg";
import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { AppStackParamList } from "../navigation/types";
import { useLocale } from "../i18n/LocaleContext";
import { formatDuration } from "../utils/itineraryFormat";
import { colors, fonts, radii } from "../theme/tokens";

type Props = NativeStackScreenProps<AppStackParamList, "FeaturedItineraryDetail">;

export function FeaturedItineraryDetailScreen({ route, navigation }: Props) {
  const { t } = useLocale();
  const { itinerary } = route.params;

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.topbar}>
        <Pressable style={styles.back} onPress={() => navigation.goBack()}>
          <Svg width={16} height={16} viewBox="0 0 24 24" fill="none">
            <Polyline points="15 6 9 12 15 18" stroke={colors.ink} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
          </Svg>
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.title}>{itinerary.title}</Text>
        <Text style={styles.meta}>
          {formatDuration(itinerary.totalMinutes)} · {t.itineraries.stopCount.replace("{count}", String(itinerary.placeCount))}
        </Text>

        <View style={styles.timeline}>
          {(() => {
            let placeNumber = 0;
            return itinerary.stops.map((stop, i) => {
              const isSuggestion = stop.kind === "suggestion";
              if (!isSuggestion) placeNumber += 1;
              const isLast = i === itinerary.stops.length - 1;
              return (
                <View key={i} style={styles.stopRow}>
                  <View style={styles.badgeColumn}>
                    <View style={[styles.badge, isSuggestion && styles.badgeSuggestion]}>
                      {isSuggestion ? (
                        <Text style={styles.badgeEmoji}>🍽️</Text>
                      ) : (
                        <Text style={styles.badgeText}>{placeNumber}</Text>
                      )}
                    </View>
                    {!isLast && <View style={styles.connector} />}
                  </View>
                  <View style={styles.stopBody}>
                    <Text style={[styles.stopLabel, isSuggestion && styles.stopLabelSuggestion]}>
                      {stop.label}
                    </Text>
                    {isSuggestion ? (
                      <Text style={styles.suggestionCaption}>{t.itineraries.suggestionLabel}</Text>
                    ) : (
                      <Text style={styles.stopMeta}>{formatDuration(stop.timeOnSiteMinutes)}</Text>
                    )}
                    {!isLast && stop.walkToNextMinutes > 0 && (
                      <Text style={styles.walkMeta}>
                        {t.itineraries.walkToNext.replace("{minutes}", String(stop.walkToNextMinutes))}
                      </Text>
                    )}
                  </View>
                </View>
              );
            });
          })()}
        </View>
      </ScrollView>
      <Pressable style={styles.startBtn} onPress={() => navigation.navigate("Map")}>
        <Svg width={18} height={18} viewBox="0 0 24 24" fill="none">
          <Path d="M5 12h14M13 5l7 7-7 7" stroke={colors.cream} strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
        </Svg>
        <Text style={styles.startBtnText}>{t.itineraries.startItinerary}</Text>
      </Pressable>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.cream },
  topbar: { flexDirection: "row", alignItems: "center", paddingHorizontal: 20, paddingTop: 8 },
  back: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.white,
    borderWidth: 1,
    borderColor: colors.line,
    alignItems: "center",
    justifyContent: "center",
  },
  content: { paddingHorizontal: 22, paddingTop: 16, paddingBottom: 40 },
  title: { fontFamily: fonts.display, fontSize: 24, color: colors.ink, marginBottom: 6 },
  meta: { fontFamily: fonts.body, fontSize: 13.5, color: colors.inkSoft, marginBottom: 24 },
  timeline: {},
  stopRow: { flexDirection: "row", gap: 14 },
  badgeColumn: { alignItems: "center", width: 28 },
  badge: {
    width: 28,
    height: 28,
    borderRadius: 14,
    backgroundColor: colors.terracotta,
    alignItems: "center",
    justifyContent: "center",
  },
  badgeSuggestion: {
    backgroundColor: "transparent",
    borderWidth: 1.5,
    borderColor: colors.inkFaint,
    borderStyle: "dashed",
  },
  badgeText: { fontFamily: fonts.bodyBold, fontSize: 13, color: colors.cream },
  badgeEmoji: { fontSize: 12 },
  connector: { width: 2, flex: 1, minHeight: 24, backgroundColor: colors.line, marginTop: 2 },
  stopBody: { flex: 1, paddingBottom: 22 },
  stopLabel: { fontFamily: fonts.bodySemiBold, fontSize: 15.5, color: colors.ink },
  stopLabelSuggestion: { fontStyle: "italic", color: colors.inkSoft },
  stopMeta: { fontFamily: fonts.body, fontSize: 12.5, color: colors.inkSoft, marginTop: 2 },
  suggestionCaption: { fontFamily: fonts.body, fontSize: 12, fontStyle: "italic", color: colors.inkFaint, marginTop: 2 },
  walkMeta: { fontFamily: fonts.body, fontSize: 12, color: colors.inkFaint, marginTop: 8 },
  startBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    backgroundColor: colors.terracotta,
    borderRadius: radii.md,
    marginHorizontal: 20,
    marginBottom: 16,
    paddingVertical: 16,
  },
  startBtnText: { fontFamily: fonts.bodyBold, fontSize: 15, color: colors.cream },
});
```

- [ ] **Step 4: Add the new route to `navigation/types.ts`**

```ts
export type AppStackParamList = {
  Map: undefined;
  PlaceDetail: { placeId: string };
  Assistant: { placeId: string };
  Settings: undefined;
  Auth: undefined;
  EditProfile: undefined;
  ItinerariesList: undefined;
  ItineraryChat: undefined;
  ItineraryDetail: { itineraryId: string };
  FeaturedItineraryDetail: { itinerary: Itinerary };
  VerifyEmail: { email: string; password: string; codeAlreadySent: boolean };
  ForgotPassword: undefined;
  ResetPassword: { email: string };
};
```

Add the import this needs at the top of the file:

```ts
import type { Itinerary } from "../data/ItinerariesRepository";
```

- [ ] **Step 5: Register the new screen in `AppNavigator.tsx`**

```tsx
import { FeaturedItineraryDetailScreen } from "../screens/FeaturedItineraryDetail";
```

(added to the import block, alongside the other screen imports)

```tsx
      <Stack.Screen name="ItineraryDetail" component={ItineraryDetailScreen} />
      <Stack.Screen name="FeaturedItineraryDetail" component={FeaturedItineraryDetailScreen} />
```

(added right after `ItineraryDetail`'s registration — same group as the other non-modal screens, no
`presentation: "modal"` option, since this is reachable from `Map` directly like `PlaceDetail` is)

- [ ] **Step 6: Add the new dictionary key (all 4 locales)**

In `mobile/src/i18n/dictionary.ts`, each of the 4 `map: { ... }` blocks gains one new key, right after
the existing `webMapUnavailable` line:

- `fr`: `discoverTitle: "À découvrir",`
- `en`: `discoverTitle: "Discover",`
- `pt`: `discoverTitle: "Para descobrir",`
- `es`: `discoverTitle: "Para descubrir",`

No other new keys are needed — `FeaturedItineraryDetailScreen` reuses `t.itineraries.stopCount`,
`t.itineraries.suggestionLabel`, `t.itineraries.walkToNext`, and `t.itineraries.startItinerary`, all of
which already exist in all 4 locales.

- [ ] **Step 7: Typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 8: Manual verification**

With the backend running locally and Task 4's routes live: as an admin account, call
`POST /featured-itineraries` (e.g. via `curl` with a real admin JWT and a couple of real local
`place_id`s) to create one. Reload the mobile app's map screen **while logged out** and confirm the
"Discover" section appears and is tappable without a login prompt — this is the entire point of the
feature, so confirming it specifically while logged out (not just while logged in) matters.

- [ ] **Step 9: Commit**

```bash
git add src/data/ItinerariesRepository.ts src/screens/Map.tsx src/screens/FeaturedItineraryDetail.tsx src/navigation/types.ts src/navigation/AppNavigator.tsx src/i18n/dictionary.ts
git commit -m "mobile: Discover section, featured itinerary detail screen"
```
