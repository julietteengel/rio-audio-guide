package http

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Rio de Janeiro municipality, approximativement — suffisant pour un endpoint
// de démo "liste tout" ; un vrai produit paginerait ou accepterait des bornes
// en paramètre.
const (
	rioMinLat, rioMinLon = -23.1, -43.8
	rioMaxLat, rioMaxLon = -22.7, -43.0
)

type placeResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

func (s *Server) listPlaces(c echo.Context) error {
	key := fmt.Sprintf("places:%v:%v:%v:%v", rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
	return s.cachedJSON(c, key, func() (any, int, error) {
		places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
		if err != nil {
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
		}
		resp := make([]placeResponse, 0, len(places))
		for _, p := range places {
			resp = append(resp, placeResponse{
				ID:       p.ID(),
				Name:     p.Name().String(),
				Category: p.Category(),
				Lat:      p.Coordinates().Lat(),
				Lon:      p.Coordinates().Lon(),
			})
		}
		return resp, http.StatusOK, nil
	})
}
