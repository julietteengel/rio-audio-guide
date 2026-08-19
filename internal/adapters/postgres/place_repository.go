package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
)

type PlaceRepository struct {
	db DBTX
}

func NewPlaceRepository(db DBTX) *PlaceRepository {
	return &PlaceRepository{db: db}
}

const upsertPlaceSQL = `
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
`

func placeSaveArgs(place *domain.Place) []any {
	return []any{
		place.ID(), place.Name().String(), place.Category(), place.Coordinates().Lon(), place.Coordinates().Lat(),
		place.WikidataQID().String(), place.Source(), place.SourceRichness(), string(place.Status()), place.RemovedReason(),
	}
}

func (r *PlaceRepository) Save(ctx context.Context, place *domain.Place) error {
	_, err := r.db.Exec(ctx, upsertPlaceSQL, placeSaveArgs(place)...)
	return err
}

// QueueSave adds this place's upsert to a shared pgx.Batch instead of
// executing it immediately. The caller must send the batch (db.SendBatch)
// and then call BatchResults.Exec() once per queued place, in the same
// order they were queued, to read back each row's result -- see cmd/import
// for the calling pattern. Used to import many places in one pipelined
// round trip instead of one round trip per place.
func (r *PlaceRepository) QueueSave(batch *pgx.Batch, place *domain.Place) {
	batch.Queue(upsertPlaceSQL, placeSaveArgs(place)...)
}

// FindIDsByNames resolves existing active places by name in a single round
// trip, returning a name -> id map. Used by cmd/import so importing hundreds
// of places doesn't cost one FindByName query per candidate.
func (r *PlaceRepository) FindIDsByNames(ctx context.Context, names []string) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (name) name, id FROM places
		WHERE name = ANY($1) AND status = 'active'
		ORDER BY name, created_at
	`, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]string, len(names))
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, rows.Err()
}

func (r *PlaceRepository) FindByID(ctx context.Context, id string) (*domain.Place, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places WHERE id = $1
	`, id)
	return scanPlace(row)
}

// FindByName sert surtout à l'import CSV (cmd/import) pour éviter de recréer un
// lieu déjà présent. places.name n'a pas de contrainte d'unicité : on filtre
// donc les lieux retirés et on ordonne par created_at pour que deux appels
// identiques renvoient toujours la même ligne.
func (r *PlaceRepository) FindByName(ctx context.Context, name string) (*domain.Place, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places WHERE name = $1 AND status = 'active' ORDER BY created_at LIMIT 1
	`, name)
	return scanPlace(row)
}

func (r *PlaceRepository) FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error) {
	rows, err := r.db.Query(ctx, `
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
