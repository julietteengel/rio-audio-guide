# AI Itineraries Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new `Itinerary` domain concept, generated from a user's free-text request via the Claude API
(constrained to real, grounded places already in this app's database, plus one optional unverified
meal-break suggestion), persisted per-user, exposed over three new authenticated HTTP routes.

**Architecture:** Standard hexagonal layering, same as every other feature in this backend —
`internal/domain` (new `Itinerary`/`ItineraryStop` types), `internal/ports` (`ItineraryRepository`,
`ItineraryGenerator`), `internal/adapters/postgres` (repository) and `internal/adapters/claude` (the
Claude-calling generator, new adapter package), `internal/application` (orchestration), `internal/adapters/http`
(routes). As of 2026-09-06 the founder-only restriction on `internal/domain`/`internal/ports` authorship
is lifted project-wide (see `CLAUDE.md` and `mission.md`) — this is the first plan exercising that.

**Tech Stack:** Go 1.25.0, module `rioaudioguide/backend`. New dependency:
`github.com/anthropics/anthropic-sdk-go` (Claude API). No changes to the mobile app in this plan — that's
a separate, following plan once this one's HTTP contract exists to build against.

**Spec:** `docs/superpowers/specs/2026-09-06-ai-itineraries-design.md`

## Global Constraints

- Go 1.25.0, module `rioaudioguide/backend`. All file paths below are relative to `backend/`.
- The generator must never invent a place: every non-suggestion stop it returns must reference a real
  `place_id` from the candidate list it was given. The adapter validates this itself (defense in depth,
  not just prompted behavior) and rejects the whole generation if any place stop references an ID
  outside the candidate set.
- Exactly one optional "suggestion" stop is allowed per itinerary (the meal-break-style slot) — anything
  beyond that is a spec violation, not a quality nitpick.
- Claude API: model `claude-opus-5` (this project's default per its own tooling conventions — no user
  request to use a different model), no explicit `Thinking` param (Opus 5 defaults to adaptive), no
  streaming (response is a small structured tool call, not a long generation — `MaxTokens: 4096` is
  ample and keeps a synchronous HTTP request from waiting on more headroom than this task needs).
- `ANTHROPIC_API_KEY` is read from the environment by `anthropic.NewClient()` with no explicit
  configuration (matches this project's existing convention of not re-plumbing SDK-standard env vars
  through custom config) — document it in `cmd/api/main.go`'s composition root the same way
  `S3_BUCKET`/`JWT_SECRET` are already documented there.
- Itineraries are not editable after generation in this plan (no update/delete routes) — regenerating
  is the only "edit" path, and that's a future mobile-side decision, not built here.
- No component/handler is exempt from this project's existing testing conventions: pure domain logic
  and adapters get real tests; thin HTTP handlers get the same style of test as `audio_handler_test.go`
  already uses (fakes for every port, `httptest`).

---

### Task 1: `Itinerary` domain aggregate

**Files:**
- Create: `internal/domain/itinerary.go`
- Test: `internal/domain/itinerary_test.go`

**Interfaces:**
- Produces: `ItineraryStopKind` (`ItineraryStopKindPlace`/`ItineraryStopKindSuggestion`),
  `ItineraryStop` (+ `NewPlaceStop`, `NewSuggestionStop`, `ReconstructPlaceStop`,
  `ReconstructSuggestionStop`), `ItineraryTitle` (+ `NewItineraryTitle`), `Itinerary` (+ `NewItinerary`,
  `ReconstructItinerary`) — consumed by Task 2 (ports), Task 3 (postgres adapter), Task 5 (application).

- [ ] **Step 1: Write the failing tests**

```go
// internal/domain/itinerary_test.go
package domain

import "testing"

func TestNewPlaceStop(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		stop, err := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stop.Kind() != ItineraryStopKindPlace {
			t.Fatalf("got kind %q, want place", stop.Kind())
		}
		if stop.PlaceID() != "place-1" || stop.Label() != "Escadaria Selarón" {
			t.Fatalf("got placeID=%q label=%q", stop.PlaceID(), stop.Label())
		}
		if stop.TimeOnSiteMinutes() != 10 || stop.WalkToNextMinutes() != 8 {
			t.Fatalf("got timeOnSite=%d walkToNext=%d", stop.TimeOnSiteMinutes(), stop.WalkToNextMinutes())
		}
		if stop.ID() == "" {
			t.Fatal("expected a generated ID")
		}
	})
	t.Run("empty place id", func(t *testing.T) {
		if _, err := NewPlaceStop("", "label", 10, 8); err != ErrItineraryStopPlaceIDRequired {
			t.Fatalf("got %v, want ErrItineraryStopPlaceIDRequired", err)
		}
	})
	t.Run("empty label", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "", 10, 8); err != ErrItineraryStopLabelRequired {
			t.Fatalf("got %v, want ErrItineraryStopLabelRequired", err)
		}
	})
	t.Run("non-positive time on site", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "label", 0, 8); err != ErrItineraryStopInvalidDuration {
			t.Fatalf("got %v, want ErrItineraryStopInvalidDuration", err)
		}
	})
	t.Run("negative walk to next", func(t *testing.T) {
		if _, err := NewPlaceStop("place-1", "label", 10, -1); err != ErrItineraryStopInvalidWalk {
			t.Fatalf("got %v, want ErrItineraryStopInvalidWalk", err)
		}
	})
}

func TestNewSuggestionStop(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		stop, err := NewSuggestionStop("Pause déjeuner", 12)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stop.Kind() != ItineraryStopKindSuggestion {
			t.Fatalf("got kind %q, want suggestion", stop.Kind())
		}
		if stop.PlaceID() != "" {
			t.Fatalf("suggestion stop must not carry a place id, got %q", stop.PlaceID())
		}
		if stop.TimeOnSiteMinutes() != 0 {
			t.Fatalf("suggestion stop has no time-on-site, got %d", stop.TimeOnSiteMinutes())
		}
	})
	t.Run("empty label", func(t *testing.T) {
		if _, err := NewSuggestionStop("", 12); err != ErrItineraryStopLabelRequired {
			t.Fatalf("got %v, want ErrItineraryStopLabelRequired", err)
		}
	})
	t.Run("negative walk to next", func(t *testing.T) {
		if _, err := NewSuggestionStop("label", -1); err != ErrItineraryStopInvalidWalk {
			t.Fatalf("got %v, want ErrItineraryStopInvalidWalk", err)
		}
	})
}

func TestNewItinerary(t *testing.T) {
	title, _ := NewItineraryTitle("Art et rue à Santa Teresa")
	stop, _ := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)

	t.Run("valid", func(t *testing.T) {
		it, err := NewItinerary("user-1", title, []ItineraryStop{stop})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if it.ID() == "" {
			t.Fatal("expected a generated ID")
		}
		if it.UserID() != "user-1" || it.Title() != title {
			t.Fatalf("got userID=%q title=%q", it.UserID(), it.Title())
		}
	})
	t.Run("empty user id", func(t *testing.T) {
		if _, err := NewItinerary("", title, []ItineraryStop{stop}); err != ErrItineraryUserIDRequired {
			t.Fatalf("got %v, want ErrItineraryUserIDRequired", err)
		}
	})
	t.Run("no stops", func(t *testing.T) {
		if _, err := NewItinerary("user-1", title, nil); err != ErrItineraryNoStops {
			t.Fatalf("got %v, want ErrItineraryNoStops", err)
		}
	})
}

func TestItinerary_TotalMinutesAndPlaceCount(t *testing.T) {
	title, _ := NewItineraryTitle("Art et rue à Santa Teresa")
	stop1, _ := NewPlaceStop("place-1", "Escadaria Selarón", 10, 8)
	lunch, _ := NewSuggestionStop("Pause déjeuner", 5)
	stop2, _ := NewPlaceStop("place-2", "Parque das Ruínas", 20, 0)

	it, err := NewItinerary("user-1", title, []ItineraryStop{stop1, lunch, stop2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 10+8 (stop1) + 0+5 (lunch, no time-on-site) + 20+0 (stop2) = 43
	if got := it.TotalMinutes(); got != 43 {
		t.Fatalf("got TotalMinutes()=%d, want 43", got)
	}
	// Only stop1/stop2 are real places -- the meal-break suggestion doesn't count.
	if got := it.PlaceCount(); got != 2 {
		t.Fatalf("got PlaceCount()=%d, want 2", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/... -run 'TestNewPlaceStop|TestNewSuggestionStop|TestNewItinerary|TestItinerary_TotalMinutesAndPlaceCount' -v`
Expected: FAIL — none of these types/functions exist yet.

- [ ] **Step 3: Write the implementation**

```go
// internal/domain/itinerary.go
package domain

import (
	"errors"
	"time"
)

var (
	ErrItineraryUserIDRequired      = errors.New("itinerary: user id is required")
	ErrItineraryTitleRequired       = errors.New("itinerary: title is required")
	ErrItineraryNoStops             = errors.New("itinerary: at least one stop is required")
	ErrItineraryStopPlaceIDRequired = errors.New("itinerary: a place stop requires a place id")
	ErrItineraryStopLabelRequired   = errors.New("itinerary: a stop requires a label")
	ErrItineraryStopInvalidDuration = errors.New("itinerary: a place stop's time on site must be positive")
	ErrItineraryStopInvalidWalk     = errors.New("itinerary: walking time to the next stop cannot be negative")
)

// --- Value Objects ---

type ItineraryTitle string

func NewItineraryTitle(s string) (ItineraryTitle, error) {
	if s == "" {
		return "", ErrItineraryTitleRequired
	}
	return ItineraryTitle(s), nil
}

func (t ItineraryTitle) String() string { return string(t) }

type ItineraryStopKind string

const (
	ItineraryStopKindPlace      ItineraryStopKind = "place"
	ItineraryStopKindSuggestion ItineraryStopKind = "suggestion"
)

// ItineraryStop is a value object with its own generated ID (not identity in
// the DDD entity sense -- it never changes after construction -- but it
// still needs a stable ID so the Postgres adapter can persist an ordered
// list of stops as rows and read them back as the same values, the same
// reason WordMark-like child records elsewhere in this project carry an ID).
type ItineraryStop struct {
	id                string
	kind              ItineraryStopKind
	placeID           string // set only when kind == ItineraryStopKindPlace
	label             string // place name (kind=place) or suggestion label (kind=suggestion), e.g. "Pause déjeuner"
	timeOnSiteMinutes int    // 0 for a suggestion stop -- it's not a real visitable place with a duration
	walkToNextMinutes int    // 0 for the itinerary's last stop
}

// NewPlaceStop is the only way to add a real, grounded place to an
// itinerary -- placeID must be a real domain.Place ID, enforced by the
// caller (internal/application), not by this constructor, which only knows
// this is a non-empty string.
func NewPlaceStop(placeID, label string, timeOnSiteMinutes, walkToNextMinutes int) (ItineraryStop, error) {
	if placeID == "" {
		return ItineraryStop{}, ErrItineraryStopPlaceIDRequired
	}
	if label == "" {
		return ItineraryStop{}, ErrItineraryStopLabelRequired
	}
	if timeOnSiteMinutes <= 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidDuration
	}
	if walkToNextMinutes < 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidWalk
	}
	return ItineraryStop{
		id: newID(), kind: ItineraryStopKindPlace, placeID: placeID, label: label,
		timeOnSiteMinutes: timeOnSiteMinutes, walkToNextMinutes: walkToNextMinutes,
	}, nil
}

// NewSuggestionStop is the ONLY sanctioned way to add ungrounded content to
// an itinerary (the meal-break-style slot) -- deliberately has no
// timeOnSiteMinutes parameter at all, not just a validated-to-zero one, so
// a caller can never accidentally give an unverified suggestion the same
// "real place with a visit duration" shape as a grounded stop.
func NewSuggestionStop(label string, walkToNextMinutes int) (ItineraryStop, error) {
	if label == "" {
		return ItineraryStop{}, ErrItineraryStopLabelRequired
	}
	if walkToNextMinutes < 0 {
		return ItineraryStop{}, ErrItineraryStopInvalidWalk
	}
	return ItineraryStop{id: newID(), kind: ItineraryStopKindSuggestion, label: label, walkToNextMinutes: walkToNextMinutes}, nil
}

// ReconstructPlaceStop/ReconstructSuggestionStop rebuild a stop from
// already-valid data (a Postgres row) -- preserve the given ID, don't
// revalidate, same convention as ReconstructScript/ReconstructUser.
func ReconstructPlaceStop(id, placeID, label string, timeOnSiteMinutes, walkToNextMinutes int) ItineraryStop {
	return ItineraryStop{id: id, kind: ItineraryStopKindPlace, placeID: placeID, label: label, timeOnSiteMinutes: timeOnSiteMinutes, walkToNextMinutes: walkToNextMinutes}
}

func ReconstructSuggestionStop(id, label string, walkToNextMinutes int) ItineraryStop {
	return ItineraryStop{id: id, kind: ItineraryStopKindSuggestion, label: label, walkToNextMinutes: walkToNextMinutes}
}

func (s ItineraryStop) ID() string                  { return s.id }
func (s ItineraryStop) Kind() ItineraryStopKind      { return s.kind }
func (s ItineraryStop) PlaceID() string              { return s.placeID }
func (s ItineraryStop) Label() string                { return s.label }
func (s ItineraryStop) TimeOnSiteMinutes() int       { return s.timeOnSiteMinutes }
func (s ItineraryStop) WalkToNextMinutes() int       { return s.walkToNextMinutes }

// --- Entity ---

type Itinerary struct {
	id        string
	userID    string
	title     ItineraryTitle
	stops     []ItineraryStop
	createdAt time.Time
}

// NewItinerary ne retourne une erreur que sur userID/stops -- title est déjà
// un Value Object validé (même convention que NewScript pour language/text).
func NewItinerary(userID string, title ItineraryTitle, stops []ItineraryStop) (*Itinerary, error) {
	if userID == "" {
		return nil, ErrItineraryUserIDRequired
	}
	if len(stops) == 0 {
		return nil, ErrItineraryNoStops
	}
	return &Itinerary{id: newID(), userID: userID, title: title, stops: stops, createdAt: time.Now()}, nil
}

// ReconstructItinerary rebâtit un Itinerary depuis des données déjà valides
// (des lignes Postgres) -- préserve l'ID et createdAt donnés, ne revalide rien.
func ReconstructItinerary(id, userID string, title ItineraryTitle, stops []ItineraryStop, createdAt time.Time) *Itinerary {
	return &Itinerary{id: id, userID: userID, title: title, stops: stops, createdAt: createdAt}
}

func (i *Itinerary) ID() string             { return i.id }
func (i *Itinerary) UserID() string         { return i.userID }
func (i *Itinerary) Title() ItineraryTitle  { return i.title }
func (i *Itinerary) Stops() []ItineraryStop { return i.stops }
func (i *Itinerary) CreatedAt() time.Time   { return i.createdAt }

// TotalMinutes sums every stop's time-on-site plus every walk segment --
// the "1h · 4 lieux" duration the app shows is derived, never stored
// separately, so it can never drift from the stops that make it up.
func (i *Itinerary) TotalMinutes() int {
	total := 0
	for _, s := range i.stops {
		total += s.TimeOnSiteMinutes() + s.WalkToNextMinutes()
	}
	return total
}

// PlaceCount counts only real place stops -- the meal-break suggestion slot
// is not a "lieu" in the sense the UI shows ("4 lieux").
func (i *Itinerary) PlaceCount() int {
	count := 0
	for _, s := range i.stops {
		if s.Kind() == ItineraryStopKindPlace {
			count++
		}
	}
	return count
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/... -run 'TestNewPlaceStop|TestNewSuggestionStop|TestNewItinerary|TestItinerary_TotalMinutesAndPlaceCount' -v`
Expected: PASS (all cases). Then run the whole package once: `go test ./internal/domain/...` to confirm nothing else in the package broke.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/itinerary.go internal/domain/itinerary_test.go
git commit -m "domain: Itinerary aggregate (place stops + one allowed unverified suggestion stop)"
```

---

### Task 2: Ports — `ItineraryRepository`, `ItineraryGenerator`

**Files:**
- Create: `internal/ports/itinerary_repository.go`
- Create: `internal/ports/itinerary_generator.go`

**Interfaces:**
- Consumes: `domain.Itinerary` (Task 1).
- Produces: `ports.ItineraryRepository`, `ports.CandidatePlace`, `ports.GeneratedStop`,
  `ports.GeneratedItinerary`, `ports.ItineraryGenerator` — consumed by Task 3 (postgres), Task 4
  (claude adapter), Task 5 (application).

No TDD here — these are pure interface/type declarations with no logic of their own to test, same
convention already used for this project's other port files (`place_repository.go`, `tts_generator.go`'s
interface itself, as opposed to its `PermanentError` type which does get tested).

- [ ] **Step 1: Write the repository port**

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
}
```

- [ ] **Step 2: Write the generator port**

```go
// internal/ports/itinerary_generator.go
package ports

import "context"

// CandidatePlace is the minimal, real place data the generator may choose
// from -- never richer than what's actually in this app's grounded
// database, so nothing the LLM returns can reference a place that doesn't
// exist here. Built from domain.Place by internal/application, not by this
// package (ports must not depend on how adapters construct their inputs).
type CandidatePlace struct {
	ID       string
	Name     string
	Category string
	Lat, Lon float64
}

// GeneratedStop mirrors domain.ItineraryStop's two kinds without importing
// domain -- ports depend on domain for repository return types, but a
// generator's raw output hasn't been validated into real domain.ItineraryStop
// values yet (internal/application does that, via domain.NewPlaceStop/
// domain.NewSuggestionStop, which is also where invalid output from the LLM
// gets rejected rather than silently trusted).
type GeneratedStop struct {
	IsSuggestion      bool   // true only for the one allowed meal-break-style slot
	PlaceID           string // set when !IsSuggestion; must be one of the CandidatePlace IDs given to Generate
	Label             string
	TimeOnSiteMinutes int // ignored when IsSuggestion
	WalkToNextMinutes int
}

type GeneratedItinerary struct {
	Title string
	Stops []GeneratedStop
}

// ItineraryGenerator is the outbound port to an LLM -- implemented by
// internal/adapters/claude. request is the user's free-text description;
// candidates is every place the generator is allowed to choose from.
type ItineraryGenerator interface {
	Generate(ctx context.Context, request string, candidates []CandidatePlace) (GeneratedItinerary, error)
}
```

- [ ] **Step 3: Confirm the package compiles**

Run: `go build ./internal/ports/...`
Expected: builds cleanly.

- [ ] **Step 4: Commit**

```bash
git add internal/ports/itinerary_repository.go internal/ports/itinerary_generator.go
git commit -m "ports: ItineraryRepository and ItineraryGenerator"
```

---

### Task 3: Postgres adapter — schema + `ItineraryRepository`

**Files:**
- Modify: `internal/adapters/postgres/schema.sql`
- Create: `internal/adapters/postgres/itinerary_repository.go`
- Test: `internal/adapters/postgres/itinerary_repository_test.go`

**Interfaces:**
- Consumes: `domain.Itinerary`/`domain.ItineraryStop` (Task 1), `ports.ItineraryRepository` (Task 2).
- Produces: `postgres.NewItineraryRepository(pool *pgxpool.Pool) *ItineraryRepository` — consumed by
  Task 6 (`cmd/api/main.go` wiring).

- [ ] **Step 1: Add the schema**

Append to `internal/adapters/postgres/schema.sql`:

```sql

CREATE TABLE itineraries (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    title      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX itineraries_user_id_idx ON itineraries (user_id);

CREATE TABLE itinerary_stops (
    id                   TEXT PRIMARY KEY,
    itinerary_id         TEXT NOT NULL REFERENCES itineraries(id),
    position             INT NOT NULL,
    kind                 TEXT NOT NULL,
    place_id             TEXT REFERENCES places(id),
    label                TEXT NOT NULL,
    time_on_site_minutes INT NOT NULL DEFAULT 0,
    walk_to_next_minutes INT NOT NULL DEFAULT 0,
    UNIQUE (itinerary_id, position)
);
```

Apply it to your local dev database: `psql "$DATABASE_URL" -f internal/adapters/postgres/schema.sql`
(safe to re-run — every `CREATE TABLE` in this file is additive; if your local DB already has the
tables from a previous partial run, drop just those two first).

- [ ] **Step 2: Write the failing tests**

Generate a unique-enough fixture email inline the same way `script_repository_test.go` already does,
keyed off the fixture place's ID:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestItineraryRepository -v`
(requires `TEST_DATABASE_URL` pointed at a real Postgres — see this project's README)
Expected: FAIL — `NewItineraryRepository` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/adapters/postgres/itinerary_repository.go
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

// ItineraryRepository takes the pool directly, not the shared DBTX
// interface every other repository in this package uses -- Save needs a
// real transaction (the itinerary row and its stops must land atomically,
// or a failure partway through would leave an itinerary with the wrong
// stops attached to it), and DBTX deliberately has no Begin method (every
// other repository's single-row upserts never needed one).
type ItineraryRepository struct {
	pool *pgxpool.Pool
}

func NewItineraryRepository(pool *pgxpool.Pool) *ItineraryRepository {
	return &ItineraryRepository{pool: pool}
}

const upsertItinerarySQL = `
	INSERT INTO itineraries (id, user_id, title, created_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title
`

const insertStopSQL = `
	INSERT INTO itinerary_stops (id, itinerary_id, position, kind, place_id, label, time_on_site_minutes, walk_to_next_minutes)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`

func (r *ItineraryRepository) Save(ctx context.Context, itinerary *domain.Itinerary) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once Commit has already succeeded

	if _, err := tx.Exec(ctx, upsertItinerarySQL, itinerary.ID(), itinerary.UserID(), itinerary.Title().String(), itinerary.CreatedAt()); err != nil {
		return err
	}
	// Delete-then-reinsert rather than a per-stop upsert: this plan never
	// edits an itinerary after generation, so there's no existing-stop set
	// to merge with -- a full replace is simpler and can't leave a stale
	// stop from a previous save (not reachable today, but Save is still
	// named Save, not Create, so it should behave correctly if that ever
	// changes).
	if _, err := tx.Exec(ctx, "DELETE FROM itinerary_stops WHERE itinerary_id = $1", itinerary.ID()); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	for i, stop := range itinerary.Stops() {
		var placeID any
		if stop.Kind() == domain.ItineraryStopKindPlace {
			placeID = stop.PlaceID()
		}
		batch.Queue(insertStopSQL, stop.ID(), itinerary.ID(), i, string(stop.Kind()), placeID, stop.Label(), stop.TimeOnSiteMinutes(), stop.WalkToNextMinutes())
	}
	results := tx.SendBatch(ctx, batch)
	for range itinerary.Stops() {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const selectStopsByItineraryIDSQL = `
	SELECT id, kind, place_id, label, time_on_site_minutes, walk_to_next_minutes
	FROM itinerary_stops
	WHERE itinerary_id = $1
	ORDER BY position
`

func (r *ItineraryRepository) loadStops(ctx context.Context, itineraryID string) ([]domain.ItineraryStop, error) {
	rows, err := r.pool.Query(ctx, selectStopsByItineraryIDSQL, itineraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stops []domain.ItineraryStop
	for rows.Next() {
		var id, kind, label string
		var placeID *string
		var timeOnSite, walkToNext int
		if err := rows.Scan(&id, &kind, &placeID, &label, &timeOnSite, &walkToNext); err != nil {
			return nil, err
		}
		if kind == string(domain.ItineraryStopKindPlace) {
			stops = append(stops, domain.ReconstructPlaceStop(id, *placeID, label, timeOnSite, walkToNext))
		} else {
			stops = append(stops, domain.ReconstructSuggestionStop(id, label, walkToNext))
		}
	}
	return stops, rows.Err()
}

const selectItineraryByIDSQL = `SELECT id, user_id, title, created_at FROM itineraries WHERE id = $1`

// created_at scans directly into time.Time -- pgx converts Postgres
// TIMESTAMPTZ to time.Time natively (see scriptSaveArgs/ReconstructScript
// elsewhere in this same package, which pass time.Time values straight
// through); no not-found special-casing is needed here either, a missing
// row already returns a plain error from Scan, matching every other
// FindByID in this package.
func (r *ItineraryRepository) FindByID(ctx context.Context, id string) (*domain.Itinerary, error) {
	var (
		itinID, userID, titleStr string
		createdAt                time.Time
	)
	row := r.pool.QueryRow(ctx, selectItineraryByIDSQL, id)
	if err := row.Scan(&itinID, &userID, &titleStr, &createdAt); err != nil {
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
	return domain.ReconstructItinerary(itinID, userID, title, stops, createdAt), nil
}
```

```go
const selectItinerariesByUserIDSQL = `
	SELECT id, user_id, title, created_at
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
		if err := rows.Scan(&id, &uid, &titleStr, &createdAt); err != nil {
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
		itineraries = append(itineraries, domain.ReconstructItinerary(id, uid, title, stops, createdAt))
	}
	return itineraries, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestItineraryRepository -v`
Expected: PASS (both tests). If no local Postgres is reachable, report DONE_WITH_CONCERNS stating
plainly that `-tags=integration` was not executed, matching this project's existing convention for
every other postgres/rabbitmq integration test (see the AWS Polly plan's Task 5 for the precedent) --
`go build -tags=integration ./internal/adapters/postgres/...` alone (which does not need a live
database) must still pass and catch any compile-level mistake.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/postgres/schema.sql internal/adapters/postgres/itinerary_repository.go internal/adapters/postgres/itinerary_repository_test.go
git commit -m "postgres: ItineraryRepository (schema + save/find, atomic stop replace)"
```

---

### Task 4: Claude-calling `ItineraryGenerator` adapter

**Files:**
- Create: `internal/adapters/claude/itinerary_generator.go`
- Test: `internal/adapters/claude/itinerary_generator_test.go`
- Modify: `go.mod`, `go.sum` (new dependency)

**Interfaces:**
- Consumes: `ports.CandidatePlace`, `ports.GeneratedStop`, `ports.GeneratedItinerary`,
  `ports.ItineraryGenerator` (Task 2).
- Produces: `claude.NewItineraryGenerator(client messagesAPI) *ItineraryGenerator` — consumed by Task 6
  (`cmd/api/main.go`).

Every type/method name and field below (`anthropic.MessageNewParams`, `ToolParam`,
`ToolInputSchemaParam{Properties, Required}`, `ToolChoiceUnionParam{OfTool: &ToolChoiceToolParam{Name}}`,
`ContentBlockUnion`/`AsAny()`/`ToolUseBlock.Input`, `MessageService.New(ctx, params, opts...)`) was
checked directly against `github.com/anthropics/anthropic-sdk-go`'s own source before this plan was
written — this is a transcription task for that part, not a design task; do not "improve" the SDK calls.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/anthropics/anthropic-sdk-go && go mod tidy`

- [ ] **Step 2: Write the failing tests**

```go
// internal/adapters/claude/itinerary_generator_test.go
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"rioaudioguide/backend/internal/ports"
)

// fakeMessagesAPI simule client.Messages.New sans toucher le réseau --
// renvoie directement le *anthropic.Message construit par le test.
type fakeMessagesAPI struct {
	response *anthropic.Message
	err      error
}

func (f *fakeMessagesAPI) New(_ context.Context, _ anthropic.MessageNewParams, _ ...option.RequestOption) (*anthropic.Message, error) {
	return f.response, f.err
}

func toolUseResponse(t *testing.T, input any) *anthropic.Message {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal fixture input: %v", err)
	}
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "tool_use", ID: "toolu_1", Name: "propose_itinerary", Input: raw},
		},
	}
}

var candidates = []ports.CandidatePlace{
	{ID: "place-1", Name: "Escadaria Selarón", Category: "monument", Lat: -22.9147, Lon: -43.1806},
	{ID: "place-2", Name: "Parque das Ruínas", Category: "monument", Lat: -22.9207, Lon: -43.1876},
}

func TestItineraryGenerator_Generate_ParsesToolUseResponse(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": false, "place_id": "place-1", "label": "Escadaria Selarón", "time_on_site_minutes": 10, "walk_to_next_minutes": 8},
			{"is_suggestion": true, "label": "Pause déjeuner", "walk_to_next_minutes": 5},
			{"is_suggestion": false, "place_id": "place-2", "label": "Parque das Ruínas", "time_on_site_minutes": 20, "walk_to_next_minutes": 0},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	got, err := gen.Generate(context.Background(), "1h à Santa Teresa, focus street art, avec pause déjeuner", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Art et rue à Santa Teresa" {
		t.Fatalf("got title %q", got.Title)
	}
	if len(got.Stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(got.Stops))
	}
	if got.Stops[0].PlaceID != "place-1" || got.Stops[0].IsSuggestion {
		t.Fatalf("stop 0 should be place-1, got %+v", got.Stops[0])
	}
	if !got.Stops[1].IsSuggestion || got.Stops[1].Label != "Pause déjeuner" {
		t.Fatalf("stop 1 should be the suggestion, got %+v", got.Stops[1])
	}
}

func TestItineraryGenerator_Generate_RejectsUnknownPlaceID(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": false, "place_id": "place-999-not-a-real-candidate", "label": "Lieu inventé", "time_on_site_minutes": 10, "walk_to_next_minutes": 0},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error for a place id outside the candidate list, got nil")
	}
}

func TestItineraryGenerator_Generate_RejectsMoreThanOneSuggestion(t *testing.T) {
	input := map[string]any{
		"title": "Art et rue à Santa Teresa",
		"stops": []map[string]any{
			{"is_suggestion": true, "label": "Pause déjeuner", "walk_to_next_minutes": 5},
			{"is_suggestion": true, "label": "Pause café", "walk_to_next_minutes": 5},
		},
	}
	fake := &fakeMessagesAPI{response: toolUseResponse(t, input)}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error for more than one suggestion stop, got nil")
	}
}

func TestItineraryGenerator_Generate_PropagatesAPIError(t *testing.T) {
	fake := &fakeMessagesAPI{err: errors.New("connection reset")}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected the underlying API error to propagate")
	}
}

func TestItineraryGenerator_Generate_ErrorsWhenNoToolUseBlock(t *testing.T) {
	fake := &fakeMessagesAPI{response: &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "I couldn't come up with an itinerary."}},
	}}
	gen := NewItineraryGenerator(fake)

	_, err := gen.Generate(context.Background(), "1h à Santa Teresa", candidates)
	if err == nil {
		t.Fatal("expected an error when Claude didn't call the tool")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/adapters/claude/... -v`
Expected: FAIL — `NewItineraryGenerator` undefined.

- [ ] **Step 4: Write the implementation**

```go
// internal/adapters/claude/itinerary_generator.go
package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"rioaudioguide/backend/internal/ports"
)

// messagesAPI n'expose que la méthode utilisée ici -- *anthropic.MessageService
// (client.Messages) la satisfait déjà structurellement, permettant de tester
// avec un faux client Go plutôt que de simuler la requête HTTP réelle du SDK,
// même convention que pollyAPI dans internal/adapters/awspolly.
type messagesAPI interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

type ItineraryGenerator struct {
	client messagesAPI
}

func NewItineraryGenerator(client messagesAPI) *ItineraryGenerator {
	return &ItineraryGenerator{client: client}
}

const systemPrompt = `You compose short walking itineraries from Rio de Janeiro's cultural sites for a tourist audio-guide app.

Rules:
- You may ONLY reference places from the candidate list given to you, by their exact place_id. Never invent a place, never reference a place not in the list.
- You may optionally add ONE additional stop with is_suggestion=true for a meal break, drawn from your own general knowledge of the area -- this is the only stop allowed without a real place_id. Never add more than one.
- Respect the user's stated time budget, theme, and neighborhood.
- Always call propose_itinerary with your answer -- never answer in plain text.`

var proposeItineraryTool = anthropic.ToolParam{
	Name:        "propose_itinerary",
	Description: anthropic.String("Propose a walking itinerary built only from the given candidate places, plus at most one optional meal-break suggestion."),
	InputSchema: anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"title": map[string]any{"type": "string"},
			"stops": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"is_suggestion":        map[string]any{"type": "boolean"},
						"place_id":             map[string]any{"type": "string"},
						"label":                map[string]any{"type": "string"},
						"time_on_site_minutes": map[string]any{"type": "integer"},
						"walk_to_next_minutes": map[string]any{"type": "integer"},
					},
					"required": []string{"is_suggestion", "label", "walk_to_next_minutes"},
				},
			},
		},
		Required: []string{"title", "stops"},
	},
}

type toolStopInput struct {
	IsSuggestion      bool   `json:"is_suggestion"`
	PlaceID           string `json:"place_id"`
	Label             string `json:"label"`
	TimeOnSiteMinutes int    `json:"time_on_site_minutes"`
	WalkToNextMinutes int    `json:"walk_to_next_minutes"`
}

type toolInput struct {
	Title string          `json:"title"`
	Stops []toolStopInput `json:"stops"`
}

func (g *ItineraryGenerator) Generate(ctx context.Context, request string, candidates []ports.CandidatePlace) (ports.GeneratedItinerary, error) {
	candidateLines := make([]string, 0, len(candidates))
	validPlaceIDs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		candidateLines = append(candidateLines, fmt.Sprintf("- %s (%s): %s, category=%s", c.ID, c.Name, c.Category, c.Category))
		validPlaceIDs[c.ID] = true
	}
	userMessage := fmt.Sprintf("Request: %s\n\nCandidate places:\n%s", request, joinLines(candidateLines))

	resp, err := g.client.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-opus-5",
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &proposeItineraryTool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "propose_itinerary"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
	})
	if err != nil {
		return ports.GeneratedItinerary{}, fmt.Errorf("claude: generate itinerary: %w", err)
	}

	var parsed *toolInput
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok && variant.Name == "propose_itinerary" {
			var in toolInput
			if err := json.Unmarshal(variant.Input, &in); err != nil {
				return ports.GeneratedItinerary{}, fmt.Errorf("claude: parse propose_itinerary input: %w", err)
			}
			parsed = &in
			break
		}
	}
	if parsed == nil {
		return ports.GeneratedItinerary{}, fmt.Errorf("claude: response contained no propose_itinerary tool call")
	}

	// Defense in depth: even though the system prompt instructs Claude to
	// only use real candidate IDs, this is the one place that actually
	// enforces it -- a prompt is not a validation layer.
	suggestionCount := 0
	stops := make([]ports.GeneratedStop, 0, len(parsed.Stops))
	for _, s := range parsed.Stops {
		if s.IsSuggestion {
			suggestionCount++
			if suggestionCount > 1 {
				return ports.GeneratedItinerary{}, fmt.Errorf("claude: more than one suggestion stop is not allowed")
			}
		} else if !validPlaceIDs[s.PlaceID] {
			return ports.GeneratedItinerary{}, fmt.Errorf("claude: place id %q is not in the candidate list", s.PlaceID)
		}
		stops = append(stops, ports.GeneratedStop{
			IsSuggestion:      s.IsSuggestion,
			PlaceID:           s.PlaceID,
			Label:             s.Label,
			TimeOnSiteMinutes: s.TimeOnSiteMinutes,
			WalkToNextMinutes: s.WalkToNextMinutes,
		})
	}

	return ports.GeneratedItinerary{Title: parsed.Title, Stops: stops}, nil
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
```

- [ ] **Step 5: Confirm `*ItineraryGenerator` satisfies `ports.ItineraryGenerator`**

```go
// append to internal/adapters/claude/itinerary_generator_test.go
var _ ports.ItineraryGenerator = (*ItineraryGenerator)(nil)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/claude/... -v`
Expected: PASS (all 5 cases).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/adapters/claude/itinerary_generator.go internal/adapters/claude/itinerary_generator_test.go
git commit -m "claude: ItineraryGenerator adapter, validates every place id against the candidate list"
```

---

### Task 5: Application use cases

**Files:**
- Create: `internal/application/generate_itinerary.go`
- Test: `internal/application/generate_itinerary_test.go`

**Interfaces:**
- Consumes: `ports.ItineraryRepository`, `ports.ItineraryGenerator`, `ports.CandidatePlace`,
  `ports.GeneratedItinerary` (Task 2), `domain.Place`, `domain.NewPlaceStop`, `domain.NewSuggestionStop`,
  `domain.NewItinerary`, `domain.NewItineraryTitle` (Task 1).
- Produces: `application.GenerateItinerary(ctx, generator, repo, userID, requestText string, places
  []*domain.Place) (*domain.Itinerary, error)`, `application.ListItineraries(ctx, repo, userID string)
  ([]*domain.Itinerary, error)`, `application.GetItinerary(ctx, repo, id string) (*domain.Itinerary,
  error)` — consumed by Task 6 (HTTP handlers).

- [ ] **Step 1: Write the failing tests**

```go
// internal/application/generate_itinerary_test.go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

type fakeItineraryRepo struct {
	saved       *domain.Itinerary
	byID        map[string]*domain.Itinerary
	byUserID    map[string][]*domain.Itinerary
	saveErr     error
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/application/... -run 'TestGenerateItinerary|TestListItineraries|TestGetItinerary' -v`
Expected: FAIL — `GenerateItinerary`/`ListItineraries`/`GetItinerary` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/application/generate_itinerary.go
package application

import (
	"context"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// GenerateItinerary turns every candidate place into the minimal shape the
// generator port needs, calls it, then rebuilds the result as real,
// validated domain.ItineraryStop values -- this is the one place a
// generator's raw output either becomes a trustworthy domain object or gets
// rejected; nothing downstream ever sees an unvalidated stop.
func GenerateItinerary(ctx context.Context, generator ports.ItineraryGenerator, repo ports.ItineraryRepository, userID, requestText string, places []*domain.Place) (*domain.Itinerary, error) {
	candidates := make([]ports.CandidatePlace, len(places))
	for i, p := range places {
		candidates[i] = ports.CandidatePlace{
			ID:       p.ID(),
			Name:     p.Name().String(),
			Category: p.Category(),
			Lat:      p.Coordinates().Lat(),
			Lon:      p.Coordinates().Lon(),
		}
	}

	generated, err := generator.Generate(ctx, requestText, candidates)
	if err != nil {
		return nil, err
	}

	title, err := domain.NewItineraryTitle(generated.Title)
	if err != nil {
		return nil, err
	}

	stops := make([]domain.ItineraryStop, 0, len(generated.Stops))
	for _, s := range generated.Stops {
		if s.IsSuggestion {
			stop, err := domain.NewSuggestionStop(s.Label, s.WalkToNextMinutes)
			if err != nil {
				return nil, err
			}
			stops = append(stops, stop)
			continue
		}
		stop, err := domain.NewPlaceStop(s.PlaceID, s.Label, s.TimeOnSiteMinutes, s.WalkToNextMinutes)
		if err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}

	itinerary, err := domain.NewItinerary(userID, title, stops)
	if err != nil {
		return nil, err
	}
	if err := repo.Save(ctx, itinerary); err != nil {
		return nil, err
	}
	return itinerary, nil
}

func ListItineraries(ctx context.Context, repo ports.ItineraryRepository, userID string) ([]*domain.Itinerary, error) {
	return repo.FindByUserID(ctx, userID)
}

func GetItinerary(ctx context.Context, repo ports.ItineraryRepository, id string) (*domain.Itinerary, error) {
	return repo.FindByID(ctx, id)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/... -v`
Expected: PASS — every test in the package (new ones plus every pre-existing application test).

- [ ] **Step 5: Commit**

```bash
git add internal/application/generate_itinerary.go internal/application/generate_itinerary_test.go
git commit -m "application: GenerateItinerary/ListItineraries/GetItinerary use cases"
```

---

### Task 6: HTTP routes + wiring

**Files:**
- Create: `internal/adapters/http/itinerary_handler.go`
- Test: `internal/adapters/http/itinerary_handler_test.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `cmd/api/main.go`

**Interfaces:**
- Consumes: `application.GenerateItinerary`/`ListItineraries`/`GetItinerary` (Task 5),
  `ports.ItineraryRepository`/`ports.ItineraryGenerator` (Task 2), `postgres.NewItineraryRepository`
  (Task 3), `claude.NewItineraryGenerator` (Task 4).
- Produces: `POST /itineraries`, `GET /itineraries`, `GET /itineraries/:id` — no mobile consumer yet
  (a separate, following plan).

- [ ] **Step 1: Wire the new port into `Server`**

In `internal/adapters/http/server.go`, add a field and constructor parameter (append, don't reorder the
existing ones — every existing call site of `NewServer` would otherwise need reordering too):

```go
type Server struct {
	echo           *echo.Echo
	placeRepo      ports.PlaceRepository
	scriptRepo     ports.ScriptRepository
	audioFileRepo  ports.AudioFileRepository
	userRepo       ports.UserRepository
	itineraryRepo  ports.ItineraryRepository
	publisher      ports.AudioJobPublisher
	storage        ports.AudioStorage
	cache          ports.Cache
	tokens         ports.TokenIssuer
	generator      ports.ItineraryGenerator
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, userRepo ports.UserRepository, itineraryRepo ports.ItineraryRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache, tokens ports.TokenIssuer, generator ports.ItineraryGenerator) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		userRepo:      userRepo,
		itineraryRepo: itineraryRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
		tokens:        tokens,
		generator:     generator,
	}
	s.echo.Use(middleware.CORS())

	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id", s.getPlaceDetail)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.GET("/cities/:city/manifest", s.getCityManifest)

	auth := requireAuth(s.tokens)
	adminOnly := requireRole(domain.RoleAdmin)
	s.echo.POST("/scripts/:id/review", s.reviewScript, auth, adminOnly)
	s.echo.POST("/audio-files/:id/retry", s.retryAudio, auth, adminOnly)

	s.echo.POST("/register", s.registerUser)
	s.echo.POST("/login", s.login)
	s.echo.POST("/logout", s.logout, auth)
	s.echo.PATCH("/me", s.updateMe, auth)
	s.echo.DELETE("/me", s.deleteMe, auth)

	// Itineraries are always tied to the caller's own account (contextUserID),
	// never a client-supplied user ID -- same reasoning as updateMe/deleteMe.
	s.echo.POST("/itineraries", s.createItinerary, auth)
	s.echo.GET("/itineraries", s.listItineraries, auth)
	s.echo.GET("/itineraries/:id", s.getItinerary, auth)
	return s
}
```

This changes `NewServer`'s signature (inserting `itineraryRepo` after `userRepo`, appending `generator`
at the end) — every existing call site needs updating, which Step 4 below handles for `cmd/api/main.go`;
if any test file constructs `NewServer` directly (several do, per the existing `audio_handler_test.go`
pattern seen earlier in this project), update each to pass a fake `itineraryRepo`/`generator` too. Grep
for `NewServer(` across `internal/adapters/http/*_test.go` and fix every call site — do not skip one
because its test doesn't touch itineraries; a mismatched argument count fails to compile, which fails
the whole package's tests, not just the itinerary-related ones.

- [ ] **Step 2: Write the failing tests**

```go
// internal/adapters/http/itinerary_handler_test.go
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		return nil, errNotFound
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

func TestCreateItinerary_RequiresAuth(t *testing.T) {
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), &fakeItineraryRepoHTTP{}, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{})

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
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, generator)

	body, _ := json.Marshal(map[string]string{"request": "1h à Santa Teresa"})
	req := httptest.NewRequest(http.MethodPost, "/itineraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("got status %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if itineraryRepo.saved == nil {
		t.Fatal("expected the generated itinerary to have been saved")
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
	server := NewServer(&fakePlaceRepo{}, &fakeScriptRepo{}, &fakeAudioFileRepo{}, newFakeUserRepo(), itineraryRepo, &fakePublisher{}, fakeAudioStorage{}, newFakeCache(), fakeTokenIssuer{}, &fakeGeneratorHTTP{})

	req := httptest.NewRequest(http.MethodGet, "/itineraries", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
```

**Read this project's existing `internal/adapters/http/*_test.go` fixtures first** (`fakePlaceRepo`,
`fakeScriptRepo`, `fakeAudioFileRepo`, `newFakeUserRepo`, `fakePublisher`, `fakeAudioStorage`,
`newFakeCache`, `fakeTokenIssuer`, and specifically how `fakeTokenIssuer.Verify` resolves
`"Bearer valid-token"` to a user ID, and `testPlace`/`errNotFound` if either already exists under a
different name) before writing this file — reuse the exact existing fixtures and helper names rather
than redefining ones that already exist elsewhere in this package; the snippets above assume
`fakeTokenIssuer{}` resolves any bearer token to a fixed test user ID (`"test-user-id"` above) the same
way it already does for every other authenticated-route test in this package. If a name above collides
with an existing fixture, use the existing one instead of introducing a duplicate.

- [ ] **Step 3: Write the implementation**

```go
// internal/adapters/http/itinerary_handler.go
package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

// Rio de Janeiro's bounding box -- same constants already defined in
// places_handler.go, reused here (same package) rather than duplicated.
// Every itinerary candidate comes from this same box, matching how every
// other place-listing route in this API already scopes itself to Rio.

type createItineraryRequest struct {
	Request string `json:"request"`
}

type itineraryStopResponse struct {
	Kind              string `json:"kind"`
	PlaceID           string `json:"place_id,omitempty"`
	Label             string `json:"label"`
	TimeOnSiteMinutes int    `json:"time_on_site_minutes"`
	WalkToNextMinutes int    `json:"walk_to_next_minutes"`
}

type itineraryResponse struct {
	ID           string                   `json:"id"`
	Title        string                   `json:"title"`
	TotalMinutes int                      `json:"total_minutes"`
	PlaceCount   int                      `json:"place_count"`
	Stops        []itineraryStopResponse  `json:"stops"`
}

func toItineraryResponse(it *domain.Itinerary) itineraryResponse {
	stops := make([]itineraryStopResponse, len(it.Stops()))
	for i, s := range it.Stops() {
		stops[i] = itineraryStopResponse{
			Kind:              string(s.Kind()),
			PlaceID:           s.PlaceID(),
			Label:             s.Label(),
			TimeOnSiteMinutes: s.TimeOnSiteMinutes(),
			WalkToNextMinutes: s.WalkToNextMinutes(),
		}
	}
	return itineraryResponse{
		ID:           it.ID(),
		Title:        it.Title().String(),
		TotalMinutes: it.TotalMinutes(),
		PlaceCount:   it.PlaceCount(),
		Stops:        stops,
	}
}

func (s *Server) createItinerary(c echo.Context) error {
	var req createItineraryRequest
	if err := c.Bind(&req); err != nil || req.Request == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"request\" field is required"})
	}

	places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	itinerary, err := application.GenerateItinerary(c.Request().Context(), s.generator, s.itineraryRepo, contextUserID(c), req.Request, places)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, toItineraryResponse(itinerary))
}

func (s *Server) listItineraries(c echo.Context) error {
	itineraries, err := application.ListItineraries(c.Request().Context(), s.itineraryRepo, contextUserID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	responses := make([]itineraryResponse, len(itineraries))
	for i, it := range itineraries {
		responses[i] = toItineraryResponse(it)
	}
	return c.JSON(http.StatusOK, responses)
}

func (s *Server) getItinerary(c echo.Context) error {
	itinerary, err := application.GetItinerary(c.Request().Context(), s.itineraryRepo, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "itinerary not found"})
	}
	// An itinerary belongs to exactly the user who generated it -- this is
	// not a public route (unlike GET /places), so a caller fetching by ID
	// must own it, checked here rather than trusting the URL alone.
	if itinerary.UserID() != contextUserID(c) {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "itinerary not found"})
	}
	return c.JSON(http.StatusOK, toItineraryResponse(itinerary))
}
```

- [ ] **Step 4: Wire into `cmd/api/main.go`**

Add the repository and generator to the composition root:

```go
	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	itineraryRepo := postgres.NewItineraryRepository(pool)
```

and, alongside the existing `tokens`/`publisher` setup (needs a new import,
`"rioaudioguide/backend/internal/adapters/claude"`, and the SDK's own client import,
`"github.com/anthropics/anthropic-sdk-go"`):

```go
	// ANTHROPIC_API_KEY is read directly by anthropic.NewClient() from the
	// environment -- no custom env var plumbing needed, same as every other
	// SDK-standard credential this project doesn't re-wrap.
	anthropicClient := anthropic.NewClient()
	itineraryGenerator := claude.NewItineraryGenerator(anthropicClient.Messages)
```

Update the `NewServer` call to match its new signature:

```go
	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, userRepo, itineraryRepo, publisher, storage, cache, tokens, itineraryGenerator)
```

- [ ] **Step 5: Run the full test suite**

Run: `go build ./...` then `go test ./...`
Expected: builds cleanly, every package's tests pass, including every pre-existing `http` package test
whose `NewServer(...)` call site you updated in Step 1.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/itinerary_handler.go internal/adapters/http/itinerary_handler_test.go internal/adapters/http/server.go cmd/api/main.go
git commit -m "http: POST/GET /itineraries, wired to Claude generation"
```

---

## After this plan

Not covered here (explicitly out of scope per the spec):
- The mobile app (three new screens, navigation entry points) — a separate, following plan, built
  against this plan's HTTP contract once it exists.
- Community/shared itineraries, broader ungrounded recommendations, manual editing/reordering —
  deferred per the design spec.
- Prompt/effort tuning for the Claude call once real generations exist to evaluate against.
