package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type PlaceRepository struct {
	pool *pgxpool.Pool
}

func NewPlaceRepository(pool *pgxpool.Pool) *PlaceRepository {
	return &PlaceRepository{pool: pool}
}

func (r *PlaceRepository) Save(ctx context.Context, place *domain.Place) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO places (id, name, category, geom, wikidata_qid, source, source_richness, status, removed_reason)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			category = EXCLUDED.category,
			geom = EXCLUDED.geom,
			wikidata_qid = EXCLUDED.wikidata_qid,
			status = EXCLUDED.status,
			removed_reason = EXCLUDED.removed_reason,
			updated_at = now()
	`, place.ID(), place.Name().String(), place.Category(), place.Coordinates().Lon(), place.Coordinates().Lat(),
		place.WikidataQID().String(), place.Source(), place.SourceRichness(), string(place.Status()), place.RemovedReason())
	return err
}

func (r *PlaceRepository) FindByID(ctx context.Context, id string) (*domain.Place, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places WHERE id = $1
	`, id)
	return scanPlace(row)
}

func (r *PlaceRepository) FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places
		WHERE status = 'active'
		  AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)::geography
	`, minLon, minLat, maxLon, maxLat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var places []*domain.Place
	for rows.Next() {
		place, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		places = append(places, place)
	}
	return places, rows.Err()
}

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query) —
// lets FindByID and FindActiveInBoundingBox share the same scan logic.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanPlace(row rowScanner) (*domain.Place, error) {
	var id, nameRaw, category, wikidataRaw, source, sourceRichness, status, removedReason string
	var lat, lon float64
	if err := row.Scan(&id, &nameRaw, &category, &lat, &lon, &wikidataRaw, &source, &sourceRichness, &status, &removedReason); err != nil {
		return nil, err
	}

	name, err := domain.NewPlaceName(nameRaw)
	if err != nil {
		return nil, err
	}
	coords, err := domain.NewCoordinates(lat, lon)
	if err != nil {
		return nil, err
	}
	qid, err := domain.NewWikidataQID(wikidataRaw)
	if err != nil {
		return nil, err
	}

	return domain.ReconstructPlace(id, name, category, coords, qid, source, sourceRichness, domain.PlaceStatus(status), removedReason), nil
}
