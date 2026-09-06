package http

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type conversationTurnRequest struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type askAssistantRequest struct {
	Language string                    `json:"language"`
	Question string                    `json:"question"`
	History  []conversationTurnRequest `json:"history"`
}

type askAssistantResponse struct {
	Answer         string `json:"answer"`
	GroundingLevel string `json:"grounding_level"`
}

func (s *Server) askAssistant(c echo.Context) error {
	var req askAssistantRequest
	if err := c.Bind(&req); err != nil || req.Question == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"question\" field is required"})
	}
	if req.Language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "a non-empty \"language\" field is required"})
	}

	history := make([]ports.ConversationTurn, len(req.History))
	for i, turn := range req.History {
		history[i] = ports.ConversationTurn{Question: turn.Question, Answer: turn.Answer}
	}

	answer, err := application.AskAssistant(c.Request().Context(), s.assistant, s.scriptRepo, c.Param("id"), req.Language, history, req.Question)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrNoPublishedScript):
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no narration for this place/language yet"})
		case errors.Is(err, application.ErrAssistantFailed):
			log.Printf("assistant: failed to answer: %v", err)
			return c.JSON(http.StatusBadGateway, echo.Map{"error": "the assistant is temporarily unavailable, please try again"})
		default:
			log.Printf("assistant: unexpected error: %v", err)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "unexpected error"})
		}
	}
	return c.JSON(http.StatusOK, askAssistantResponse{Answer: answer.Answer, GroundingLevel: answer.GroundingLevel})
}
