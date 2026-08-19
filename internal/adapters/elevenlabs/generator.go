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
		// Cette API n'est pas streamée : la réponse n'arrive qu'une fois
		// l'audio ENTIER généré côté ElevenLabs, et ce temps croît avec la
		// longueur du texte -- 60s tenait pour un texte court, mais une
		// narration longue (~1300 mots, ~9 min d'audio) peut dépasser 60s de
		// synthèse à elle seule, sans que rien ne soit cassé. Un texte trop
		// long pour raisonnablement tenir dans 5 minutes reviendrait de toute
		// façon buter sur le plafond d'essais de worker.go (maxTTSAttempts),
		// donc pas de boucle infinie même si ce plafond s'avère encore trop
		// court pour un cas extrême.
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

type ttsRequest struct {
	Text    string `json:"text"`
	ModelID string `json:"model_id"`
}

// Generate appelle l'API ElevenLabs. Le paramètre `language` est volontairement
// inutilisé : il fait partie du port ports.TTSGenerator (d'autres backends TTS
// en auraient besoin), mais eleven_multilingual_v2 déduit la langue du texte
// lui-même — le lui passer explicitement n'est pas prévu par l'API. Ce n'est
// donc pas un oubli.
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

	// 4xx = la requête elle-même est en cause, réessayer à l'identique ne changera
	// rien — sauf 408 (timeout) et 429 (rate limit), qui sont temporels.
	// ElevenLabs renvoie notamment 422 (model_id inconnu, texte trop long),
	// 404 (voice_id inexistant) et 403 (tier/permission) : tous définitifs.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 &&
		resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
		// Préfixe "elevenlabs" pour la même raison que côté S3 : ports.PermanentError
		// est partagé entre les deux adaptateurs, son message ne dit donc plus d'où
		// vient l'erreur. Sans ça, un failure_reason stocké en base est ambigu.
		// Même convention que le fmt.Errorf juste en dessous.
		return nil, 0, &ports.PermanentError{StatusCode: resp.StatusCode, Body: "elevenlabs: " + string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("elevenlabs: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// Estimation grossière (~5 caractères par mot, ~400ms par mot). Plancher à 1
	// mot : un texte très court ("Oi!") donnerait sinon une durée de 0, que
	// domain.NewGeneratedAudio rejette.
	wordCount := max(1, len(text)/5)
	duration := time.Duration(wordCount) * 400 * time.Millisecond
	return respBody, duration, nil
}
