package http

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
)

type reviewScriptRequest struct {
	Reviewer string `json:"reviewer"`
	VoiceID  string `json:"voice_id"`
}

func (s *Server) reviewScript(c echo.Context) error {
	scriptID := c.Param("id")

	var req reviewScriptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request body"})
	}
	if req.Reviewer == "" || req.VoiceID == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "reviewer and voice_id are required"})
	}

	err := application.ReviewAndRequestAudio(c.Request().Context(), s.scriptRepo, s.audioFileRepo, s.publisher, scriptID, req.Reviewer, req.VoiceID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, echo.Map{"error": err.Error()})
	}
	return c.NoContent(http.StatusAccepted)
}
