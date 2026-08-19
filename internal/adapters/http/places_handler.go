package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/domain"
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

// listPlaces sert aussi de recherche : ?q= filtre par sous-chaîne du nom
// (insensible à la casse), en mémoire — pas de méthode de recherche dédiée
// dans PlaceRepository, donc on filtre côté adaptateur plutôt que de changer
// le port. Suffisant à l'échelle actuelle (~236 lieux au plus) ; un vrai index
// de recherche serait à envisager si ça grossit beaucoup plus.
func (s *Server) listPlaces(c echo.Context) error {
	query := strings.ToLower(strings.TrimSpace(c.QueryParam("q")))
	key := fmt.Sprintf("places:%v:%v:%v:%v:q=%s", rioMinLat, rioMinLon, rioMaxLat, rioMaxLon, query)
	return s.cachedJSON(c, key, func() (any, int, error) {
		places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
		if err != nil {
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
		}
		resp := make([]placeResponse, 0, len(places))
		for _, p := range places {
			if query != "" && !strings.Contains(strings.ToLower(p.Name().String()), query) {
				continue
			}
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

type placeDetailResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Lat            float64 `json:"lat"`
	Lon            float64 `json:"lon"`
	Language       string  `json:"language"`
	Narration      string  `json:"narration"`
	Source         string  `json:"source"`
	SourceRichness string  `json:"source_richness"`
}

type placeDetailNotReadyResponse struct {
	Status string `json:"status"`
}

// getPlaceDetail sert le lieu et sa narration dans la langue demandée. La
// narration n'est jamais servie tant que le Script n'est pas Published — même
// garde que getPlaceAudio (audio_handler.go) pour l'audio : un texte encore en
// brouillon ou en relecture n'a pas été validé, donc pas de fuite de contenu
// non approuvé, même en lecture seule.
func (s *Server) getPlaceDetail(c echo.Context) error {
	placeID := c.Param("id")
	language := c.QueryParam("language")
	if language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "language query param is required"})
	}

	key := "place-detail:" + placeID + ":" + language
	return s.cachedJSON(c, key, func() (any, int, error) {
		place, err := s.placeRepo.FindByID(c.Request().Context(), placeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return echo.Map{"error": "place not found"}, http.StatusNotFound, nil
			}
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
		}

		script, err := s.scriptRepo.FindByPlaceIDAndLanguage(c.Request().Context(), placeID, language)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return echo.Map{"error": "no narration for this place/language"}, http.StatusNotFound, nil
			}
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
		}

		if script.Status() != domain.ScriptStatusPublished {
			return placeDetailNotReadyResponse{Status: "narration not yet published"}, http.StatusAccepted, nil
		}

		return placeDetailResponse{
			ID:             place.ID(),
			Name:           place.Name().String(),
			Category:       place.Category(),
			Lat:            place.Coordinates().Lat(),
			Lon:            place.Coordinates().Lon(),
			Language:       language,
			Narration:      script.Text().String(),
			Source:         place.Source(),
			SourceRichness: place.SourceRichness(),
		}, http.StatusOK, nil
	})
}
