package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
)

type ScriptRepository struct {
	db DBTX
}

func NewScriptRepository(db DBTX) *ScriptRepository {
	return &ScriptRepository{db: db}
}

const upsertScriptSQL = `
	INSERT INTO scripts (id, place_id, language, text, source_text, status, reviewer, reviewed_at, published_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT (id) DO UPDATE SET
		text = EXCLUDED.text,
		status = EXCLUDED.status,
		reviewer = EXCLUDED.reviewer,
		reviewed_at = EXCLUDED.reviewed_at,
		published_at = EXCLUDED.published_at,
		updated_at = now()
`

func scriptSaveArgs(script *domain.Script) []any {
	var reviewedAt, publishedAt any
	if !script.ReviewedAt().IsZero() {
		reviewedAt = script.ReviewedAt()
	}
	if !script.PublishedAt().IsZero() {
		publishedAt = script.PublishedAt()
	}
	return []any{
		script.ID(), script.PlaceID(), script.Language().String(), script.Text().String(), script.SourceText(),
		string(script.Status()), script.Reviewer(), reviewedAt, publishedAt,
	}
}

func (r *ScriptRepository) Save(ctx context.Context, script *domain.Script) error {
	_, err := r.db.Exec(ctx, upsertScriptSQL, scriptSaveArgs(script)...)
	return err
}

// QueueSave adds this script's upsert to a shared pgx.Batch instead of
// executing it immediately -- same calling pattern as
// PlaceRepository.QueueSave, see there for details.
func (r *ScriptRepository) QueueSave(batch *pgx.Batch, script *domain.Script) {
	batch.Queue(upsertScriptSQL, scriptSaveArgs(script)...)
}

// FindExistingLanguages returns, for each of the given place IDs, the set of
// languages that already have a script -- in one round trip. Used by
// cmd/import to filter out scripts that would otherwise trip the
// (place_id, language) unique constraint: catching that violation per row
// would abort a shared import transaction on what is actually an expected,
// non-error condition (re-running the import over already-imported places).
func (r *ScriptRepository) FindExistingLanguages(ctx context.Context, placeIDs []string) (map[string]map[string]bool, error) {
	rows, err := r.db.Query(ctx, `
		SELECT place_id, language FROM scripts WHERE place_id = ANY($1)
	`, placeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := make(map[string]map[string]bool, len(placeIDs))
	for rows.Next() {
		var placeID, language string
		if err := rows.Scan(&placeID, &language); err != nil {
			return nil, err
		}
		if existing[placeID] == nil {
			existing[placeID] = make(map[string]bool)
		}
		existing[placeID][language] = true
	}
	return existing, rows.Err()
}

func (r *ScriptRepository) FindByID(ctx context.Context, id string) (*domain.Script, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, place_id, language, text, COALESCE(source_text, ''), status,
		       COALESCE(reviewer, ''), reviewed_at, published_at
		FROM scripts WHERE id = $1
	`, id)

	var scriptID, placeID, languageRaw, textRaw, sourceText, status, reviewer string
	var reviewedAt, publishedAt *time.Time
	if err := row.Scan(&scriptID, &placeID, &languageRaw, &textRaw, &sourceText, &status,
		&reviewer, &reviewedAt, &publishedAt); err != nil {
		return nil, err
	}

	language, err := domain.NewLanguage(languageRaw)
	if err != nil {
		return nil, err
	}
	text, err := domain.NewScriptText(textRaw)
	if err != nil {
		return nil, err
	}

	var reviewedAtVal, publishedAtVal time.Time
	if reviewedAt != nil {
		reviewedAtVal = *reviewedAt
	}
	if publishedAt != nil {
		publishedAtVal = *publishedAt
	}

	return domain.ReconstructScript(scriptID, placeID, language, text, sourceText, domain.ScriptStatus(status),
		reviewer, reviewedAtVal, publishedAtVal), nil
}

func (r *ScriptRepository) FindByPlaceIDAndLanguage(ctx context.Context, placeID, language string) (*domain.Script, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, place_id, language, text, COALESCE(source_text, ''), status,
		       COALESCE(reviewer, ''), reviewed_at, published_at
		FROM scripts WHERE place_id = $1 AND language = $2
	`, placeID, language)

	var scriptID, placeIDCol, languageRaw, textRaw, sourceText, status, reviewer string
	var reviewedAt, publishedAt *time.Time
	if err := row.Scan(&scriptID, &placeIDCol, &languageRaw, &textRaw, &sourceText, &status,
		&reviewer, &reviewedAt, &publishedAt); err != nil {
		return nil, err
	}

	language2, err := domain.NewLanguage(languageRaw)
	if err != nil {
		return nil, err
	}
	text, err := domain.NewScriptText(textRaw)
	if err != nil {
		return nil, err
	}

	var reviewedAtVal, publishedAtVal time.Time
	if reviewedAt != nil {
		reviewedAtVal = *reviewedAt
	}
	if publishedAt != nil {
		publishedAtVal = *publishedAt
	}

	return domain.ReconstructScript(scriptID, placeIDCol, language2, text, sourceText, domain.ScriptStatus(status),
		reviewer, reviewedAtVal, publishedAtVal), nil
}
