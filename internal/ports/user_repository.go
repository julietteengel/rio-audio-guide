package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
}
