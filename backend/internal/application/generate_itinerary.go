package application

import (
	"context"
	"errors"
	"fmt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrNoCandidatePlaces is returned when GenerateItinerary has no candidate
// places to build from -- generating from zero real places could only ever
// produce a fully-ungrounded itinerary via the meal-break slot, which
// domain.NewItinerary rejects anyway (ErrItineraryNoPlaceStops); this fails
// fast with a clearer error before ever calling the LLM.
var ErrNoCandidatePlaces = errors.New("application: no candidate places available for itinerary generation")

// ErrGenerationFailed wraps any error from the generator port (the Claude
// API being down, rate-limited, or returning a malformed response) -- an
// upstream failure, not a rejection of the caller's request. HTTP handlers
// map errors.Is(err, ErrGenerationFailed) to a 5xx, not a 422 (see
// internal/adapters/http/itinerary_handler.go).
var ErrGenerationFailed = errors.New("application: itinerary generation failed")

// ErrSaveFailed wraps any error from the repository's Save call -- a
// persistence failure, not a rejection of the caller's request.
var ErrSaveFailed = errors.New("application: could not save itinerary")

// maxCandidatePlaces caps how many places are sent to the LLM per request.
// This is an interim safeguard against unbounded token cost, not a real
// fix -- a proper fix would filter candidates by neighborhood/proximity to
// what the request actually asks for, which is its own design decision for
// a future plan, not something to improvise here.
const maxCandidatePlaces = 300

// GenerateItinerary turns every candidate place into the minimal shape the
// generator port needs, calls it, then rebuilds the result as real,
// validated domain.ItineraryStop values -- this is the one place a
// generator's raw output either becomes a trustworthy domain object or gets
// rejected; nothing downstream ever sees an unvalidated stop. Every
// non-suggestion stop's place_id is checked against this call's own
// candidate set (validPlaceIDs below) -- a place_id from a different
// request, or one the generator invented outright, is rejected here, not
// trusted because it merely looks like a real ID.
func GenerateItinerary(ctx context.Context, generator ports.ItineraryGenerator, repo ports.ItineraryRepository, userID, requestText string, places []*domain.Place) (*domain.Itinerary, error) {
	if len(places) == 0 {
		return nil, ErrNoCandidatePlaces
	}
	if len(places) > maxCandidatePlaces {
		places = places[:maxCandidatePlaces]
	}

	candidates := make([]ports.CandidatePlace, len(places))
	validPlaceIDs := make(map[string]bool, len(places))
	for i, p := range places {
		candidates[i] = ports.CandidatePlace{
			ID:       p.ID(),
			Name:     p.Name().String(),
			Category: p.Category(),
			Lat:      p.Coordinates().Lat(),
			Lon:      p.Coordinates().Lon(),
		}
		validPlaceIDs[p.ID()] = true
	}

	generated, err := generator.Generate(ctx, requestText, candidates)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGenerationFailed, err)
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
		if !validPlaceIDs[s.PlaceID] {
			return nil, fmt.Errorf("application: place id %q from generator is not a known candidate", s.PlaceID)
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
		return nil, fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}
	return itinerary, nil
}

func ListItineraries(ctx context.Context, repo ports.ItineraryRepository, userID string) ([]*domain.Itinerary, error) {
	return repo.FindByUserID(ctx, userID)
}

func GetItinerary(ctx context.Context, repo ports.ItineraryRepository, id string) (*domain.Itinerary, error) {
	return repo.FindByID(ctx, id)
}
