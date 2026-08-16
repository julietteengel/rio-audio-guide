package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// presignExpiry : durée de vie de l'URL présignée renvoyée au client — assez
// long pour un téléchargement immédiat, pas fait pour être un lien partageable
// durablement.
const presignExpiry = 15 * time.Minute

type audioResponse struct {
	URL string `json:"url"`
}

type audioNotReadyResponse struct {
	Status string `json:"status"`
}

func (s *Server) getPlaceAudio(c echo.Context) error {
	placeID := c.Param("id")
	language := c.QueryParam("language")
	if language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "language query param is required"})
	}

	key := "audio:" + placeID + ":" + language
	if cached, found, err := s.cache.Get(c.Request().Context(), key); err == nil && found {
		return c.JSONBlob(http.StatusOK, []byte(cached))
	}

	script, err := s.scriptRepo.FindByPlaceIDAndLanguage(c.Request().Context(), placeID, language)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no script for this place/language"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	audioFile, err := s.audioFileRepo.FindByScriptID(c.Request().Context(), script.ID())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no audio ever requested for this script"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	if audioFile.Status() != "ready" {
		return c.JSON(http.StatusAccepted, audioNotReadyResponse{Status: string(audioFile.Status())})
	}

	s3Key, err := parseS3Key(audioFile.Audio().StorageURL())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	url, err := s.storage.PresignURL(c.Request().Context(), s3Key, presignExpiry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	body, err := json.Marshal(audioResponse{URL: url})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	_ = s.cache.Set(c.Request().Context(), key, string(body), cacheTTL)
	return c.JSONBlob(http.StatusOK, body)
}

// parseS3Key extrait la clé d'objet d'un storage_url au format s3://bucket/clé
// — c'est ce que domain.AudioFile.Audio().StorageURL() contient toujours,
// c'est le seul format que le worker écrit (internal/adapters/s3.Upload).
//
// getPlaceAudio distingue explicitement "vraiment absent" (pgx.ErrNoRows,
// 404) de toute autre erreur (panne DB transitoire, 500) — traiter toute
// erreur comme un 404 masquerait une vraie panne derrière une réponse "ça
// n'existe pas", la même classe de bug que l'erreur avalée trouvée dans
// cmd/import pendant le chantier ElevenLabs.
func parseS3Key(storageURL string) (string, error) {
	rest, ok := strings.CutPrefix(storageURL, "s3://")
	if !ok {
		return "", errors.New("storage URL is not an s3:// URL")
	}
	_, key, ok := strings.Cut(rest, "/")
	if !ok || key == "" {
		return "", errors.New("storage URL has no object key")
	}
	return key, nil
}
