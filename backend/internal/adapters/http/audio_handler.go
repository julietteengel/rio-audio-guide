package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/domain"
)

// presignExpiry : durée de vie de l'URL présignée renvoyée au client — assez
// long pour un téléchargement immédiat, pas fait pour être un lien partageable
// durablement.
const presignExpiry = 15 * time.Minute

type audioResponse struct {
	URL           string  `json:"url"`
	TimestampsURL *string `json:"timestamps_url,omitempty"`
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
	cached, found, err := s.cache.Get(c.Request().Context(), key)
	if err != nil {
		// Fail-open : on continue vers la base, mais on le dit — sinon un Redis
		// en panne est indistinguable d'un cache qui marche (cf. cachedJSON).
		log.Printf("cache get failed for key %q: %v", key, err)
	}
	if err == nil && found {
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

	if audioFile.Status() != domain.AudioFileStatusReady {
		return c.JSON(http.StatusAccepted, audioNotReadyResponse{Status: string(audioFile.Status())})
	}

	// "audio ready" ne suffit pas comme feu vert. CompleteAudioGeneration fait DEUX
	// sauvegardes non transactionnelles : l'AudioFile passe "ready" et est sauvé,
	// PUIS le Script est publié et sauvé. Un crash entre les deux laisse un audio
	// "ready" accroché à un Script encore "reviewed" — et cette route est le premier
	// chemin de lecture public sur ces données. Rendre les deux sauvegardes atomiques
	// est le vrai correctif (côté application layer) ; en attendant, on refuse de
	// servir l'URL plutôt que d'exposer un contenu jamais publié.
	//
	// Vérifié ici, après le statut de l'AudioFile, et pas avant : le cas courant
	// "génération en cours" a un Script encore "reviewed", et doit répondre
	// "generating" (l'info utile pour le client), pas "script not yet published".
	// Seul le chemin qui sert réellement l'URL a besoin de cette garde.
	if script.Status() != domain.ScriptStatusPublished {
		return c.JSON(http.StatusAccepted, audioNotReadyResponse{Status: "script not yet published"})
	}

	s3Key, err := parseS3Key(audioFile.Audio().StorageURL())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	url, err := s.storage.PresignURL(c.Request().Context(), s3Key, presignExpiry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	resp := audioResponse{URL: url}
	if timestampsStorageURL := audioFile.Audio().TimestampsURL(); timestampsStorageURL != "" {
		timestampsKey, err := parseS3Key(timestampsStorageURL)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
		}
		timestampsURL, err := s.storage.PresignURL(c.Request().Context(), timestampsKey, presignExpiry)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
		}
		resp.TimestampsURL = &timestampsURL
	}

	body, err := json.Marshal(resp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	if err := s.cache.Set(c.Request().Context(), key, string(body), cacheTTL); err != nil {
		log.Printf("cache set failed for key %q: %v", key, err) // fail-open : logué, jamais fatal
	}
	return c.JSONBlob(http.StatusOK, body)
}

// parseS3Key extrait la clé d'objet d'un storage_url au format s3://bucket/clé
// — c'est ce que domain.AudioFile.Audio().StorageURL() contient toujours,
// quel que soit l'adaptateur TTS actif qui l'a produit (actuellement Polly,
// via OutputS3KeyPrefix dans internal/adapters/awspolly) : ce format-là est
// le contrat à préserver, pas un détail d'implémentation d'un adaptateur
// donné.
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
