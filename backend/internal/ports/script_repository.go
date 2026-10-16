package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type ScriptRepository interface {
	Save(ctx context.Context, script *domain.Script) error
	FindByID(ctx context.Context, id string) (*domain.Script, error)
	FindByPlaceIDAndLanguage(ctx context.Context, placeID string, language string) (*domain.Script, error)
	FindByPlaceID(ctx context.Context, placeID string) ([]*domain.Script, error)
	// FindAllDraft returns every script still in ScriptStatusDraft, across
	// every place/language -- the backlog cmd/batchreview works through.
	// No pagination: the realistic scale here is hundreds, not millions.
	FindAllDraft(ctx context.Context) ([]*domain.Script, error)
}
