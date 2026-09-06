// internal/ports/itinerary_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type ItineraryRepository interface {
	Save(ctx context.Context, itinerary *domain.Itinerary) error
	FindByID(ctx context.Context, id string) (*domain.Itinerary, error)
	FindByUserID(ctx context.Context, userID string) ([]*domain.Itinerary, error)
}
