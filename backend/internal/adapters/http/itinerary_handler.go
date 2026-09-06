package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

// Rio de Janeiro's bounding box -- same constants already defined in
// places_handler.go, reused here (same package) rather than duplicated.
// Every itinerary candidate comes from this same box, matching how every
// other place-listing route in this API already scopes itself to Rio.

type createItineraryRequest struct {
	Request string `json:"request"`
}

type itineraryStopResponse struct {
	Kind              string `json:"kind"`
	PlaceID           string `json:"place_id,omitempty"`
	Label             string `json:"label"`
	TimeOnSiteMinutes int    `json:"time_on_site_minutes"`
	WalkToNextMinutes int    `json:"walk_to_next_minutes"`
}

type itineraryResponse struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	TotalMinutes int                     `json:"total_minutes"`
	PlaceCount   int                     `json:"place_count"`
	Stops        []itineraryStopResponse `json:"stops"`
}

func toItineraryResponse(it *domain.Itinerary) itineraryResponse {
	stops := make([]itineraryStopResponse, len(it.Stops()))
	for i, s := range it.Stops() {
		stops[i] = itineraryStopResponse{
			Kind:              string(s.Kind()),
			PlaceID:           s.PlaceID(),
			Label:             s.Label(),
			TimeOnSiteMinutes: s.TimeOnSiteMinutes(),
			WalkToNextMinutes: s.WalkToNextMinutes(),
		}
	}
	return itineraryResponse{
		ID:           it.ID(),
		Title:        it.Title().String(),
		TotalMinutes: it.TotalMinutes(),
		PlaceCount:   it.PlaceCount(),
		Stops:        stops,
	}
}

func (s *Server) createItinerary(c echo.Context) error {
	var req createItineraryRequest
	if err := c.Bind(&req); err != nil || req.Request == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"request\" field is required"})
	}

	places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	itinerary, err := application.GenerateItinerary(c.Request().Context(), s.generator, s.itineraryRepo, contextUserID(c), req.Request, places)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, toItineraryResponse(itinerary))
}

func (s *Server) listItineraries(c echo.Context) error {
	itineraries, err := application.ListItineraries(c.Request().Context(), s.itineraryRepo, contextUserID(c))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	responses := make([]itineraryResponse, len(itineraries))
	for i, it := range itineraries {
		responses[i] = toItineraryResponse(it)
	}
	return c.JSON(http.StatusOK, responses)
}

func (s *Server) getItinerary(c echo.Context) error {
	itinerary, err := application.GetItinerary(c.Request().Context(), s.itineraryRepo, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "itinerary not found"})
	}
	// An itinerary belongs to exactly the user who generated it -- this is
	// not a public route (unlike GET /places), so a caller fetching by ID
	// must own it, checked here rather than trusting the URL alone.
	if itinerary.UserID() != contextUserID(c) {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "itinerary not found"})
	}
	return c.JSON(http.StatusOK, toItineraryResponse(itinerary))
}
