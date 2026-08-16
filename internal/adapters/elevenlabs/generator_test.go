package elevenlabs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rioaudioguide/backend/internal/ports"
)

// La classification permanent/transitoire décide si le worker Ack (abandon
// définitif, AudioFile "failed") ou Nack/requeue (nouvelle tentative). Se
// tromper de côté coûte cher : un 404 (voice_id inexistant) classé transitoire
// rejouerait le message pour toujours.
func TestGenerator_Generate_StatusClassification(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		wantErr     bool
		wantPermErr bool
	}{
		{name: "200 succès", status: http.StatusOK},
		{name: "400 requête invalide", status: http.StatusBadRequest, wantErr: true, wantPermErr: true},
		{name: "401 clé invalide", status: http.StatusUnauthorized, wantErr: true, wantPermErr: true},
		{name: "403 tier insuffisant", status: http.StatusForbidden, wantErr: true, wantPermErr: true},
		{name: "404 voice_id inconnu", status: http.StatusNotFound, wantErr: true, wantPermErr: true},
		{name: "422 validation", status: http.StatusUnprocessableEntity, wantErr: true, wantPermErr: true},
		{name: "429 rate limit", status: http.StatusTooManyRequests, wantErr: true, wantPermErr: false},
		{name: "500 erreur serveur", status: http.StatusInternalServerError, wantErr: true, wantPermErr: false},
		{name: "503 indisponible", status: http.StatusServiceUnavailable, wantErr: true, wantPermErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("xi-api-key") != "test-key" {
					t.Errorf("got xi-api-key %q, want test-key", r.Header.Get("xi-api-key"))
				}
				if r.URL.Path != "/voice-1" {
					t.Errorf("got path %q, want /voice-1", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("FAKE-MP3-BYTES"))
			}))
			defer server.Close()

			gen := &Generator{apiKey: "test-key", baseURL: server.URL, httpClient: server.Client()}

			audioBytes, duration, err := gen.Generate(context.Background(), "Bonjour le monde", "fr", "voice-1")

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(audioBytes) != "FAKE-MP3-BYTES" {
					t.Fatalf("got audio bytes %q, want FAKE-MP3-BYTES", audioBytes)
				}
				if duration <= 0 {
					t.Fatalf("got duration %v, want > 0", duration)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected a non-nil error for status %d", tt.status)
			}
			var permErr *ports.PermanentError
			isPerm := errors.As(err, &permErr)
			if isPerm != tt.wantPermErr {
				t.Fatalf("status %d: got permanent=%v (err %v), want permanent=%v", tt.status, isPerm, err, tt.wantPermErr)
			}
			if isPerm && permErr.StatusCode != tt.status {
				t.Fatalf("got PermanentError.StatusCode %d, want %d", permErr.StatusCode, tt.status)
			}
		})
	}
}

// Le modèle multilingue est ce qui permet aux scripts fr/en/es/pt de partager
// une seule voix — vérifier qu'il survit jusqu'au payload, pas seulement au
// littéral dans le code.
func TestGenerator_Generate_SendsMultilingualModelID(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("FAKE-MP3-BYTES"))
	}))
	defer server.Close()

	gen := &Generator{apiKey: "test-key", baseURL: server.URL, httpClient: server.Client()}
	if _, _, err := gen.Generate(context.Background(), "Bonjour le monde", "fr", "voice-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, `"model_id":"eleven_multilingual_v2"`) {
		t.Fatalf("got request body %q, want it to carry model_id eleven_multilingual_v2", gotBody)
	}
	if !strings.Contains(gotBody, "Bonjour le monde") {
		t.Fatalf("got request body %q, want it to carry the text", gotBody)
	}
}

// len(text)/5 tombe à 0 sur un texte très court, et domain.NewGeneratedAudio
// refuse une durée <= 0 — le plancher évite un échec sur un script minuscule.
func TestGenerator_Generate_ShortTextStillHasPositiveDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("FAKE-MP3-BYTES"))
	}))
	defer server.Close()

	gen := &Generator{apiKey: "test-key", baseURL: server.URL, httpClient: server.Client()}
	_, duration, err := gen.Generate(context.Background(), "Oi!", "pt", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duration <= 0 {
		t.Fatalf("got duration %v for a short text, want > 0", duration)
	}
}
