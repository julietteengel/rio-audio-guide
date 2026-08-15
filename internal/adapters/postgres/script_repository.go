package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type ScriptRepository struct {
	pool *pgxpool.Pool
}

func NewScriptRepository(pool *pgxpool.Pool) *ScriptRepository {
	return &ScriptRepository{pool: pool}
}

func (r *ScriptRepository) Save(ctx context.Context, script *domain.Script) error {
	var reviewedAt, publishedAt any
	if !script.ReviewedAt().IsZero() {
		reviewedAt = script.ReviewedAt()
	}
	if !script.PublishedAt().IsZero() {
		publishedAt = script.PublishedAt()
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO scripts (id, place_id, language, text, source_text, status, reviewer, reviewed_at, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			text = EXCLUDED.text,
			status = EXCLUDED.status,
			reviewer = EXCLUDED.reviewer,
			reviewed_at = EXCLUDED.reviewed_at,
			published_at = EXCLUDED.published_at,
			updated_at = now()
	`, script.ID(), script.PlaceID(), script.Language().String(), script.Text().String(), script.SourceText(),
		string(script.Status()), script.Reviewer(), reviewedAt, publishedAt)
	return err
}

func (r *ScriptRepository) FindByID(ctx context.Context, id string) (*domain.Script, error) {
	row := r.pool.QueryRow(ctx, `
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
