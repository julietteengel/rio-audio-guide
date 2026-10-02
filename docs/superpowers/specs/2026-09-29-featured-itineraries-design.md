# Featured itineraries

## Why

Itineraries today are always user-generated (via a free-text request to the LLM-driven assistant),
always owned by exactly one account, and only ever visible to that account (`GET /itineraries`,
`GET /itineraries/:id` both require `auth` and check `UserID() == contextUserID(c)`). There is no way
to publish a curated, hand-picked itinerary (e.g. "Roteiro do Rio Colonial") that every user — including
someone who hasn't created an account yet — can browse and follow.

## Scope

In scope: an admin-authored, publicly-viewable itinerary that reuses the existing `Itinerary` domain
entity and stop model, discoverable from a new section on the mobile app's map/home screen, without
requiring a logged-in session to view.

Out of scope (explicitly deferred, decided during brainstorming): letting a user "adopt" a featured
itinerary into their own personal list (view/follow only, for now — no cloning); creating a featured
itinerary via the LLM assistant (admin specifies the exact ordered place list directly, no generation
step); any UI for browsing featured itineraries from inside the existing "My Itineraries" screen (they
live in their own new discovery section instead); un-featuring one once created (no `DELETE`/`PATCH`
route — `MarkFeatured()` is one-directional at the domain level; removing or editing one, if ever
needed, is a direct-database operation for now, the same way local dev data has been managed all
session — not worth a dedicated endpoint for founder-curated content at this scale).

## Data model

One new column on the existing `itineraries` table:

```sql
ALTER TABLE itineraries
  ADD COLUMN IF NOT EXISTS is_featured BOOLEAN NOT NULL DEFAULT false;
```

No changes to `itinerary_stops` — a featured itinerary's stops are the same `ItineraryStop` value objects
every other itinerary already uses (place stops with a real `place_id`; the existing suggestion-stop
slot remains available for a featured itinerary too, capped at one per itinerary by the same domain rule
already in place).

## Domain (`internal/domain/itinerary.go`)

- `Itinerary` gains an `isFeatured bool` field and an `IsFeatured() bool` accessor.
- `NewItinerary`'s signature is unchanged (still `userID string, title ItineraryTitle, stops
  []ItineraryStop`) — a freshly-created itinerary always starts `isFeatured: false`, matching how every
  other boolean flag in this codebase starts false at construction (`User.emailVerified`,
  `Script.status` starting at `draft`). Marking one featured is a separate, explicit step (see
  Application section below), not a constructor parameter — the vast majority of itineraries created
  through the existing assistant flow are never featured, so defaulting to `false` and flipping it
  explicitly keeps the common path unchanged.
- New `func (i *Itinerary) MarkFeatured()` — no validation, no error return (unlike
  `User.MarkEmailVerified()`, there's no "deleted" concept for itineraries to guard against). Idempotent
  by construction (setting a bool to `true` twice is a no-op).
- `ReconstructItinerary`'s signature gains a 6th parameter: `ReconstructItinerary(id, userID string,
  title ItineraryTitle, stops []ItineraryStop, createdAt time.Time, isFeatured bool) *Itinerary` — same
  convention as every other Reconstruct* function in this codebase (preserve given data, don't
  revalidate).

## Ports (`internal/ports/itinerary_repository.go`)

`ItineraryRepository` gains one new method:

```go
// FindFeatured returns every itinerary with IsFeatured() true, in the same
// order as FindByUserID (most recently created first) -- there is no
// pagination yet because the founder curates these by hand; a few dozen at
// most is the realistic scale for the foreseeable future.
FindFeatured(ctx context.Context) ([]*domain.Itinerary, error)
```

## Application (`internal/application/`)

- `Save`'s call sites (`GenerateItinerary` in `generate_itinerary.go`) are unaffected — they still
  construct via `domain.NewItinerary`, which defaults `isFeatured` to `false`, and `Save` persists
  whatever the entity's current state is.
- New file `create_featured_itinerary.go`:

  ```go
  package application

  import (
      "context"
      "errors"
      "fmt"

      "rioaudioguide/backend/internal/domain"
      "rioaudioguide/backend/internal/ports"
  )

  // ErrFeaturedStopPlaceNotFound is returned when a stop in the request names
  // a place_id that doesn't exist (or isn't active) -- a featured itinerary
  // is hand-curated by an admin, but the actual place lookup still goes
  // through the real repository rather than trusting the given ID blindly,
  // same "never trust an ID that merely looks real" posture
  // GenerateItinerary already applies to LLM-generated stops.
  var ErrFeaturedStopPlaceNotFound = errors.New("application: one of the featured itinerary's places was not found")

  type FeaturedStopInput struct {
      PlaceID           string
      TimeOnSiteMinutes int
      WalkToNextMinutes int
  }

  // CreateFeaturedItinerary builds each stop's label from the place's own
  // real name (via placeRepo), rather than accepting a label in the
  // request -- the admin picks which places and in what order, not a
  // second, possibly-inconsistent copy of each place's display name.
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

  (`ErrSaveFailed` is reused as-is from `generate_itinerary.go` — same package, same meaning: a
  persistence failure, not a rejection of the request.)

## Adapter (`internal/adapters/postgres/itinerary_repository.go`)

- `upsertItinerarySQL` gains `is_featured` in both the column list and the `ON CONFLICT` update clause;
  `Save` passes `itinerary.IsFeatured()` as a new bind parameter.
- `selectItineraryByIDSQL` and `selectItinerariesByUserIDSQL` both add `is_featured` to their `SELECT`
  list; `FindByID` and `FindByUserID` both scan it into a new `isFeatured bool` local and pass it to
  `ReconstructItinerary`.
- New `selectFeaturedItinerariesSQL` and `FindFeatured` method, mirroring `FindByUserID`'s shape exactly
  (`SELECT ... FROM itineraries WHERE is_featured = true ORDER BY created_at DESC`, then `loadStops` per
  row) — no new SQL pattern needed, `loadStops` is already itinerary-agnostic.

## HTTP (`internal/adapters/http/`)

Two new routes:

- `POST /featured-itineraries` — **admin-only** (`auth, adminOnly`, the same `requireRole(domain.RoleAdmin)`
  middleware already used for `POST /scripts/:id/review`). Body: `{title: string, stops: [{place_id:
  string, time_on_site_minutes: int, walk_to_next_minutes: int}]}`. On success, `201` with the same
  `itineraryResponse` shape `createItinerary` already returns (reused as-is — the wire shape doesn't
  change based on how an itinerary was created). On `ErrFeaturedStopPlaceNotFound`, `422`; on
  `ErrSaveFailed`, `500`; on any domain validation error (empty title, no stops, etc.), `422` with the
  error message, matching `createItinerary`'s existing fallback branch.
- `GET /featured-itineraries` — **public, no auth middleware at all** (the deliberate choice from
  brainstorming: a visitor without an account can see this, same posture as `GET /places`). Returns a
  JSON array using the same `itineraryResponse` shape, `200` always (an empty array if none are marked
  featured yet — not a 404, mirroring `listItineraries`'s existing behavior for a user with zero
  itineraries).

No changes to the three existing itinerary routes' handlers, middleware, or response shapes.

## Mobile (`mobile/src/`)

- `ItinerariesRepository.ts` gains a new function that does **not** take a token (unlike every existing
  function in this file):

  ```ts
  export async function listFeaturedItineraries(): Promise<Itinerary[]> {
    const res = await fetch(`${API_BASE_URL}/featured-itineraries`);
    if (!res.ok) throw new ItinerariesApiError("request failed", res.status);
    const wire = (await res.json()) as WireItinerary[];
    return wire.map(fromWire);
  }
  ```

  (Reuses the existing `WireItinerary`/`fromWire`/`Itinerary` types as-is — the wire shape is identical
  to a personal itinerary's.)

- `Map.tsx` gains a new "À découvrir" ("Discover") section — fetched via `listFeaturedItineraries()` on
  mount, rendered regardless of `isLoggedIn` (unlike anything else gated by `useAuth()` elsewhere in this
  screen). Tapping a card navigates to the new `FeaturedItineraryDetail` screen with the itinerary's `id`.
- New `FeaturedItineraryDetail.tsx` — a read-only sibling of `ItineraryDetail.tsx`: same visual layout
  (stop list, `formatDuration` usage, theme tokens), but fetches via `listFeaturedItineraries()` client-side
  filtered to the matching `id` on mount (no per-ID detail route exists server-side — the list response
  already carries full stop data, so a second network round-trip isn't needed; this screen receives the
  already-fetched itinerary object via route params instead of re-fetching by ID, avoiding both an extra
  request and the need for a `GET /featured-itineraries/:id` route this spec deliberately doesn't add).
  No `token` dependency anywhere in this screen — it must render correctly for a logged-out visitor.
- `navigation/types.ts`: `AppStackParamList` gains `FeaturedItineraryDetail: { itinerary: Itinerary }`
  (passing the full object through route params, per the point above — not just an id).
- `AppNavigator.tsx`: registers the new screen (not behind any auth gate — `Map`, `PlaceDetail` etc.
  already aren't, this one belongs alongside them, not alongside `Auth`/`VerifyEmail`'s modal group).
- `i18n/dictionary.ts`: new `map.discoverSectionTitle` (or similarly-named) key in the existing `auth`-adjacent
  top-level structure, all 4 locales — the exact copy is an implementation-time detail, not fixed here.

## Testing

- Domain: `MarkFeatured`/`IsFeatured` round-trip test, `ReconstructItinerary` preserving `isFeatured` in
  both the `true` and `false` case (mirrors `TestReconstructUser_PreservesEmailVerified`'s existing
  pattern in this codebase).
- Application: `CreateFeaturedItinerary` — happy path (real place IDs, stops built with the place's own
  name as label), unknown place ID → `ErrFeaturedStopPlaceNotFound`, empty title → domain error
  propagated. `ListFeaturedItineraries` — returns only itineraries with `isFeatured = true` (a fake
  repository test, not dependent on Postgres).
- Postgres integration: `FindFeatured` returns only featured itineraries, in `created_at DESC` order;
  `Save`/`FindByID` round-trip `is_featured` correctly in both states.
- HTTP: `POST /featured-itineraries` — `201` for an admin caller with valid stops, `403` for a
  non-admin caller (existing `adminOnly` middleware behavior, not new), `422` for an unknown place ID.
  `GET /featured-itineraries` — `200` with an empty array when none exist, `200` with the expected
  itinerary once one is created, and critically, a request with **no** `Authorization` header at all
  still succeeds (proves the route is genuinely unauthenticated, not just tolerant of a bad token).
- Mobile: `npx tsc --noEmit` clean; manual verification that the "À découvrir" section renders and is
  tappable while logged out (not just while logged in), since that is the entire point of this feature.
