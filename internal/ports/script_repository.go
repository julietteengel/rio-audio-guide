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
}
