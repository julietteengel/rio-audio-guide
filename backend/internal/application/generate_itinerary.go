package application

import (
	"context"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// GenerateItinerary turns every candidate place into the minimal shape the
// generator port needs, calls it, then rebuilds the result as real,
// validated domain.ItineraryStop values -- this is the one place a
// generator's raw output either becomes a trustworthy domain object or gets
// rejected; nothing downstream ever sees an unvalidated stop.
func GenerateItinerary(ctx context.Context, generator ports.ItineraryGenerator, repo ports.ItineraryRepository, userID, requestText string, places []*domain.Place) (*domain.Itinerary, error) {
	candidates := make([]ports.CandidatePlace, len(places))
	for i, p := range places {
		candidates[i] = ports.CandidatePlace{
			ID:       p.ID(),
			Name:     p.Name().String(),
			Category: p.Category(),
			Lat:      p.Coordinates().Lat(),
			Lon:      p.Coordinates().Lon(),
		}
	}

	generated, err := generator.Generate(ctx, requestText, candidates)
	if err != nil {
		return nil, err
	}

	title, err := domain.NewItineraryTitle(generated.Title)
	if err != nil {
		return nil, err
	}

	stops := make([]domain.ItineraryStop, 0, len(generated.Stops))
	for _, s := range generated.Stops {
		if s.IsSuggestion {
			stop, err := domain.NewSuggestionStop(s.Label, s.WalkToNextMinutes)
			if err != nil {
				return nil, err
			}
			stops = append(stops, stop)
			continue
		}
		stop, err := domain.NewPlaceStop(s.PlaceID, s.Label, s.TimeOnSiteMinutes, s.WalkToNextMinutes)
		if err != nil {
			return nil, err
		}
		stops = append(stops, stop)
	}

	itinerary, err := domain.NewItinerary(userID, title, stops)
	if err != nil {
		return nil, err
	}
	if err := repo.Save(ctx, itinerary); err != nil {
		return nil, err
	}
	return itinerary, nil
}

func ListItineraries(ctx context.Context, repo ports.ItineraryRepository, userID string) ([]*domain.Itinerary, error) {
	return repo.FindByUserID(ctx, userID)
}

func GetItinerary(ctx context.Context, repo ports.ItineraryRepository, id string) (*domain.Itinerary, error) {
	return repo.FindByID(ctx, id)
}
