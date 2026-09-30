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
	// FindFeatured returns every itinerary with IsFeatured() true, most
	// recently created first (same ordering as FindByUserID) -- no
	// pagination yet, the founder curates these by hand and a few dozen at
	// most is the realistic scale for the foreseeable future.
	FindFeatured(ctx context.Context) ([]*domain.Itinerary, error)
}
