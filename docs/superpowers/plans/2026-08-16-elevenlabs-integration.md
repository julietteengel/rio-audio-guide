# ElevenLabs Integration + CSV-to-Postgres Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stubbed TTS call in the RabbitMQ worker with a real ElevenLabs call, and build the
missing bridge that gets Python-pipeline narrations into Postgres as `Place`/`Script` rows the worker can
actually process.

**Architecture:** Go worker keeps owning `tts_jobs` end to end (unchanged); its stub is replaced by a new
`TTSGenerator` port + `elevenlabs` adapter (plain REST, no SDK). A new `cmd/import` Go CLI reads the two
Python-produced CSVs and writes `Place`/`Script` rows through the existing repositories, respecting domain
invariants. A one-off Python script handles ElevenLabs voice cloning separately, decoupled from both.

**Tech Stack:** Go 1.25 (stdlib `net/http`, `encoding/csv`, `github.com/jackc/pgx/v5`), Python 3.11+
(`requests`, already a pipeline dependency).

**Spec:** `docs/superpowers/specs/2026-08-16-elevenlabs-integration-design.md`

**Worktrees:** Tasks 1–7 touch the `backend` branch/worktree (`.worktrees/backend/`, module
`rioaudioguide/backend`). Task 8 touches the `sourcing-pipeline` branch/worktree
(`.worktrees/sourcing-pipeline/`, `pipeline/curation/`). All file paths below are relative to the
worktree named in the task header.

## Global Constraints

- Go 1.25.0, module `rioaudioguide/backend` — no new go.mod dependency for the ElevenLabs call itself
  (`net/http`/`encoding/json` cover it); `pgconn` for Postgres error codes is already pulled in
  transitively by `jackc/pgx/v5`.
- ElevenLabs TTS model: `eleven_multilingual_v2` (scripts are fr/en/es/pt).
- `ELEVENLABS_API_KEY` is read from environment/K8s secret, never committed.
- Domain writes (`Place`, `Script`, `AudioFile`) only ever happen through the existing repositories/domain
  constructors — no raw SQL or direct DB writes from Python, per `CLAUDE.md`'s protection of
  `internal/domain`/`internal/ports`.
- `pipeline/curation/` scripts stay one-off/untested, matching the rest of that directory.

---

### Task 1: `TTSGenerator` port + `PermanentError`

**Worktree:** `backend`

**Files:**
- Create: `internal/ports/tts_generator.go`
- Test: `internal/ports/tts_generator_test.go`

**Interfaces:**
- Produces: `ports.TTSGenerator` interface (`Generate(ctx, text, language, voiceID string) ([]byte,
  time.Duration, error)`), `ports.PermanentError{StatusCode int; Body string}` (implements `error`).

- [ ] **Step 1: Write the failing test**

```go
// internal/ports/tts_generator_test.go
package ports

import (
	"errors"
	"testing"
)

func TestPermanentError_Error(t *testing.T) {
	err := &PermanentError{StatusCode: 401, Body: "invalid api key"}
	got := err.Error()
	if got == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestPermanentError_IsDetectableWithErrorsAs(t *testing.T) {
	var wrapped error = &PermanentError{StatusCode: 400, Body: "bad text"}

	var permErr *PermanentError
	if !errors.As(wrapped, &permErr) {
		t.Fatal("expected errors.As to unwrap a *PermanentError")
	}
	if permErr.StatusCode != 400 {
		t.Fatalf("got status %d, want 400", permErr.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ports/... -run TestPermanentError -v`
Expected: FAIL — compilation error, `PermanentError` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/ports/tts_generator.go
package ports

import (
	"context"
	"fmt"
	"time"
)

// TTSGenerator is the outbound port to a text-to-speech provider — implemented
// by internal/adapters/elevenlabs.
type TTSGenerator interface {
	Generate(ctx context.Context, text, language, voiceID string) (audioBytes []byte, duration time.Duration, err error)
}

// PermanentError indicates the TTS provider rejected the request in a way
// retrying the same message won't fix (bad API key, invalid text/voice_id).
// The RabbitMQ worker uses this to stop requeueing instead of looping
// forever on an unrecoverable message.
type PermanentError struct {
	StatusCode int
	Body       string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("tts generator: permanent error (status %d): %s", e.StatusCode, e.Body)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ports/... -run TestPermanentError -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ports/tts_generator.go internal/ports/tts_generator_test.go
git commit -m "Add TTSGenerator port and PermanentError"
```

---

### Task 2: ElevenLabs adapter

**Worktree:** `backend`

**Files:**
- Create: `internal/adapters/elevenlabs/generator.go`
- Test: `internal/adapters/elevenlabs/generator_test.go`

**Interfaces:**
- Consumes: `ports.TTSGenerator`, `ports.PermanentError` (Task 1).
- Produces: `elevenlabs.NewGenerator(apiKey string) *Generator`, satisfying `ports.TTSGenerator`.

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/elevenlabs/generator_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/elevenlabs/... -v`
Expected: FAIL — compilation error, `Generator` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/adapters/elevenlabs/generator.go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/elevenlabs/... -v`
Expected: PASS (all three tests)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/elevenlabs/generator.go internal/adapters/elevenlabs/generator_test.go
git commit -m "Add ElevenLabs TTS adapter"
```

---

### Task 3: `FailAudioGeneration` use case

**Worktree:** `backend`

**Files:**
- Modify: `internal/application/publish_script.go`
- Modify: `internal/application/publish_script_test.go`

**Interfaces:**
- Consumes: `domain.AudioFile.MarkFailed(reason string) error` (already exists), `ports.AudioFileRepository`.
- Produces: `application.FailAudioGeneration(ctx, audioFileRepo, audioFileID, reason string) error`.

- [ ] **Step 1: Write the failing test**

Append to `internal/application/publish_script_test.go`:

```go
func TestFailAudioGeneration(t *testing.T) {
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFileRepo.Save(ctx, audioFile)

	if err := FailAudioGeneration(ctx, audioFileRepo, audioFile.ID(), "TTS quota exceeded"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID())
	if saved.Status() != domain.AudioFileStatusFailed {
		t.Fatalf("got status %v, want failed", saved.Status())
	}
	if saved.FailureReason() != "TTS quota exceeded" {
		t.Fatalf("got failure reason %q, want %q", saved.FailureReason(), "TTS quota exceeded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/... -run TestFailAudioGeneration -v`
Expected: FAIL — compilation error, `FailAudioGeneration` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/application/publish_script.go`:

```go
func FailAudioGeneration(ctx context.Context, audioFileRepo ports.AudioFileRepository, audioFileID, reason string) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}
	if err := audioFile.MarkFailed(reason); err != nil {
		return err
	}
	return audioFileRepo.Save(ctx, audioFile)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/... -v`
Expected: PASS (all tests in the package, including the pre-existing ones)

- [ ] **Step 5: Commit**

```bash
git add internal/application/publish_script.go internal/application/publish_script_test.go
git commit -m "Add FailAudioGeneration use case"
```

---

### Task 4: Wire `TTSGenerator` into the RabbitMQ worker

**Worktree:** `backend`

**Files:**
- Modify: `internal/adapters/rabbitmq/worker.go`
- Modify: `internal/adapters/rabbitmq/worker_test.go`

**Interfaces:**
- Consumes: `ports.TTSGenerator`, `ports.PermanentError` (Task 1), `application.FailAudioGeneration` (Task 3).
- Produces: `NewWorker(channel, scriptRepo, audioFileRepo, storage, ttsGenerator)` — signature changes,
  gains a 5th parameter.

- [ ] **Step 1: Write the failing test**

In `internal/adapters/rabbitmq/worker_test.go`, add `"rioaudioguide/backend/internal/ports"` to the
import block, change the existing `TestWorker_ProcessesJobEndToEnd`'s call from
`NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{})` to
`NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{}, fakeTTSGenerator{})`, and append:

```go
type fakeTTSGenerator struct{}

func (fakeTTSGenerator) Generate(_ context.Context, text, _, _ string) ([]byte, time.Duration, error) {
	return []byte("FAKE-AUDIO:" + text), 5 * time.Second, nil
}

type failingTTSGenerator struct{ err error }

func (f failingTTSGenerator) Generate(_ context.Context, _, _, _ string) ([]byte, time.Duration, error) {
	return nil, 0, f.err
}

func TestWorker_PermanentTTSError_MarksAudioFileFailedAndAcks(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	permErr := &ports.PermanentError{StatusCode: 401, Body: "invalid api key"}
	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{}, failingTTSGenerator{err: permErr})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = worker.Run(ctx) }()

	body, _ := json.Marshal(ttsJobMessage{
		AudioFileID: audioFile.ID(),
		ScriptID:    script.ID(),
		Text:        "Texte",
		Language:    "fr",
		VoiceID:     "voice-1",
	})
	if err := channel.PublishWithContext(context.Background(), "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	}); err != nil {
		t.Fatalf("publish test job: %v", err)
	}

	deadline := time.After(4 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to be processed")
		case <-tick.C:
			found, err := audioFileRepo.FindByID(context.Background(), audioFile.ID())
			if err == nil && found.Status() == domain.AudioFileStatusFailed {
				if found.FailureReason() == "" {
					t.Fatal("expected a non-empty failure reason")
				}
				return
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -run TestWorker_PermanentTTSError -v`
Expected: FAIL — compilation error: `NewWorker` still takes 4 arguments in `worker.go`, but the test now
calls it with 5 (`fakeStorage{}, fakeTTSGenerator{}` / `..., failingTTSGenerator{...}`).

- [ ] **Step 3: Write minimal implementation**

Replace the full contents of `internal/adapters/rabbitmq/worker.go`:

```go
package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type Worker struct {
	channel       *amqp.Channel
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	storage       ports.AudioStorage
	ttsGenerator  ports.TTSGenerator
}

func NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, storage ports.AudioStorage, ttsGenerator ports.TTSGenerator) (*Worker, error) {
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &Worker{channel: channel, scriptRepo: scriptRepo, audioFileRepo: audioFileRepo, storage: storage, ttsGenerator: ttsGenerator}, nil
}

// Run consomme tts_jobs jusqu'à annulation du ctx. Bloquant — à lancer dans
// sa propre goroutine ou son propre binaire (cmd/worker).
func (w *Worker) Run(ctx context.Context) error {
	msgs, err := w.channel.Consume(TTSJobQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			w.handle(ctx, msg)
		}
	}
}

func (w *Worker) handle(ctx context.Context, msg amqp.Delivery) {
	var job ttsJobMessage
	if err := json.Unmarshal(msg.Body, &job); err != nil {
		log.Printf("tts worker: bad message, dropping: %v", err)
		_ = msg.Nack(false, false)
		return
	}

	if err := application.StartAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID); err != nil {
		log.Printf("tts worker: start generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	audioBytes, duration, err := w.ttsGenerator.Generate(ctx, job.Text, job.Language, job.VoiceID)
	if err != nil {
		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			log.Printf("tts worker: permanent TTS error for %s, marking failed: %v", job.AudioFileID, err)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			// Ack, pas Nack : réessayer le même message ne changera rien à une
			// clé invalide ou un texte/voice_id rejeté.
			_ = msg.Ack(false)
			return
		}
		log.Printf("tts worker: TTS generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	storageURL, err := w.storage.Upload(ctx, job.AudioFileID+".mp3", audioBytes, "audio/mpeg")
	if err != nil {
		log.Printf("tts worker: upload failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	if err := application.CompleteAudioGeneration(ctx, w.scriptRepo, w.audioFileRepo, job.AudioFileID, storageURL, "", duration); err != nil {
		log.Printf("tts worker: complete generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true)
		return
	}

	_ = msg.Ack(false)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: PASS (both `TestWorker_ProcessesJobEndToEnd` and the new permanent-error test) — requires a
local RabbitMQ: `docker run --rm -d --name rio-rabbitmq -p 5672:5672 rabbitmq:3-management`

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/rabbitmq/worker.go internal/adapters/rabbitmq/worker_test.go
git commit -m "Wire TTSGenerator into the RabbitMQ worker, replacing the stub"
```

---

### Task 5: Wire the real ElevenLabs adapter into `cmd/worker` + Helm secret

**Worktree:** `backend`

**Files:**
- Modify: `cmd/worker/main.go`
- Modify: `deploy/helm/rio-backend/templates/worker-deployment.yaml`

**Interfaces:**
- Consumes: `elevenlabs.NewGenerator(apiKey string) *Generator` (Task 2), `rabbitmq.NewWorker` (now 5 args,
  Task 4).

- [ ] **Step 1: Wire the generator into `cmd/worker/main.go`**

In `cmd/worker/main.go`, add the import `"rioaudioguide/backend/internal/adapters/elevenlabs"` to the
import block, and change:

```go
	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)

	worker, err := rabbitmq.NewWorker(channel, scriptRepo, audioFileRepo, storage)
```

to:

```go
	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	ttsGenerator := elevenlabs.NewGenerator(mustEnv("ELEVENLABS_API_KEY"))

	worker, err := rabbitmq.NewWorker(channel, scriptRepo, audioFileRepo, storage, ttsGenerator)
```

Add a `mustEnv` helper next to the existing `envOr` at the bottom of the file — unlike
`RABBITMQ_URL`/`DATABASE_URL`, there's no sane default for an API key, so a missing one should fail fast
at startup rather than fail on the first job:

```go
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Add the secret to the Helm worker deployment**

In `deploy/helm/rio-backend/templates/worker-deployment.yaml`, add after the existing `S3_BUCKET` entry:

```yaml
            - name: ELEVENLABS_API_KEY
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: elevenlabs-api-key } }
```

- [ ] **Step 4: Verify the chart still renders**

Run: `helm template deploy/helm/rio-backend`
Expected: no errors; `ELEVENLABS_API_KEY` appears in the rendered `worker` Deployment's env, absent from
the `api` Deployment's env.

- [ ] **Step 5: Commit**

```bash
git add cmd/worker/main.go deploy/helm/rio-backend/templates/worker-deployment.yaml
git commit -m "Wire real ElevenLabs generator into cmd/worker"
```

---

### Task 6: `PlaceRepository.FindByName`

**Worktree:** `backend`

**Files:**
- Modify: `internal/ports/place_repository.go`
- Modify: `internal/adapters/postgres/place_repository.go`
- Modify: `internal/adapters/postgres/place_repository_test.go`

**Interfaces:**
- Produces: `PlaceRepository.FindByName(ctx, name string) (*domain.Place, error)` — added to the
  `ports.PlaceRepository` interface and implemented in the Postgres adapter. Needed by Task 7 so the
  import CLI can check whether a `Place` was already imported before creating a duplicate.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/postgres/place_repository_test.go`:

```go
func TestPlaceRepository_FindByName(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	name, err := domain.NewPlaceName("Escadaria Selarón")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	coords, err := domain.NewCoordinates(-22.9147, -43.1806)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	place := domain.NewPlace(name, "monument", coords, "", "overture", "correct")
	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	found, err := repo.FindByName(ctx, "Escadaria Selarón")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != place.ID() {
		t.Fatalf("got ID %q, want %q", found.ID(), place.ID())
	}

	if _, err := repo.FindByName(ctx, "does not exist"); err == nil {
		t.Fatal("expected an error for an unknown name, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestPlaceRepository_FindByName -v`
Expected: FAIL — compilation error, `FindByName` undefined on `*PlaceRepository`.

- [ ] **Step 3: Write minimal implementation**

In `internal/ports/place_repository.go`, add `FindByName` to the interface:

```go
type PlaceRepository interface {
	Save(ctx context.Context, place *domain.Place) error
	FindByID(ctx context.Context, id string) (*domain.Place, error)
	FindByName(ctx context.Context, name string) (*domain.Place, error)
	FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error)
}
```

In `internal/adapters/postgres/place_repository.go`, add the implementation (next to `FindByID`):

```go
func (r *PlaceRepository) FindByName(ctx context.Context, name string) (*domain.Place, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places WHERE name = $1
	`, name)
	return scanPlace(row)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: PASS — requires a local Postgres with PostGIS and the schema applied (see existing
`place_repository_test.go` tests for the same requirement).

- [ ] **Step 5: Commit**

```bash
git add internal/ports/place_repository.go internal/adapters/postgres/place_repository.go internal/adapters/postgres/place_repository_test.go
git commit -m "Add PlaceRepository.FindByName"
```

---

### Task 7: `cmd/import` — CSV-to-Postgres import CLI

**Worktree:** `backend`

**Files:**
- Create: `cmd/import/join.go`
- Create: `cmd/import/join_test.go`
- Create: `cmd/import/csv.go`
- Create: `cmd/import/main.go`

**Interfaces:**
- Consumes: `postgres.NewPlaceRepository`, `postgres.NewScriptRepository`, `PlaceRepository.FindByName`
  (Task 6), `domain.NewPlace`/`domain.NewScript`/`domain.NewPlaceName`/`domain.NewCoordinates`/
  `domain.NewWikidataQID`/`domain.NewLanguage`/`domain.NewScriptText`.
- Produces: `buildImportPlan(places []placeRow, narrations []narrationRow) ([]placeRow,
  []scriptToImport)` — pure, tested in isolation from Postgres/CSV I/O.

- [ ] **Step 1: Write the failing test for the pure join logic**

```go
// cmd/import/join_test.go
package main

import "testing"

func TestBuildImportPlan(t *testing.T) {
	places := []placeRow{
		{Name: "Cristo Redentor", Category: "monument", Source: "wikidata", Lat: -22.9519, Lon: -43.2105, WikidataQID: "Q1963380"},
		{Name: "Lugar sans narration", Category: "monument", Source: "overture", Lat: -22.9, Lon: -43.2},
	}
	narrations := []narrationRow{
		{Name: "Cristo Redentor", FR: "Texte FR", EN: "Text EN", ES: "", PT: "Texto PT"},
		{Name: "Lieu inconnu", FR: "Orphelin"},
	}

	matchedPlaces, scripts := buildImportPlan(places, narrations)

	if len(matchedPlaces) != 1 || matchedPlaces[0].Name != "Cristo Redentor" {
		t.Fatalf("got matched places %+v, want only Cristo Redentor", matchedPlaces)
	}

	want := []scriptToImport{
		{PlaceName: "Cristo Redentor", Language: "fr", Text: "Texte FR"},
		{PlaceName: "Cristo Redentor", Language: "en", Text: "Text EN"},
		{PlaceName: "Cristo Redentor", Language: "pt", Text: "Texto PT"},
	}
	if len(scripts) != len(want) {
		t.Fatalf("got %d scripts, want %d: %+v", len(scripts), len(want), scripts)
	}
	for i, s := range scripts {
		if s != want[i] {
			t.Fatalf("script %d: got %+v, want %+v", i, s, want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/import/... -v`
Expected: FAIL — compilation error, `buildImportPlan`/`placeRow`/`narrationRow`/`scriptToImport`
undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// cmd/import/join.go
package main

type placeRow struct {
	Name        string
	Category    string
	Source      string
	Lat, Lon    float64
	WikidataQID string
}

type narrationRow struct {
	Name           string
	FR, EN, ES, PT string
}

type scriptToImport struct {
	PlaceName string
	Language  string
	Text      string
}

// buildImportPlan croise les lieux sourcés et les narrations traduites sur le
// nom du lieu. Une narration sans lieu correspondant est ignorée (orpheline —
// on ne crée jamais un Place à partir d'une narration seule). Un lieu sans
// narration n'est simplement pas importé : rien à publier pour lui pour
// l'instant.
func buildImportPlan(places []placeRow, narrations []narrationRow) ([]placeRow, []scriptToImport) {
	byName := make(map[string]placeRow, len(places))
	for _, p := range places {
		byName[p.Name] = p
	}

	var matchedPlaces []placeRow
	var scripts []scriptToImport
	for _, n := range narrations {
		place, ok := byName[n.Name]
		if !ok {
			continue
		}
		matchedPlaces = append(matchedPlaces, place)

		languages := []struct{ code, text string }{
			{"fr", n.FR}, {"en", n.EN}, {"es", n.ES}, {"pt", n.PT},
		}
		for _, l := range languages {
			if l.text == "" {
				continue
			}
			scripts = append(scripts, scriptToImport{PlaceName: n.Name, Language: l.code, Text: l.text})
		}
	}
	return matchedPlaces, scripts
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/import/... -v`
Expected: PASS

- [ ] **Step 5: Commit the pure logic**

```bash
git add cmd/import/join.go cmd/import/join_test.go
git commit -m "Add pure CSV-join logic for the Postgres import"
```

- [ ] **Step 6: Write the CSV readers**

```go
// cmd/import/csv.go
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

func columnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[col] = i
	}
	return idx
}

func readPlacesCSV(path string) ([]placeRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := columnIndex(header)

	var rows []placeRow
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lat, err := strconv.ParseFloat(record[idx["lat"]], 64)
		if err != nil {
			return nil, fmt.Errorf("parse lat for %q: %w", record[idx["name"]], err)
		}
		lon, err := strconv.ParseFloat(record[idx["lon"]], 64)
		if err != nil {
			return nil, fmt.Errorf("parse lon for %q: %w", record[idx["name"]], err)
		}
		rows = append(rows, placeRow{
			Name:        record[idx["name"]],
			Category:    record[idx["category"]],
			Source:      record[idx["source"]],
			Lat:         lat,
			Lon:         lon,
			WikidataQID: record[idx["wikidata_qid"]],
		})
	}
	return rows, nil
}

func readNarrationsCSV(path string) ([]narrationRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := columnIndex(header)

	var rows []narrationRow
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, narrationRow{
			Name: record[idx["name"]],
			FR:   record[idx["narration_fr"]],
			EN:   record[idx["narration_en"]],
			ES:   record[idx["narration_es"]],
			PT:   record[idx["narration_pt"]],
		})
	}
	return rows, nil
}
```

- [ ] **Step 7: Write `main.go`**

```go
// cmd/import/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/domain"
)

func main() {
	placesPath := flag.String("places", "", "path to places_clean_vN.csv")
	narrationsPath := flag.String("narrations", "", "path to narrations_multi_full.csv")
	dryRun := flag.Bool("dry-run", false, "parse and report counts without writing to Postgres")
	flag.Parse()

	if *placesPath == "" || *narrationsPath == "" {
		log.Fatal("both -places and -narrations are required")
	}

	places, err := readPlacesCSV(*placesPath)
	if err != nil {
		log.Fatalf("read places csv: %v", err)
	}
	narrations, err := readNarrationsCSV(*narrationsPath)
	if err != nil {
		log.Fatalf("read narrations csv: %v", err)
	}

	matchedPlaces, scripts := buildImportPlan(places, narrations)
	log.Printf("matched %d places with narrations, %d scripts to import", len(matchedPlaces), len(scripts))

	if *dryRun {
		return
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)

	placeIDs := make(map[string]string, len(matchedPlaces))
	for _, p := range matchedPlaces {
		if existing, err := placeRepo.FindByName(ctx, p.Name); err == nil {
			placeIDs[p.Name] = existing.ID()
			continue
		}

		name, err := domain.NewPlaceName(p.Name)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}
		coords, err := domain.NewCoordinates(p.Lat, p.Lon)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}
		qid, err := domain.NewWikidataQID(p.WikidataQID)
		if err != nil {
			log.Printf("skip place %q: %v", p.Name, err)
			continue
		}

		place := domain.NewPlace(name, p.Category, coords, qid, p.Source, "")
		if err := placeRepo.Save(ctx, place); err != nil {
			log.Printf("save place %q: %v", p.Name, err)
			continue
		}
		placeIDs[p.Name] = place.ID()
	}

	imported := 0
	for _, s := range scripts {
		placeID, ok := placeIDs[s.PlaceName]
		if !ok {
			continue // le lieu correspondant a échoué plus haut — pas de script orphelin
		}

		language, err := domain.NewLanguage(s.Language)
		if err != nil {
			log.Printf("skip script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}
		text, err := domain.NewScriptText(s.Text)
		if err != nil {
			log.Printf("skip script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}

		script := domain.NewScript(placeID, language, text, "")
		if err := scriptRepo.Save(ctx, script); err != nil {
			if isUniqueViolation(err) {
				log.Printf("script %q/%s already imported, skipping", s.PlaceName, s.Language)
				continue
			}
			log.Printf("save script %q/%s: %v", s.PlaceName, s.Language, err)
			continue
		}
		imported++
	}

	log.Printf("imported %d places, %d scripts", len(placeIDs), imported)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 8: Verify it builds and dry-runs against fixture CSVs**

Run: `go build ./...`
Expected: no errors.

Run against small fixture files to sanity-check the wiring without touching Postgres:

```bash
mkdir -p /tmp/import-fixture
cat > /tmp/import-fixture/places.csv <<'EOF'
name,category,source,lat,lon,wikidata_qid
Cristo Redentor,monument,wikidata,-22.9519,-43.2105,Q1963380
EOF
cat > /tmp/import-fixture/narrations.csv <<'EOF'
name,narration_fr,narration_en,narration_es,narration_pt
Cristo Redentor,Texte FR,Text EN,Texto ES,Texto PT
EOF
go run ./cmd/import -places=/tmp/import-fixture/places.csv -narrations=/tmp/import-fixture/narrations.csv -dry-run
```

Expected output: `matched 1 places with narrations, 4 scripts to import`

- [ ] **Step 9: Commit**

```bash
git add cmd/import/csv.go cmd/import/main.go
git commit -m "Add cmd/import: CSV-to-Postgres import CLI"
```

---

### Task 8: One-off ElevenLabs voice-cloning script

**Worktree:** `sourcing-pipeline`

**Files:**
- Create: `pipeline/curation/clone_voice.py`

No test — matches the existing `pipeline/curation/` convention (one-off/ad-hoc scripts, no test
coverage, per `CLAUDE.md`).

- [ ] **Step 1: Write the script**

```python
"""One-off admin script: clone a voice on ElevenLabs from a local audio sample.

Usage:
    ELEVENLABS_API_KEY=... python clone_voice.py --name "Julie" --sample /path/to/sample.mp3

Prints the resulting voice_id, to be stored in config/secrets and passed manually
later at script-review time (POST /scripts/{id}/review) — this script has no other
effect and is not called by the import or the TTS worker.

1-2 minutes of clean audio (quiet room, consistent mic) in a single language is
enough: ElevenLabs' multilingual model speaks the cloned voice in 32+ languages
from translated text, it does not need one sample per language.
"""
import argparse
import os
import sys

import requests

API_URL = "https://api.elevenlabs.io/v1/voices/add"


def clone_voice(api_key, name, sample_path):
    with open(sample_path, "rb") as f:
        response = requests.post(
            API_URL,
            headers={"xi-api-key": api_key},
            data={"name": name},
            files={"files": (os.path.basename(sample_path), f)},
            timeout=60,
        )
    response.raise_for_status()
    return response.json()["voice_id"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--name", required=True, help="Name to give the cloned voice on ElevenLabs")
    parser.add_argument("--sample", required=True, help="Path to a clean audio sample (1-2 min, wav/mp3)")
    args = parser.parse_args()

    api_key = os.environ.get("ELEVENLABS_API_KEY")
    if not api_key:
        print("ELEVENLABS_API_KEY is not set", file=sys.stderr)
        sys.exit(1)

    voice_id = clone_voice(api_key, args.name, args.sample)
    print(f"voice_id: {voice_id}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Verify it runs (manual, requires a real sample + API key)**

Run: `ELEVENLABS_API_KEY=<key> python pipeline/curation/clone_voice.py --name "Test" --sample
/path/to/sample.mp3`
Expected: prints a line `voice_id: <some id>`. (Skip this step in CI/agentic execution if no sample file
or API key is available — flag it for the user to run manually instead of fabricating a result.)

- [ ] **Step 3: Commit**

```bash
git add pipeline/curation/clone_voice.py
git commit -m "Add one-off ElevenLabs voice-cloning script"
```
