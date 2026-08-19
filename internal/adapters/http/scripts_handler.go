package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
)

type reviewScriptRequest struct {
	VoiceID string `json:"voice_id"`
}

// reviewScript est derrière requireAuth (voir server.go) -- le reviewer
// n'est plus un nom en texte libre fourni par le client (n'importe qui
// pouvait prétendre être n'importe qui), c'est l'ID du compte réellement
// authentifié par le token, lu via contextUserID.
func (s *Server) reviewScript(c echo.Context) error {
	scriptID := c.Param("id")

	var req reviewScriptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}
	if req.VoiceID == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "voice_id is required"})
	}

	err := application.ReviewAndRequestAudio(c.Request().Context(), s.scriptRepo, s.audioFileRepo, s.publisher, scriptID, contextUserID(c), req.VoiceID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.NoContent(http.StatusAccepted)
}
