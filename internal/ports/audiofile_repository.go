package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type AudioFileRepository interface {
	Save(ctx context.Context, audioFile *domain.AudioFile) error
	FindByID(ctx context.Context, id string) (*domain.AudioFile, error)
}
