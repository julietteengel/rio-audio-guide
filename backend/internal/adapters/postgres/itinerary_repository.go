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
	INSERT INTO itineraries (id, user_id, title, created_at, is_featured)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, is_featured = EXCLUDED.is_featured
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

	if _, err := tx.Exec(ctx, upsertItinerarySQL, itinerary.ID(), itinerary.UserID(), itinerary.Title().String(), itinerary.CreatedAt(), itinerary.IsFeatured()); err != nil {
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

const selectItineraryByIDSQL = `SELECT id, user_id, title, created_at, is_featured FROM itineraries WHERE id = $1`

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
		isFeatured               bool
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
