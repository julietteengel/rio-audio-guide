package elevenlabs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"rioaudioguide/backend/internal/ports"
)

func TestGenerator_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "test-key" {
			t.Errorf("got xi-api-key %q, want test-key", r.Header.Get("xi-api-key"))
		}
		if r.URL.Path != "/voice-1" {
			t.Errorf("got path %q, want /voice-1", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("FAKE-MP3-BYTES"))
	}))
	defer server.Close()

	gen := &Generator{apiKey: "test-key", baseURL: server.URL, httpClient: server.Client()}

	audioBytes, duration, err := gen.Generate(context.Background(), "Bonjour le monde", "fr", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(audioBytes) != "FAKE-MP3-BYTES" {
		t.Fatalf("got audio bytes %q, want FAKE-MP3-BYTES", audioBytes)
	}
	if duration <= 0 {
		t.Fatalf("got duration %v, want > 0", duration)
	}
}

func TestGenerator_Generate_UnauthorizedIsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid api key"}`))
	}))
	defer server.Close()

	gen := &Generator{apiKey: "bad-key", baseURL: server.URL, httpClient: server.Client()}

	_, _, err := gen.Generate(context.Background(), "text", "fr", "voice-1")
	var permErr *ports.PermanentError
	if !errors.As(err, &permErr) {
		t.Fatalf("got error %v, want a *ports.PermanentError", err)
	}
	if permErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got status %d, want 401", permErr.StatusCode)
	}
}

func TestGenerator_Generate_RateLimitIsTransient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
	}))
	defer server.Close()

	gen := &Generator{apiKey: "test-key", baseURL: server.URL, httpClient: server.Client()}

	_, _, err := gen.Generate(context.Background(), "text", "fr", "voice-1")
	var permErr *ports.PermanentError
	if errors.As(err, &permErr) {
		t.Fatal("got a *ports.PermanentError for a 429, want a plain transient error")
	}
	if err == nil {
		t.Fatal("expected a non-nil error for a 429 response")
	}
}
