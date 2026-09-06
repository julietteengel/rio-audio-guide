// internal/ports/itinerary_generator.go
package ports

import "context"

// CandidatePlace is the minimal, real place data the generator may choose
// from -- never richer than what's actually in this app's grounded
// database, so nothing the LLM returns can reference a place that doesn't
// exist here. Built from domain.Place by internal/application, not by this
// package (ports must not depend on how adapters construct their inputs).
type CandidatePlace struct {
	ID       string
	Name     string
	Category string
	Lat, Lon float64
}

// GeneratedStop mirrors domain.ItineraryStop's two kinds without importing
// domain -- ports depend on domain for repository return types, but a
// generator's raw output hasn't been validated into real domain.ItineraryStop
// values yet (internal/application does that, via domain.NewPlaceStop/
// domain.NewSuggestionStop, which is also where invalid output from the LLM
// gets rejected rather than silently trusted).
type GeneratedStop struct {
	IsSuggestion      bool   // true only for the one allowed meal-break-style slot
	PlaceID           string // set when !IsSuggestion; must be one of the CandidatePlace IDs given to Generate
	Label             string
	TimeOnSiteMinutes int // ignored when IsSuggestion
	WalkToNextMinutes int
}

type GeneratedItinerary struct {
	Title string
	Stops []GeneratedStop
}

// ItineraryGenerator is the outbound port to an LLM -- implemented by
// internal/adapters/claude. request is the user's free-text description;
// candidates is every place the generator is allowed to choose from.
type ItineraryGenerator interface {
	Generate(ctx context.Context, request string, candidates []CandidatePlace) (GeneratedItinerary, error)
}
