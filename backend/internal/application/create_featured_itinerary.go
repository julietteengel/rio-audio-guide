package application

import (
	"context"
	"errors"
	"fmt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrFeaturedStopPlaceNotFound is returned when a stop names a place_id that
// doesn't exist -- a featured itinerary is hand-curated by an admin, but the
// actual place lookup still goes through the real repository rather than
// trusting the given ID blindly, same "never trust an ID that merely looks
// real" posture GenerateItinerary already applies to LLM-generated stops.
var ErrFeaturedStopPlaceNotFound = errors.New("application: one of the featured itinerary's places was not found")

type FeaturedStopInput struct {
	PlaceID           string
	TimeOnSiteMinutes int
	WalkToNextMinutes int
}

// CreateFeaturedItinerary builds each stop's label from the place's own real
// name (via placeRepo), rather than accepting a label in the request -- the
// admin picks which places and in what order, not a second, possibly-
// inconsistent copy of each place's display name.
func CreateFeaturedItinerary(ctx context.Context, placeRepo ports.PlaceRepository, itineraryRepo ports.ItineraryRepository, adminUserID, titleStr string, stopInputs []FeaturedStopInput) (*domain.Itinerary, error) {
	title, err := domain.NewItineraryTitle(titleStr)
	if err != nil {
		return nil, err
	}

	stops := make([]domain.ItineraryStop, 0, len(stopInputs))
	for _, in := range stopInputs {
		place, err := placeRepo.FindByID(ctx, in.PlaceID)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrFeaturedStopPlaceNotFound, in.PlaceID)
		}
		stop, err := domain.NewPlaceStop(place.ID(), place.Name().String(), in.TimeOnSiteMinutes, in.WalkToNextMinutes)
		if err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}

	itinerary, err := domain.NewItinerary(adminUserID, title, stops)
	if err != nil {
		return nil, err
	}
	itinerary.MarkFeatured()
	if err := itineraryRepo.Save(ctx, itinerary); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}
	return itinerary, nil
}

func ListFeaturedItineraries(ctx context.Context, repo ports.ItineraryRepository) ([]*domain.Itinerary, error) {
	return repo.FindFeatured(ctx)
}
