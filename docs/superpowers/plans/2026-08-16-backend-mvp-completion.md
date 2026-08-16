# Backend MVP Completion Implementation Plan

> **Note — mode de collaboration révisé.** Par défaut, l'autrice écrit elle-même chaque tâche, avec
> l'aide de Claude (explications, relecture, débogage). Claude n'écrit dans les fichiers `.go` que si
> elle le demande explicitement, tâche par tâche — pas par défaut, contrairement au plan
> `2026-08-12-backend-domain-model.md` qui interdisait toute écriture. Steps en `- [ ]` pour suivre ta
> propre progression.

**Goal:** Compléter le backend pour un MVP montrable — worker RabbitMQ, stockage S3, API HTTP, import
des données du pipeline, CI/CD, manifests K8s (écrits, pas déployés).

**Architecture:** Suite directe de `2026-08-12-backend-domain-model.md` (domaine/ports/application/
Postgres déjà faits). Ajoute les adaptateurs restants (RabbitMQ complet, S3), une API HTTP (Echo) comme
adaptateur entrant, et l'outillage (import, CI, déploiement).

**Tech Stack:** Go 1.25, `github.com/rabbitmq/amqp091-go`, `github.com/aws/aws-sdk-go-v2` (+`config`,
`credentials`, `service/s3`), `github.com/labstack/echo/v4`, un vrai bucket AWS S3 (tests, révisé
2026-08-16 — plus LocalStack/MinIO), GitHub Actions.

## Global Constraints

- Le worker RabbitMQ et l'API HTTP sont **deux binaires séparés** (`cmd/worker`, `cmd/api`), pas un seul
  processus avec une goroutine — nécessaire pour que le HPA (API) et le KEDA ScaledObject (worker)
  puissent scaler indépendamment (sous-système 6 du spec).
- Le stub de génération TTS reste un stub explicite, jamais présenté comme une vraie intégration
  ElevenLabs.
- `ports.AudioStorage` est un nouveau port, à ajouter dans `internal/ports/` avant l'adaptateur S3.
- Le script d'import lit `narrations_export.json` — généré par un export Python **hors de ce plan**
  (voir Tâche 8, prérequis manuel).
- K8s : manifests écrits et versionnés, **pas déployés** dans ce plan.

---

## File Structure

```
internal/
  ports/
    audio_storage.go                  (nouveau)
  adapters/
    rabbitmq/
      audio_job_publisher.go          audio_job_publisher_test.go
      worker.go                       worker_test.go
    s3/
      audio_storage.go                audio_storage_test.go
    http/
      server.go
      places_handler.go
      scripts_handler.go              server_test.go
cmd/
  api/
    main.go                           (HTTP uniquement)
  worker/
    main.go                           (worker uniquement, nouveau binaire)
  import/
    main.go
.github/
  workflows/
    backend-ci.yml
deploy/
  docker/
    Dockerfile.api
    Dockerfile.worker
  k8s/
    api-deployment.yaml   api-service.yaml   api-hpa.yaml
    worker-deployment.yaml   worker-scaledobject.yaml
```

---

### Task 1: Port `AudioStorage`

**Files:**
- Modify: `internal/ports/audio_storage.go`

**Interfaces:**
- Produces: `AudioStorage` interface, `Upload(ctx, key string, data []byte, contentType string) (url string, err error)`.

- [ ] **Step 1: Écrire l'interface**

```go
// internal/ports/audio_storage.go
package ports

import "context"

// AudioStorage est le port sortant vers le stockage objet (S3) — implémenté
// par l'adaptateur s3, Tâche 4.
type AudioStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (url string, err error)
}
```

- [ ] **Step 2: Vérifier que ça compile**

Run: `go build ./internal/ports/...`
Expected: aucune sortie, code de sortie 0.

- [ ] **Step 3: Commit**

```bash
git add internal/ports/audio_storage.go
git commit -m "Add AudioStorage port"
```

---

### Task 2: Adaptateur RabbitMQ — publisher

**Files:**
- Modify: `internal/adapters/rabbitmq/audio_job_publisher.go`
- Modify: `internal/adapters/rabbitmq/audio_job_publisher_test.go`

**Interfaces:**
- Produces: `TTSJobQueue` (const), `ttsJobMessage` (type interne, réutilisé par le worker — Tâche 3, même
  package), `NewAudioJobPublisher(channel *amqp.Channel) (*AudioJobPublisher, error)` satisfaisant
  `ports.AudioJobPublisher`.

Nécessite RabbitMQ local : `docker run --rm -d --name rio-rabbitmq -p 5672:5672 -p 15672:15672
rabbitmq:3-management`

- [ ] **Step 1: Ajouter la dépendance**

```bash
go get github.com/rabbitmq/amqp091-go
```

- [ ] **Step 2: Écrire le test qui échoue**

```go
// internal/adapters/rabbitmq/audio_job_publisher_test.go
//go:build integration

package rabbitmq

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func testChannel(t *testing.T) *amqp.Channel {
	t.Helper()
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	channel, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	t.Cleanup(func() { _ = channel.Close() })
	return channel
}

func TestAudioJobPublisher_PublishTTSJob(t *testing.T) {
	channel := testChannel(t)

	publisher, err := NewAudioJobPublisher(channel)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	if err := publisher.PublishTTSJob(context.Background(), "audio-1", "script-1", "Texte", "fr", "voice-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	msgs, err := channel.Consume(TTSJobQueue, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	select {
	case msg := <-msgs:
		if len(msg.Body) == 0 {
			t.Fatal("got empty message body")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published message")
	}
}
```

- [ ] **Step 3: Vérifier que ça échoue**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: échec de compilation — `NewAudioJobPublisher`/`TTSJobQueue` non définis.

- [ ] **Step 4: Écrire l'implémentation**

```go
// internal/adapters/rabbitmq/audio_job_publisher.go
package rabbitmq

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

const TTSJobQueue = "tts_jobs"

type ttsJobMessage struct {
	AudioFileID string `json:"audio_file_id"`
	ScriptID    string `json:"script_id"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	VoiceID     string `json:"voice_id"`
}

type AudioJobPublisher struct {
	channel *amqp.Channel
}

func NewAudioJobPublisher(channel *amqp.Channel) (*AudioJobPublisher, error) {
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &AudioJobPublisher{channel: channel}, nil
}

func (p *AudioJobPublisher) PublishTTSJob(ctx context.Context, audioFileID, scriptID, text, language, voiceID string) error {
	body, err := json.Marshal(ttsJobMessage{
		AudioFileID: audioFileID,
		ScriptID:    scriptID,
		Text:        text,
		Language:    language,
		VoiceID:     voiceID,
	})
	if err != nil {
		return err
	}
	return p.channel.PublishWithContext(ctx, "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}
```

- [ ] **Step 5: Vérifier que ça passe**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/rabbitmq/audio_job_publisher.go internal/adapters/rabbitmq/audio_job_publisher_test.go go.mod go.sum
git commit -m "Add AudioJobPublisher RabbitMQ adapter"
```

---

### Task 3: Adaptateur RabbitMQ — worker (rôle entrant)

**Files:**
- Modify: `internal/adapters/rabbitmq/worker.go`
- Modify: `internal/adapters/rabbitmq/worker_test.go`

**Interfaces:**
- Consumes: `ttsJobMessage`, `TTSJobQueue` (Tâche 2, même package) ; `application.StartAudioGeneration`,
  `application.CompleteAudioGeneration` ; `ports.ScriptRepository`, `ports.AudioFileRepository`,
  `ports.AudioStorage`.
- Produces: `NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo
  ports.AudioFileRepository, storage ports.AudioStorage) (*Worker, error)`, `(*Worker).Run(ctx) error`.

- [ ] **Step 1: Écrire le test qui échoue**

```go
// internal/adapters/rabbitmq/worker_test.go
//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/domain"
)

type fakeScriptRepo struct {
	mu      sync.Mutex
	scripts map[string]*domain.Script
}

func newFakeScriptRepo() *fakeScriptRepo { return &fakeScriptRepo{scripts: map[string]*domain.Script{}} }

func (f *fakeScriptRepo) Save(_ context.Context, s *domain.Script) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts[s.ID()] = s
	return nil
}

func (f *fakeScriptRepo) FindByID(_ context.Context, id string) (*domain.Script, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.scripts[id]
	if !ok {
		return nil, errors.New("script not found")
	}
	return s, nil
}

type fakeAudioFileRepo struct {
	mu    sync.Mutex
	files map[string]*domain.AudioFile
}

func newFakeAudioFileRepo() *fakeAudioFileRepo {
	return &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
}

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[a.ID()] = a
	return nil
}

func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.files[id]
	if !ok {
		return nil, errors.New("audio file not found")
	}
	return a, nil
}

type fakeStorage struct{}

func (fakeStorage) Upload(_ context.Context, key string, _ []byte, _ string) (string, error) {
	return "fake://bucket/" + key, nil
}

func TestWorker_ProcessesJobEndToEnd(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Le Cristo Redentor...")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	if err := script.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = worker.Run(ctx) }()

	body, _ := json.Marshal(ttsJobMessage{
		AudioFileID: audioFile.ID(),
		ScriptID:    script.ID(),
		Text:        "Le Cristo Redentor...",
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
			if err == nil && found.Status() == domain.AudioFileStatusReady {
				savedScript, _ := scriptRepo.FindByID(context.Background(), script.ID())
				if savedScript.Status() != domain.ScriptStatusPublished {
					t.Fatalf("got script status %v, want published", savedScript.Status())
				}
				return
			}
		}
	}
}
```

- [ ] **Step 2: Vérifier que ça échoue**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -run TestWorker -v`
Expected: échec de compilation — `NewWorker` non défini.

- [ ] **Step 3: Écrire l'implémentation**

```go
// internal/adapters/rabbitmq/worker.go
package rabbitmq

import (
	"context"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/ports"
)

type Worker struct {
	channel       *amqp.Channel
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	storage       ports.AudioStorage
}

func NewWorker(channel *amqp.Channel, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, storage ports.AudioStorage) (*Worker, error) {
	if _, err := channel.QueueDeclare(TTSJobQueue, true, false, false, false, nil); err != nil {
		return nil, err
	}
	return &Worker{channel: channel, scriptRepo: scriptRepo, audioFileRepo: audioFileRepo, storage: storage}, nil
}

// Run consomme tts_jobs jusqu'à annulation du ctx. Bloquant — à lancer dans
// sa propre goroutine ou son propre binaire (cmd/worker, Tâche 7).
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
		_ = msg.Nack(false, false) // malformé — ne jamais requeue, boucle infinie sinon
		return
	}

	if err := application.StartAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID); err != nil {
		log.Printf("tts worker: start generation failed for %s: %v", job.AudioFileID, err)
		_ = msg.Nack(false, true) // requeue — probablement transitoire
		return
	}

	audioBytes, duration := generateAudioStub(job.Text)
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

// generateAudioStub remplace le vrai appel TTS (ElevenLabs) — pas construit ici,
// nécessiterait une clé API et sa propre conception. Renvoie un résultat
// plausible mais factice pour exercer réellement le reste du pipeline (upload,
// Postgres, transition Script→published).
func generateAudioStub(text string) (audioBytes []byte, duration time.Duration) {
	wordCount := len(text) / 5
	duration = time.Duration(wordCount) * 400 * time.Millisecond
	return []byte("STUB-AUDIO:" + text), duration
}
```

- [ ] **Step 4: Vérifier que ça passe**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: PASS (publisher + worker).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/rabbitmq/worker.go internal/adapters/rabbitmq/worker_test.go
git commit -m "Add RabbitMQ worker: consumes tts_jobs, drives the domain workflow"
```

---

### Task 4: Adaptateur S3

**Files:**
- Modify: `internal/adapters/s3/audio_storage.go`
- Modify: `internal/adapters/s3/audio_storage_test.go`

**Interfaces:**
- Produces: `NewAudioStorage(client *s3.Client, bucket string) *AudioStorage` satisfaisant `ports.AudioStorage`.

**Révisé le 2026-08-16 : vrai bucket AWS S3 (`rio-audioguide-bucket`), plus LocalStack/MinIO** —
pratique AWS réelle, cohérent avec la maîtrise visée. Identifiants dans l'environnement du terminal
(`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN`/`AWS_REGION`), jamais dans le code ni
codés en dur — le SDK les lit automatiquement via `config.LoadDefaultConfig`.

- [ ] **Step 1: Ajouter les dépendances**

```bash
go get github.com/aws/aws-sdk-go-v2/aws github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/s3
```

- [ ] **Step 2: Écrire le test qui échoue**

```go
// internal/adapters/s3/audio_storage_test.go
//go:build integration

package s3

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func testClient(t *testing.T) *s3.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return s3.NewFromConfig(cfg)
}

func testBucket(t *testing.T) string {
	t.Helper()
	bucket := os.Getenv("S3_TEST_BUCKET")
	if bucket == "" {
		t.Skip("S3_TEST_BUCKET not set — skipping real-S3 integration test")
	}
	return bucket
}

func TestAudioStorage_Upload(t *testing.T) {
	client := testClient(t)
	bucket := testBucket(t)

	storage := NewAudioStorage(client, bucket)
	url, err := storage.Upload(context.Background(), "test/audio.mp3", []byte("fake audio bytes"), "audio/mpeg")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	want := "s3://" + bucket + "/test/audio.mp3"
	if url != want {
		t.Fatalf("got %q, want %q", url, want)
	}
}
```

Pas de `CreateBucket` dans le test — le bucket existe déjà, créé à la main dans la console AWS.
`testBucket` lit `S3_TEST_BUCKET` (à exporter toi-même, ex. `export S3_TEST_BUCKET=rio-audioguide-bucket`)
plutôt que de coder le nom en dur dans le test.

- [ ] **Step 3: Vérifier que ça échoue**

Run: `go test -tags=integration ./internal/adapters/s3/... -v`
Expected: échec de compilation — `NewAudioStorage` non défini.

- [ ] **Step 4: Écrire l'implémentation**

```go
// internal/adapters/s3/audio_storage.go
package s3

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AudioStorage struct {
	client *s3.Client
	bucket string
}

func NewAudioStorage(client *s3.Client, bucket string) *AudioStorage {
	return &AudioStorage{client: client, bucket: bucket}
}

func (a *AudioStorage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, key), nil
}
```

- [ ] **Step 5: Vérifier que ça passe**

Run: `go test -tags=integration ./internal/adapters/s3/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/s3/ go.mod go.sum
git commit -m "Add S3 audio storage adapter"
```

---

### Task 5: API HTTP (Echo)

**Files:**
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/places_handler.go`
- Modify: `internal/adapters/http/scripts_handler.go`
- Modify: `internal/adapters/http/server_test.go`

**Interfaces:**
- Consumes: `ports.PlaceRepository`, `ports.ScriptRepository`, `ports.AudioFileRepository`,
  `ports.AudioJobPublisher`, `application.ReviewAndRequestAudio`.
- Produces: `NewServer(placeRepo, scriptRepo, audioFileRepo, publisher) *Server`, `(*Server).Start(addr string) error`.

- [ ] **Step 1: Ajouter la dépendance**

```bash
go get github.com/labstack/echo/v4
```

- [ ] **Step 2: Écrire le test qui échoue**

```go
// internal/adapters/http/server_test.go
package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

type fakePlaceRepo struct{ places []*domain.Place }

func (f *fakePlaceRepo) Save(_ context.Context, _ *domain.Place) error { return nil }
func (f *fakePlaceRepo) FindByID(_ context.Context, _ string) (*domain.Place, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakePlaceRepo) FindActiveInBoundingBox(_ context.Context, _, _, _, _ float64) ([]*domain.Place, error) {
	return f.places, nil
}

type fakeScriptRepo struct{ scripts map[string]*domain.Script }

func (f *fakeScriptRepo) Save(_ context.Context, s *domain.Script) error {
	f.scripts[s.ID()] = s
	return nil
}
func (f *fakeScriptRepo) FindByID(_ context.Context, id string) (*domain.Script, error) {
	s, ok := f.scripts[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}

type fakeAudioFileRepo struct{ files map[string]*domain.AudioFile }

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.files[a.ID()] = a
	return nil
}
func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	a, ok := f.files[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

type fakePublisher struct{ published int }

func (f *fakePublisher) PublishTTSJob(_ context.Context, _, _, _, _, _ string) error {
	f.published++
	return nil
}

func TestListPlaces(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{})

	req := httptest.NewRequest(http.MethodGet, "/places", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cristo Redentor") {
		t.Fatalf("expected response to contain place name, got %s", rec.Body.String())
	}
}

func TestReviewScript(t *testing.T) {
	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
	publisher := &fakePublisher{}
	server := NewServer(&fakePlaceRepo{}, scriptRepo, audioFileRepo, publisher)

	body := strings.NewReader(`{"reviewer":"julie","voice_id":"voice-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/scripts/"+script.ID()+"/review", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if publisher.published != 1 {
		t.Fatalf("got %d published jobs, want 1", publisher.published)
	}
}
```

- [ ] **Step 3: Vérifier que ça échoue**

Run: `go test ./internal/adapters/http/... -v`
Expected: échec de compilation — `NewServer` non défini.

- [ ] **Step 4: Écrire l'implémentation**

```go
// internal/adapters/http/server.go
package http

import (
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/ports"
)

type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	publisher     ports.AudioJobPublisher
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		publisher:     publisher,
	}
	s.echo.GET("/places", s.listPlaces)
	s.echo.POST("/scripts/:id/review", s.reviewScript)
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
```

```go
// internal/adapters/http/places_handler.go
package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Rio de Janeiro municipality, approximativement — suffisant pour un endpoint
// de démo "liste tout" ; un vrai produit paginerait ou accepterait des bornes
// en paramètre.
const (
	rioMinLat, rioMinLon = -23.1, -43.8
	rioMaxLat, rioMaxLon = -22.7, -43.0
)

type placeResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

func (s *Server) listPlaces(c echo.Context) error {
	places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	resp := make([]placeResponse, 0, len(places))
	for _, p := range places {
		resp = append(resp, placeResponse{
			ID:       p.ID(),
			Name:     p.Name().String(),
			Category: p.Category(),
			Lat:      p.Coordinates().Lat(),
			Lon:      p.Coordinates().Lon(),
		})
	}
	return c.JSON(http.StatusOK, resp)
}
```

```go
// internal/adapters/http/scripts_handler.go
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
```

- [ ] **Step 5: Vérifier que ça passe**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/ go.mod go.sum
git commit -m "Add minimal HTTP API: GET /places, POST /scripts/{id}/review"
```

---

### Task 6: `cmd/api/main.go` — serveur HTTP

**Files:**
- Modify: `cmd/api/main.go`

**Interfaces:**
- Consumes : tous les constructeurs des Tâches 2 (publisher), 4 n'est pas nécessaire ici (S3 est côté
  worker uniquement), 5 (`http.NewServer`), plus `postgres.NewPlaceRepository`/`NewScriptRepository`/
  `NewAudioFileRepository` (déjà écrits).

- [ ] **Step 1: Écrire `main.go`**

```go
// cmd/api/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	httpadapter "rioaudioguide/backend/internal/adapters/http"
	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	conn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("open rabbitmq channel: %v", err)
	}
	defer channel.Close()

	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)

	publisher, err := rabbitmq.NewAudioJobPublisher(channel)
	if err != nil {
		log.Fatalf("set up audio job publisher: %v", err)
	}

	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, publisher)
	log.Println("api ready, listening on :8080")
	if err := server.Start(":8080"); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Lancer contre les conteneurs des Tâches 2/4**

```bash
go run ./cmd/api
```

Expected : `api ready, listening on :8080`. Dans un autre terminal, `curl localhost:8080/places` doit
répondre `[]` (base vide) sans erreur.

- [ ] **Step 3: Commit**

```bash
git add cmd/api/main.go
git commit -m "Wire cmd/api: HTTP server only, no worker (separate binary)"
```

---

### Task 7: `cmd/worker/main.go` — worker RabbitMQ

**Files:**
- Modify: `cmd/worker/main.go` (nouveau dossier `cmd/worker/`)

**Interfaces:**
- Consumes : `rabbitmq.NewWorker` (Tâche 3), `s3.NewAudioStorage` (Tâche 4), les repositories Postgres.

Binaire **séparé** de `cmd/api` — nécessaire pour que le worker scale indépendamment (KEDA, Tâche 10) de
l'API (HPA, Tâche 10).

- [ ] **Step 1: Écrire `main.go`**

```go
// cmd/worker/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
	"rioaudioguide/backend/internal/adapters/s3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	conn, err := amqp.Dial(envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"))
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("open rabbitmq channel: %v", err)
	}
	defer channel.Close()

	// Vrai AWS : LoadDefaultConfig lit AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/
	// AWS_SESSION_TOKEN/AWS_REGION depuis l'environnement (ou ~/.aws/credentials)
	// automatiquement — rien à coder en dur, pas d'endpoint local à forcer.
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audioguide-bucket"))

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)

	worker, err := rabbitmq.NewWorker(channel, scriptRepo, audioFileRepo, storage)
	if err != nil {
		log.Fatalf("set up worker: %v", err)
	}

	log.Println("worker ready, consuming tts_jobs")
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("worker stopped unexpectedly: %v", err)
	}
	log.Println("worker shut down cleanly")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Lancer contre Postgres, RabbitMQ (conteneurs) et le vrai bucket S3**

```bash
go run ./cmd/worker
```

Expected : `worker ready, consuming tts_jobs`. Publier un job de test (via l'API `POST /scripts/{id}/review`
sur un script existant en base) doit faire apparaître les logs de traitement et faire passer le Script en
`published` (vérifiable via Postgres).

- [ ] **Step 3: Commit**

```bash
git add cmd/worker/
git commit -m "Wire cmd/worker: separate binary consuming tts_jobs"
```

---

### Task 8: `cmd/import` — import pipeline → Postgres

**Prérequis manuel, hors Go** : exporter `narrations_data_part1-4.py` (listes Python littérales) vers un
JSON plat. Script Python à faire une fois côté pipeline, pas dans ce plan :

```python
# à exécuter dans pipeline/curation/, une fois
import json
from narrations_data_part1 import DATA_PART1
from narrations_data_part2 import DATA_PART2
from narrations_data_part3 import DATA_PART3
from narrations_data_part4 import DATA_PART4

entries = []
for part in (DATA_PART1, DATA_PART2, DATA_PART3, DATA_PART4):
    for place in part:
        for lang in ("fr", "en", "es", "pt"):
            if place.get(lang):
                entries.append({"place_id": place["id"], "language": lang, "text": place[lang]})

with open("narrations_export.json", "w") as f:
    json.dump(entries, f, ensure_ascii=False, indent=2)
```

**Files:**
- Modify: `cmd/import/main.go`
- Modify: `cmd/import/main_test.go`

**Interfaces:**
- Consumes: `domain.NewPlace`, `domain.NewScript` et Value Objects associés ; `postgres.PlaceRepository`,
  `postgres.ScriptRepository`.

- [ ] **Step 1: Écrire un test pour la seule fonction pure (`parseLatLon`)**

```go
// cmd/import/main_test.go
package main

import "testing"

func TestParseLatLon(t *testing.T) {
	lat, lon, err := parseLatLon("-22.9519", "-43.2105")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lat != -22.9519 || lon != -43.2105 {
		t.Fatalf("got (%v, %v), want (-22.9519, -43.2105)", lat, lon)
	}

	if _, _, err := parseLatLon("not-a-number", "-43.2105"); err == nil {
		t.Fatal("expected an error for a malformed latitude")
	}
}
```

Le reste (`importPlaces`/`importScripts`) est un outil d'import ponctuel, vérifié manuellement contre une
vraie base (Step 4) — même convention que les scripts `pipeline/curation/` côté Python, pas de suite de
tests automatisée complète pour un outil à usage unique.

- [ ] **Step 2: Vérifier que ça échoue**

Run: `go test ./cmd/import/... -v`
Expected: échec de compilation — `parseLatLon` non défini.

- [ ] **Step 3: Écrire l'implémentation**

```go
// cmd/import/main.go
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/domain"
)

type narrationEntry struct {
	PlaceID  string `json:"place_id"`
	Language string `json:"language"`
	Text     string `json:"text"`
}

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres"))
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	placeRepo := postgres.NewPlaceRepository(pool)
	scriptRepo := postgres.NewScriptRepository(pool)

	placesCSV := envOr("PLACES_CSV", "pipeline_data/places_cultural_v3.csv")
	narrationsJSON := envOr("NARRATIONS_JSON", "pipeline_data/narrations_export.json")

	placeIDByRawID, err := importPlaces(ctx, placesCSV, placeRepo)
	if err != nil {
		log.Fatalf("import places: %v", err)
	}
	log.Printf("imported %d places", len(placeIDByRawID))

	scriptCount, err := importScripts(ctx, narrationsJSON, placeIDByRawID, scriptRepo)
	if err != nil {
		log.Fatalf("import scripts: %v", err)
	}
	log.Printf("imported %d scripts", scriptCount)
}

func importPlaces(ctx context.Context, path string, repo *postgres.PlaceRepository) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	placeIDByRawID := map[string]string{}
	for _, row := range rows[1:] { // sauter l'en-tête
		rawID, name, category, source, latRaw, lonRaw, wikidataRaw := row[0], row[1], row[2], row[3], row[4], row[5], row[6]

		placeName, err := domain.NewPlaceName(name)
		if err != nil {
			log.Printf("skip %s: %v", rawID, err)
			continue
		}
		lat, lon, err := parseLatLon(latRaw, lonRaw)
		if err != nil {
			log.Printf("skip %s: %v", rawID, err)
			continue
		}
		coords, err := domain.NewCoordinates(lat, lon)
		if err != nil {
			log.Printf("skip %s: %v", rawID, err)
			continue
		}
		qid, err := domain.NewWikidataQID(wikidataRaw)
		if err != nil {
			log.Printf("skip %s: %v", rawID, err)
			continue
		}

		place := domain.NewPlace(placeName, category, coords, qid, source, "")
		if err := repo.Save(ctx, place); err != nil {
			return nil, err
		}
		placeIDByRawID[rawID] = place.ID()
	}
	return placeIDByRawID, nil
}

func parseLatLon(latRaw, lonRaw string) (float64, float64, error) {
	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return 0, 0, err
	}
	lon, err := strconv.ParseFloat(lonRaw, 64)
	if err != nil {
		return 0, 0, err
	}
	return lat, lon, nil
}

func importScripts(ctx context.Context, path string, placeIDByRawID map[string]string, repo *postgres.ScriptRepository) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var entries []narrationEntry
	if err := json.NewDecoder(f).Decode(&entries); err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		placeID, ok := placeIDByRawID[entry.PlaceID]
		if !ok {
			log.Printf("skip narration for unknown place %s", entry.PlaceID)
			continue
		}
		language, err := domain.NewLanguage(entry.Language)
		if err != nil {
			log.Printf("skip narration for %s: %v", entry.PlaceID, err)
			continue
		}
		text, err := domain.NewScriptText(entry.Text)
		if err != nil {
			log.Printf("skip narration for %s: %v", entry.PlaceID, err)
			continue
		}

		script := domain.NewScript(placeID, language, text, "")
		if err := repo.Save(ctx, script); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 4: Vérifier — test unitaire puis import réel**

Run: `go test ./cmd/import/... -v` → PASS.

Puis, avec Postgres réel + les fichiers du pipeline copiés/liés dans `pipeline_data/` :
```bash
go run ./cmd/import
```
Expected : logs `imported N places`, `imported M scripts`, `N`/`M` > 0. Vérifier avec `psql` que
`SELECT COUNT(*) FROM places;` et `SELECT COUNT(*) FROM scripts;` correspondent.

- [ ] **Step 5: Commit**

```bash
git add cmd/import/
git commit -m "Add pipeline-to-Postgres import command"
```

---

### Task 9: CI/CD — GitHub Actions

**Files:**
- Modify: `.github/workflows/backend-ci.yml`

- [ ] **Step 1: Écrire le workflow**

```yaml
# .github/workflows/backend-ci.yml
name: Backend CI

on:
  push:
    branches: [backend]
  pull_request:
    branches: [backend]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgis/postgis:16-3.4
        env:
          POSTGRES_PASSWORD: postgres
        ports: ["5432:5432"]
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      rabbitmq:
        image: rabbitmq:3-management
        ports: ["5672:5672"]
      localstack:
        image: localstack/localstack
        env:
          SERVICES: s3
        ports: ["4566:4566"]

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Apply schema
        run: |
          sudo apt-get update && sudo apt-get install -y postgresql-client
          PGPASSWORD=postgres psql -h localhost -U postgres -d postgres -f internal/adapters/postgres/schema.sql

      - name: Build
        run: go build ./...

      - name: Vet
        run: go vet ./...

      - name: Unit tests
        run: go test ./...

      - name: Integration tests
        env:
          TEST_DATABASE_URL: postgres://postgres:postgres@localhost:5432/postgres
        run: go test -tags=integration ./...
```

- [ ] **Step 2: Vérifier localement que les commandes du workflow fonctionnent**

Run (avec Postgres/RabbitMQ déjà lancés localement, et les identifiants AWS déjà exportés dans le terminal) :
```bash
go build ./... && go vet ./... && go test ./... && TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" go test -tags=integration ./...
```
Expected : tout passe, avant même de pousser sur GitHub.

- [ ] **Step 3: Commit et pousser pour voir le workflow tourner réellement**

```bash
git add .github/workflows/backend-ci.yml
git commit -m "Add GitHub Actions CI: build, vet, unit + integration tests"
git push -u origin backend
```

Vérifier l'onglet Actions du repo GitHub — c'est la vraie preuve, pas juste le fichier YAML.

---

### Task 10: Manifests Kubernetes (écrits, pas déployés)

**Files:**
- Modify: `deploy/docker/Dockerfile.api`
- Modify: `deploy/docker/Dockerfile.worker`
- Modify: `deploy/k8s/api-deployment.yaml`, `api-service.yaml`, `api-hpa.yaml`
- Modify: `deploy/k8s/worker-deployment.yaml`, `worker-scaledobject.yaml`

- [ ] **Step 1: Dockerfiles multi-stage**

```dockerfile
# deploy/docker/Dockerfile.api
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
```

```dockerfile
# deploy/docker/Dockerfile.worker
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/worker /worker
ENTRYPOINT ["/worker"]
```

- [ ] **Step 2: Vérifier que les images se construisent (sans déployer)**

Run: `docker build -f deploy/docker/Dockerfile.api -t rio-api:local .` puis pareil pour worker.
Expected : build réussi, deux images locales.

- [ ] **Step 3: Manifests API — Deployment, Service, HPA (scaling sur charge)**

```yaml
# deploy/k8s/api-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-api
spec:
  replicas: 2
  selector:
    matchLabels: { app: rio-api }
  template:
    metadata:
      labels: { app: rio-api }
    spec:
      containers:
        - name: api
          image: rio-api:local
          ports: [{ containerPort: 8080 }]
          env:
            - { name: DATABASE_URL, valueFrom: { secretKeyRef: { name: rio-backend-secrets, key: database-url } } }
            - { name: RABBITMQ_URL, valueFrom: { secretKeyRef: { name: rio-backend-secrets, key: rabbitmq-url } } }
```

```yaml
# deploy/k8s/api-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: rio-api
spec:
  selector: { app: rio-api }
  ports:
    - { port: 80, targetPort: 8080 }
```

```yaml
# deploy/k8s/api-hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: rio-api
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: rio-api }
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource: { name: cpu, target: { type: Utilization, averageUtilization: 70 } }
```

- [ ] **Step 4: Manifests worker — Deployment, ScaledObject KEDA (scaling sur profondeur de queue)**

```yaml
# deploy/k8s/worker-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-worker
spec:
  replicas: 1
  selector:
    matchLabels: { app: rio-worker }
  template:
    metadata:
      labels: { app: rio-worker }
    spec:
      containers:
        - name: worker
          image: rio-worker:local
          env:
            - { name: DATABASE_URL, valueFrom: { secretKeyRef: { name: rio-backend-secrets, key: database-url } } }
            - { name: RABBITMQ_URL, valueFrom: { secretKeyRef: { name: rio-backend-secrets, key: rabbitmq-url } } }
            - { name: S3_BUCKET, value: "rio-audio-guide" }
```

```yaml
# deploy/k8s/worker-scaledobject.yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: rio-worker
spec:
  scaleTargetRef: { name: rio-worker }
  minReplicaCount: 0
  maxReplicaCount: 10
  triggers:
    - type: rabbitmq
      metadata:
        queueName: tts_jobs
        mode: QueueLength
        value: "5" # 5 jobs en attente par réplique avant de scaler
      authenticationRef:
        name: rio-rabbitmq-auth
```

- [ ] **Step 5: Valider la syntaxe sans déployer**

Run: `kubectl apply --dry-run=client -f deploy/k8s/` (nécessite un contexte kubectl configuré, même
vide/local type kind — valide juste la syntaxe YAML/schema, ne contacte pas de vrai cluster de prod).
Expected : pas d'erreur de syntaxe. Si aucun contexte kubectl n'est disponible, `kubectl apply
--dry-run=client --validate=false -f deploy/k8s/ -o yaml` ou un linter YAML suffit pour cette étape.

- [ ] **Step 6: Commit**

```bash
git add deploy/
git commit -m "Add Dockerfiles and K8s manifests (API: HPA, worker: KEDA) — not deployed"
```

---

## Ce que ce plan ne couvre pas

- Déploiement réel sur un cluster K8s (les manifests sont écrits, vérifiés en syntaxe, jamais appliqués
  à un vrai cluster dans ce plan).
- Vraie intégration TTS (ElevenLabs) — le stub reste un stub.
- Authentification sur l'API HTTP.
- Secrets réels (`rio-backend-secrets` référencé mais jamais créé) — à faire au moment du déploiement réel.
