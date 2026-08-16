package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// Generator calls the ElevenLabs REST API directly — no SDK, the API is a
// single well-documented POST.
type Generator struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewGenerator(apiKey string) *Generator {
	return &Generator{
		apiKey:     apiKey,
		baseURL:    "https://api.elevenlabs.io/v1/text-to-speech",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type ttsRequest struct {
	Text    string `json:"text"`
	ModelID string `json:"model_id"`
}

func (g *Generator) Generate(ctx context.Context, text, language, voiceID string) ([]byte, time.Duration, error) {
	body, err := json.Marshal(ttsRequest{Text: text, ModelID: "eleven_multilingual_v2"})
	if err != nil {
		return nil, 0, fmt.Errorf("elevenlabs: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", g.baseURL, voiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("elevenlabs: build request: %w", err)
	}
	req.Header.Set("xi-api-key", g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("elevenlabs: read response: %w", err)
	}

	// 401 (clé invalide) et 400 (texte/voice_id rejeté) ne se corrigeront pas
	// en réessayant le même message — tout le reste (429, 5xx) peut l'être.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		return nil, 0, &ports.PermanentError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("elevenlabs: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	wordCount := len(text) / 5
	duration := time.Duration(wordCount) * 400 * time.Millisecond
	return respBody, duration, nil
}
