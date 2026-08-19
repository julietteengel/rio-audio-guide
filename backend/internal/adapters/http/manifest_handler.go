package http

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/domain"
)

// rioCitySlug est la seule valeur de :city acceptée pour l'instant — il n'y a
// pas de concept "ville" dans le domaine (Place n'a que des coordonnées), donc
// on réutilise la même bounding box que listPlaces. À revoir si/quand
// plusieurs villes sont vraiment sourcées.
const rioCitySlug = "rio"

type manifestPlace struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Category       string  `json:"category"`
	Lat            float64 `json:"lat"`
	Lon            float64 `json:"lon"`
	Narration      string  `json:"narration"`
	Source         string  `json:"source"`
	SourceRichness string  `json:"source_richness"`
	AudioURL       string  `json:"audio_url"`
}

type cityManifestResponse struct {
	City     string          `json:"city"`
	Language string          `json:"language"`
	Places   []manifestPlace `json:"places"`
}

// getCityManifest liste, pour une ville et une langue données, tous les lieux
// prêts à être téléchargés pour un usage hors ligne : narration ET audio tous
// les deux publiés/prêts. Un lieu qui n'a pas encore de script publié dans
// cette langue, ou dont l'audio n'est pas encore généré, est simplement omis
// du manifeste plutôt que de faire échouer toute la requête — le bundle
// offline ne contient jamais de contenu partiel ou non approuvé pour un lieu
// donné, mais l'absence d'un lieu ne bloque pas les autres.
//
// Fan-out N+1 assumé : un aller-retour DB/S3 par lieu candidat. Acceptable à
// l'échelle visée (jusqu'à ~236 lieux) ; à revisiter avec une requête groupée
// si ça devient un vrai goulot.
func (s *Server) getCityManifest(c echo.Context) error {
	city := c.Param("city")
	language := c.QueryParam("language")
	if language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "language query param is required"})
	}
	if city != rioCitySlug {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "unknown city"})
	}

	key := "manifest:" + city + ":" + language
	return s.cachedJSON(c, key, func() (any, int, error) {
		places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
		if err != nil {
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
		}

		ready := make([]manifestPlace, 0, len(places))
		for _, p := range places {
			script, err := s.scriptRepo.FindByPlaceIDAndLanguage(c.Request().Context(), p.ID(), language)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
			}
			if script.Status() != domain.ScriptStatusPublished {
				continue
			}

			audioFile, err := s.audioFileRepo.FindByScriptID(c.Request().Context(), script.ID())
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
			}
			if audioFile.Status() != domain.AudioFileStatusReady {
				continue
			}

			s3Key, err := parseS3Key(audioFile.Audio().StorageURL())
			if err != nil {
				return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
			}
			audioURL, err := s.storage.PresignURL(c.Request().Context(), s3Key, presignExpiry)
			if err != nil {
				return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
			}

			ready = append(ready, manifestPlace{
				ID:             p.ID(),
				Name:           p.Name().String(),
				Category:       p.Category(),
				Lat:            p.Coordinates().Lat(),
				Lon:            p.Coordinates().Lon(),
				Narration:      script.Text().String(),
				Source:         p.Source(),
				SourceRichness: p.SourceRichness(),
				AudioURL:       audioURL,
			})
		}

		return cityManifestResponse{City: city, Language: language, Places: ready}, http.StatusOK, nil
	})
}
