package openaitts

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"rioaudioguide/backend/internal/ports"
)

type fakeUploader struct {
	mu   sync.Mutex
	keys []string
	data map[string][]byte
	err  error
}

func (f *fakeUploader) Upload(_ context.Context, key string, data []byte, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	f.keys = append(f.keys, key)
	f.data[key] = data
	return "s3://bucket/" + key, nil
}

// fakeOpenAI serves /audio/speech and /audio/transcriptions.
type fakeOpenAI struct {
	speechStatus, transcribeStatus int
	speechBody, transcribeBody     string
	speechCalls                    int
	speechInputs                   []string
	transcribeLanguage             string
	mu                             sync.Mutex
}

func (f *fakeOpenAI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.speechCalls++
		b, _ := io.ReadAll(r.Body)
		f.speechInputs = append(f.speechInputs, string(b))
		if f.speechStatus != 0 && f.speechStatus != 200 {
			w.WriteHeader(f.speechStatus)
			_, _ = w.Write([]byte(f.speechBody))
			return
		}
		_, _ = w.Write([]byte("MP3"))
	})
	mux.HandleFunc("/audio/transcriptions", func(w http.ResponseWriter, r *http.Request) {
		if f.transcribeStatus != 0 && f.transcribeStatus != 200 {
			w.WriteHeader(f.transcribeStatus)
			_, _ = w.Write([]byte(f.transcribeBody))
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		f.transcribeLanguage = r.FormValue("language")
		_, _ = w.Write([]byte(`{"duration": 12.5, "segments": [{"start": 0.0, "text": "Oi."}]}`))
	})
	return mux
}

func newTestGenerator(t *testing.T, f *fakeOpenAI, up *fakeUploader) *Generator {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	g := NewGenerator("test-key", up)
	g.baseURL = srv.URL
	g.retryDelay = 0
	return g
}

func TestGenerate_UploadsAudioAndWhisperTimestamps(t *testing.T) {
	up := &fakeUploader{}
	f := &fakeOpenAI{}
	g := newTestGenerator(t, f, up)

	audioURL, tsURL, dur, err := g.Generate(context.Background(), "Olá, Rio.", "pt", "onyx")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(audioURL, "s3://bucket/audio/openai-onyx/") || !strings.HasSuffix(audioURL, ".mp3") {
		t.Errorf("audio url %q", audioURL)
	}
	if !strings.HasPrefix(tsURL, "s3://bucket/timestamps/openai-onyx/") || !strings.HasSuffix(tsURL, ".json") {
		t.Errorf("timestamps url %q", tsURL)
	}
	if dur.Seconds() != 12.5 {
		t.Errorf("duration %v, want Whisper's 12.5s", dur)
	}
	if f.transcribeLanguage != "pt" {
		t.Errorf("whisper got language %q, want pt", f.transcribeLanguage)
	}
	if got := string(up.data[strings.TrimPrefix(audioURL, "s3://bucket/")]); got != "MP3" {
		t.Errorf("uploaded audio %q", got)
	}
}

func TestGenerate_LongTextIsSplitIntoSeveralTTSCalls(t *testing.T) {
	f := &fakeOpenAI{}
	g := newTestGenerator(t, f, &fakeUploader{})
	text := strings.Repeat("Uma frase de teste que se repete. ", 400) // ~13k chars

	if _, _, _, err := g.Generate(context.Background(), text, "pt", "onyx"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if f.speechCalls < 3 {
		t.Fatalf("got %d TTS calls, want the text split into several", f.speechCalls)
	}
}

func TestGenerate_ExhaustedCreditIsPermanent(t *testing.T) {
	f := &fakeOpenAI{speechStatus: 429, speechBody: `{"error":{"code":"insufficient_quota"}}`}
	g := newTestGenerator(t, f, &fakeUploader{})

	_, _, _, err := g.Generate(context.Background(), "Olá.", "pt", "onyx")
	var perm *ports.PermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("got %v, want *ports.PermanentError so the worker stops retrying", err)
	}
}

func TestGenerate_BadKeyIsPermanent(t *testing.T) {
	g := newTestGenerator(t, &fakeOpenAI{speechStatus: 401, speechBody: "bad key"}, &fakeUploader{})
	_, _, _, err := g.Generate(context.Background(), "Olá.", "pt", "onyx")
	var perm *ports.PermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("got %v, want permanent", err)
	}
}

func TestGenerate_RateLimitAndServerErrorsAreTransient(t *testing.T) {
	for _, status := range []int{429, 500, 503} {
		g := newTestGenerator(t, &fakeOpenAI{speechStatus: status, speechBody: "slow down"}, &fakeUploader{})
		_, _, _, err := g.Generate(context.Background(), "Olá.", "pt", "onyx")
		var perm *ports.PermanentError
		if err == nil || errors.As(err, &perm) {
			t.Errorf("status %d: got %v, want a transient (retryable) error", status, err)
		}
	}
}

func TestGenerate_WhisperFailureKeepsTheAudio(t *testing.T) {
	up := &fakeUploader{}
	g := newTestGenerator(t, &fakeOpenAI{transcribeStatus: 500, transcribeBody: "boom"}, up)

	audioURL, tsURL, dur, err := g.Generate(context.Background(), "Uma duas três quatro cinco seis sete oito.", "pt", "onyx")
	if err != nil {
		t.Fatalf("a Whisper failure must not discard paid-for audio: %v", err)
	}
	if audioURL == "" || tsURL != "" {
		t.Errorf("audio=%q timestamps=%q, want audio kept and no timestamps", audioURL, tsURL)
	}
	if dur <= 0 {
		t.Errorf("duration must be positive (estimated), got %v", dur)
	}
}

func TestGenerate_EmptyTextOrVoiceIsPermanent(t *testing.T) {
	g := newTestGenerator(t, &fakeOpenAI{}, &fakeUploader{})
	for _, c := range []struct{ text, voice string }{{"  ", "onyx"}, {"Olá.", ""}} {
		_, _, _, err := g.Generate(context.Background(), c.text, "pt", c.voice)
		var perm *ports.PermanentError
		if !errors.As(err, &perm) {
			t.Errorf("text=%q voice=%q: got %v, want permanent", c.text, c.voice, err)
		}
	}
}

func TestSplitForTTS_RespectsLimitAndKeepsSentencesWhole(t *testing.T) {
	text := strings.Repeat("Frase número um. ", 100)
	chunks := splitForTTS(text, 200)
	if len(chunks) < 2 {
		t.Fatal("expected several chunks")
	}
	for _, c := range chunks {
		if len(c) > 200 {
			t.Errorf("chunk of %d bytes exceeds limit", len(c))
		}
		if !strings.HasSuffix(c, ".") {
			t.Errorf("chunk cut mid-sentence: %q", c[len(c)-10:])
		}
	}
	if strings.Join(chunks, " ") != strings.TrimSpace(text) {
		t.Error("text lost or duplicated")
	}
}

func TestSplitForTTS_HardSplitsOneHugeSentence(t *testing.T) {
	chunks := splitForTTS(strings.Repeat("palavra ", 200), 100)
	for _, c := range chunks {
		if len(c) > 100 {
			t.Fatalf("chunk of %d bytes exceeds limit", len(c))
		}
	}
}

func TestGenerate_RetriesATransientWhisperFailureOnce(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/audio/speech", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("MP3")) })
	mux.HandleFunc("/audio/transcriptions", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"duration": 7.0, "segments": []}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	g := NewGenerator("test-key", &fakeUploader{})
	g.baseURL, g.retryDelay = srv.URL, 0

	_, tsURL, dur, err := g.Generate(context.Background(), "Olá, Rio.", "pt", "onyx")
	if err != nil || tsURL == "" || dur.Seconds() != 7 {
		t.Fatalf("err=%v timestamps=%q duration=%v, want the retry to succeed", err, tsURL, dur)
	}
	if calls != 2 {
		t.Fatalf("whisper called %d times, want exactly 2", calls)
	}
}

func TestGenerate_PermanentWhisperErrorIsNotRetried(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/audio/speech", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("MP3")) })
	mux.HandleFunc("/audio/transcriptions", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	g := NewGenerator("test-key", &fakeUploader{})
	g.baseURL, g.retryDelay = srv.URL, 0

	if _, _, _, err := g.Generate(context.Background(), "Olá, Rio.", "pt", "onyx"); err != nil {
		t.Fatalf("audio must be kept: %v", err)
	}
	if calls != 1 {
		t.Fatalf("whisper called %d times, want 1 (no retry on a permanent error)", calls)
	}
}
