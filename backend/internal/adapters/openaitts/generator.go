// Package openaitts implements ports.TTSGenerator on OpenAI's text-to-speech
// and Whisper APIs. This is the provider that produced the audio actually in
// the database (see backend/README.md); pipeline/audiogen is the batch-script
// equivalent of this adapter.
package openaitts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode"

	"rioaudioguide/backend/internal/ports"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "tts-1"
	// /audio/speech rejects input over 4096 characters; stay under it.
	maxTTSChars = 4000
	// Used only if Whisper fails and we must still report a positive duration
	// (domain.NewGeneratedAudio rejects 0): a calm narration pace.
	estimatedWordsPerSecond = 2.5
)

// Uploader is the slice of ports.AudioStorage this adapter needs.
type Uploader interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (url string, err error)
}

type Generator struct {
	apiKey   string
	model    string
	baseURL  string
	http     *http.Client
	uploader Uploader
	// Pause before the single Whisper retry; a field so tests don't wait.
	retryDelay time.Duration
}

func NewGenerator(apiKey string, uploader Uploader) *Generator {
	return &Generator{
		apiKey:   apiKey,
		model:    defaultModel,
		baseURL:  defaultBaseURL,
		http:     &http.Client{Timeout: 3 * time.Minute},
		uploader: uploader,

		retryDelay: 2 * time.Second,
	}
}

// Generate synthesizes text with the OpenAI voice voiceID (e.g. "onyx"),
// uploads the mp3, then asks Whisper for per-segment timestamps (neither OpenAI
// nor Google TTS return timing themselves).
//
// A Whisper failure does NOT fail the job: the audio is already paid for and
// usable, and the mobile app falls back to static text without timestamps.
// pipeline/audiogen/transcribe.py can backfill them later.
func (g *Generator) Generate(ctx context.Context, text, language, voiceID string) (string, string, time.Duration, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", 0, &ports.PermanentError{StatusCode: 400, Body: "empty script text"}
	}
	if voiceID == "" {
		return "", "", 0, &ports.PermanentError{StatusCode: 400, Body: "an OpenAI voice id (e.g. \"onyx\") is required"}
	}

	var audio []byte
	for _, chunk := range splitForTTS(text, maxTTSChars) {
		part, err := g.synthesize(ctx, chunk, voiceID)
		if err != nil {
			return "", "", 0, err
		}
		audio = append(audio, part...)
	}

	id, err := randomID()
	if err != nil {
		return "", "", 0, err
	}
	label := "openai-" + voiceID
	storageURL, err := g.uploader.Upload(ctx, "audio/"+label+"/"+id+".mp3", audio, "audio/mpeg")
	if err != nil {
		return "", "", 0, err // already classified (transient/permanent) by the storage adapter
	}

	body, whisperDuration, err := g.transcribeWithRetry(ctx, audio, language, text)
	if err != nil {
		// The duration is an estimate here; pipeline/audiogen/transcribe.py
		// replaces it with Whisper's real one when it backfills timestamps.
		log.Printf("openaitts: whisper failed for %s [%s], keeping audio without timestamps (duration estimated): %v", storageURL, language, err)
		return storageURL, "", estimateDuration(text), nil
	}
	timestampsURL, err := g.uploader.Upload(ctx, "timestamps/"+label+"/"+id+".json", body, "application/json")
	if err != nil {
		log.Printf("openaitts: timestamps upload failed for %s [%s], keeping audio without timestamps: %v", storageURL, language, err)
		return storageURL, "", whisperDuration, nil
	}
	return storageURL, timestampsURL, whisperDuration, nil
}

func (g *Generator) synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]string{
		"model": g.model, "voice": voice, "input": text, "response_format": "mp3",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return g.do(req)
}

// transcribeWithRetry gives a transient Whisper failure (429, 5xx, network)
// one more try before the caller falls back to audio without timestamps; a
// permanent one (bad key, exhausted credit) is not worth repeating.
func (g *Generator) transcribeWithRetry(ctx context.Context, audio []byte, language, scriptText string) ([]byte, time.Duration, error) {
	body, duration, err := g.transcribe(ctx, audio, language, scriptText)
	var permanent *ports.PermanentError
	if err == nil || errors.As(err, &permanent) || ctx.Err() != nil {
		return body, duration, err
	}
	select {
	case <-time.After(g.retryDelay):
	case <-ctx.Done():
		return nil, 0, err
	}
	return g.transcribe(ctx, audio, language, scriptText)
}

func (g *Generator) transcribe(ctx context.Context, audio []byte, language, scriptText string) ([]byte, time.Duration, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{
		"model":                     "whisper-1",
		"response_format":           "verbose_json",
		"timestamp_granularities[]": "segment",
		// Biases proper nouns toward the script's own spelling; Whisper once
		// heard "Praça XV" as "Traça 15" without it.
		"prompt": truncateRunes(scriptText, 500),
	}
	if code := whisperLanguage(language); code != "" {
		fields["language"] = code
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, 0, err
		}
	}
	part, err := w.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, 0, err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, 0, err
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	body, err := g.do(req)
	if err != nil {
		return nil, 0, err
	}
	var parsed struct {
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("whisper response is not JSON: %w (body: %s)", err, truncateRunes(string(body), 200))
	}
	if parsed.Duration <= 0 {
		return nil, 0, fmt.Errorf("whisper response has no usable duration (body: %s)", truncateRunes(string(body), 200))
	}
	return body, time.Duration(parsed.Duration * float64(time.Second)), nil
}

// do sends the request and classifies failures the way the worker needs them:
// a *ports.PermanentError for anything retrying the same message cannot fix
// (bad key, exhausted credit, invalid request), a plain error for what can
// (network, 5xx, rate limiting).
func (g *Generator) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}
	if isPermanent(resp.StatusCode, string(body)) {
		return nil, &ports.PermanentError{StatusCode: resp.StatusCode, Body: truncateRunes(string(body), 300)}
	}
	return nil, fmt.Errorf("openai transient error (status %d): %s", resp.StatusCode, truncateRunes(string(body), 300))
}

func isPermanent(status int, body string) bool {
	if status == http.StatusTooManyRequests {
		// 429 is normally "slow down" (retry), but OpenAI also uses it for an
		// exhausted balance, which no amount of retrying fixes.
		return strings.Contains(body, "insufficient_quota")
	}
	if status == http.StatusRequestTimeout || status >= 500 {
		return false
	}
	return status >= 400 // 400, 401, 403, 404, 422...
}

func whisperLanguage(language string) string {
	code := strings.ToLower(strings.TrimSpace(language))
	if len(code) > 2 {
		code = code[:2]
	}
	switch code {
	case "en", "fr", "pt", "es":
		return code
	}
	return ""
}

func estimateDuration(text string) time.Duration {
	words := len(strings.Fields(text))
	return time.Duration(float64(words) / estimatedWordsPerSecond * float64(time.Second))
}

func randomID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// splitForTTS cuts text into chunks of at most limit bytes on sentence
// boundaries; a single sentence longer than limit is split on whitespace.
func splitForTTS(text string, limit int) []string {
	var chunks []string
	current := ""
	flush := func() {
		if current != "" {
			chunks = append(chunks, current)
			current = ""
		}
	}
	for _, sentence := range splitSentences(strings.TrimSpace(text)) {
		for _, piece := range hardSplit(sentence, limit) {
			if current != "" && len(current)+1+len(piece) > limit {
				flush()
			}
			if current == "" {
				current = piece
			} else {
				current += " " + piece
			}
		}
	}
	flush()
	return chunks
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		if (r == '.' || r == '!' || r == '?' || r == '…') && i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
			out = append(out, strings.TrimSpace(string(runes[start:i+1])))
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

func hardSplit(sentence string, limit int) []string {
	if len(sentence) <= limit {
		return []string{sentence}
	}
	var pieces []string
	current := ""
	for _, word := range strings.Fields(sentence) {
		if current != "" && len(current)+1+len(word) > limit {
			pieces = append(pieces, current)
			current = word
		} else if current == "" {
			current = word
		} else {
			current += " " + word
		}
	}
	if current != "" {
		pieces = append(pieces, current)
	}
	return pieces
}
