// internal/ports/place_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type PlaceRepository interface {
	Save(ctx context.Context, place *domain.Place) error
	FindByID(ctx context.Context, id string) (*domain.Place, error)
	FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error)
}
