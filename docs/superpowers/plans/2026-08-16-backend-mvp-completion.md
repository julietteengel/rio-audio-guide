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

**Révisé le 2026-08-16** — structure en jobs parallèles puis séquentiels, inspirée de
`fiap-dclt-aula02/ci-multistage.yml` (cours FIAP de l'autrice), pas un seul job monolithique.

**Fait le 2026-08-16.** `backend-ci.yml` écrit, vérifié localement (les 5 commandes de chaque job
tournent en clair, y compris les tests d'intégration contre un vrai Postgres/RabbitMQ), commité.
`docker-build.yml` (push ECR) et `k8s-deploy.yml` (déploiement EKS via Helm) — hors scope initial de
cette tâche, prévus pour un cycle ultérieur — ont aussi été écrits le soir même (décision reprise en
session, voir `docs/superpowers/specs/2026-08-16-backend-mvp-completion-design.md` sous-système 5) :
tous deux syntaxiquement validés, aucun des deux encore exécuté en réel (`docker-build.yml` a besoin
des dépôts ECR `rio-api`/`rio-worker`, pas encore créés ; `k8s-deploy.yml` a besoin d'un vrai cluster
EKS, pas encore monté).

**Files:**
- Modify: `.github/workflows/backend-ci.yml`
- Create: `.github/workflows/docker-build.yml`
- Create: `.github/workflows/k8s-deploy.yml`

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
  # Étage 1 — en parallèle, aucun "needs" entre eux
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: Vet (vérifications basiques intégrées à Go)
        run: go vet ./...
      - name: golangci-lint (équivalent Go d'ESLint — bugs probables, style, code mort)
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: Unit tests
        run: go test ./...

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - name: Scan for known vulnerabilities
        run: govulncheck ./...

  # Étage 2 — dépend des trois précédents
  build:
    runs-on: ubuntu-latest
    needs: [lint, test, security]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: Build
        run: go build ./...

  # Étage 3 — dépend du build, tests d'intégration contre de vrais services
  integration-test:
    runs-on: ubuntu-latest
    needs: build
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
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - name: Apply schema
        run: |
          sudo apt-get update && sudo apt-get install -y postgresql-client
          PGPASSWORD=postgres psql -h localhost -U postgres -d postgres -f internal/adapters/postgres/schema.sql
      - name: Integration tests (Postgres + RabbitMQ; S3 skips — no S3_TEST_BUCKET in CI yet)
        env:
          TEST_DATABASE_URL: postgres://postgres:postgres@localhost:5432/postgres
        run: go test -tags=integration ./...
```

`cache: true` sur chaque `setup-go` met en cache le module Go (`~/go/pkg/mod`) entre les runs, à partir
d'une clé dérivée de `go.sum` — évite de retélécharger toutes les dépendances à chaque job, dans les 5
jobs de ce workflow. `golangci-lint-action` télécharge et lance `golangci-lint` avec sa config par
défaut (activable/configurable plus tard via un `.golangci.yml` si besoin de régler des règles précises).

Le test S3 (`internal/adapters/s3/audio_storage_test.go`) appelle `t.Skip` si `S3_TEST_BUCKET` n'est
pas défini — donc il se met de côté proprement en CI tant qu'on n'a pas ajouté d'identifiants AWS en
GitHub Secrets, sans faire échouer le workflow.

- [ ] **Step 2: Vérifier localement que les commandes de chaque job fonctionnent**

Run (avec Postgres/RabbitMQ déjà lancés localement) :
```bash
go vet ./... && go test ./... && go build ./... && TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/postgres" go test -tags=integration ./...
```
Expected : tout passe, avant même de pousser sur GitHub.

- [ ] **Step 3: Commit et pousser pour voir les 5 jobs tourner réellement**

```bash
git add .github/workflows/backend-ci.yml
git commit -m "Add GitHub Actions CI: parallel lint/test/security, then build, then integration"
git push origin backend
```

Vérifier l'onglet Actions du repo — les jobs `lint`/`test`/`security` doivent apparaître en parallèle,
puis `build`, puis `integration-test`.

---

### Task 10: Chart Helm + manifests Kubernetes

**Révisé le 2026-08-16** — voir `docs/superpowers/specs/2026-08-16-backend-mvp-completion-design.md`,
Sous-système 6, pour le raisonnement complet (Helm plutôt que YAML bruts, canary Istio et blue-green
comme deux stratégies alternatives documentées séparément — pas combinées, pas déployées simultanément —,
Karpenter documenté à titre d'illustration).

**Fait le 2026-08-16 (écriture + validation locale) :** Dockerfiles écrits, les deux images
(`rio-api:local`, `rio-worker:local`) buildent sans erreur. Chart Helm écrit, `helm lint` et `helm
template` passent sans erreur. Manifests `canary-istio/` et `blue-green/` écrits, syntaxe YAML validée.
`karpenter-nodepool-example.yaml` écrit à titre illustratif (ses CRDs n'existent que sur un cluster EKS
avec Karpenter installé, non validable hors cluster réel). Tout commité.

**Reste à faire — décision reprise en session (2026-08-16, soirée) :** initialement "écrit, pas
déployé" restait la ligne d'arrivée de cette tâche. L'autrice a explicitement choisi de monter un vrai
cluster EKS ce soir pour valider le déploiement de bout en bout (coût/risque d'infra live assumés en
connaissance de cause, cluster prévu pour être détruit après validation, pas laissé tourner sans
raison) — voir Tâche 11 pour ce déploiement réel.

**Files :**
- Create: `deploy/docker/Dockerfile.api`
- Create: `deploy/docker/Dockerfile.worker`
- Create: `deploy/helm/rio-backend/Chart.yaml`, `values.yaml`, `templates/*.yaml`
- Create: `deploy/k8s/canary-istio/*.yaml`
- Create: `deploy/k8s/blue-green/*.yaml`
- Create: `deploy/k8s/karpenter-nodepool-example.yaml`

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

- [ ] **Step 3: Chart Helm — squelette (`Chart.yaml`, `values.yaml`)**

```yaml
# deploy/helm/rio-backend/Chart.yaml
apiVersion: v2
name: rio-backend
description: Rio Audio Guide backend (API + worker)
type: application
version: 0.1.0
appVersion: "0.1.0"
```

```yaml
# deploy/helm/rio-backend/values.yaml
api:
  image:
    repository: rio-api
    tag: local
  replicas: 2
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70

worker:
  image:
    repository: rio-worker
    tag: local
  minReplicas: 0
  maxReplicas: 10
  rabbitmq:
    queueName: tts_jobs
    queueLength: "5" # jobs en attente par réplique avant de scaler

s3:
  bucket: rio-audio-guide

secrets:
  name: rio-backend-secrets # créé hors chart (kubectl create secret), jamais commité
```

- [ ] **Step 4: Templates Helm — API (Deployment, Service, HPA sur CPU)**

```yaml
# deploy/helm/rio-backend/templates/api-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-api
spec:
  replicas: {{ .Values.api.replicas }}
  selector:
    matchLabels: { app: {{ .Release.Name }}-api }
  template:
    metadata:
      labels: { app: {{ .Release.Name }}-api }
    spec:
      containers:
        - name: api
          image: "{{ .Values.api.image.repository }}:{{ .Values.api.image.tag }}"
          ports: [{ containerPort: 8080 }]
          env:
            - name: DATABASE_URL
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: database-url } }
            - name: RABBITMQ_URL
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: rabbitmq-url } }
```

```yaml
# deploy/helm/rio-backend/templates/api-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-api
spec:
  selector: { app: {{ .Release.Name }}-api }
  ports:
    - { port: 80, targetPort: 8080 }
```

```yaml
# deploy/helm/rio-backend/templates/api-hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ .Release.Name }}-api
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: {{ .Release.Name }}-api }
  minReplicas: {{ .Values.api.minReplicas }}
  maxReplicas: {{ .Values.api.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: {{ .Values.api.targetCPUUtilization }} }
```

- [ ] **Step 5: Templates Helm — worker (Deployment, ScaledObject KEDA sur profondeur de queue)**

```yaml
# deploy/helm/rio-backend/templates/worker-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-worker
spec:
  replicas: 1
  selector:
    matchLabels: { app: {{ .Release.Name }}-worker }
  template:
    metadata:
      labels: { app: {{ .Release.Name }}-worker }
    spec:
      containers:
        - name: worker
          image: "{{ .Values.worker.image.repository }}:{{ .Values.worker.image.tag }}"
          env:
            - name: DATABASE_URL
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: database-url } }
            - name: RABBITMQ_URL
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: rabbitmq-url } }
            - name: S3_BUCKET
              value: "{{ .Values.s3.bucket }}"
```

```yaml
# deploy/helm/rio-backend/templates/worker-scaledobject.yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: {{ .Release.Name }}-worker
spec:
  scaleTargetRef: { name: {{ .Release.Name }}-worker }
  minReplicaCount: {{ .Values.worker.minReplicas }}
  maxReplicaCount: {{ .Values.worker.maxReplicas }}
  triggers:
    - type: rabbitmq
      metadata:
        queueName: {{ .Values.worker.rabbitmq.queueName }}
        mode: QueueLength
        value: "{{ .Values.worker.rabbitmq.queueLength }}"
      authenticationRef:
        name: {{ .Release.Name }}-rabbitmq-auth
```

- [ ] **Step 6: Valider le chart sans déployer**

Run: `helm lint deploy/helm/rio-backend` puis `helm template rio deploy/helm/rio-backend`.
Expected : `helm lint` ne remonte aucune erreur ; `helm template` produit du YAML valide (Deployment,
Service, HPA, ScaledObject) sans contacter de cluster. Si `helm` n'est pas installé, `brew install
helm` (ou équivalent) avant cette étape.

- [ ] **Step 7: Canary Istio — Gateway, VirtualService, DestinationRule, deux Deployments**

Stratégie 1/2 : bascule progressive du trafic par pourcentage (nécessite un maillage Istio installé sur
le cluster). Le `Service` `rio-api` reste unique et sélectionne les deux versions ; c'est le
`DestinationRule` qui distingue les sous-ensembles `stable`/`canary` par label, et le `VirtualService`
qui répartit le trafic entre eux.

```yaml
# deploy/k8s/canary-istio/gateway.yaml
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: rio-api-gateway
spec:
  selector:
    istio: ingressgateway
  servers:
    - port: { number: 80, name: http, protocol: HTTP }
      hosts: ["*"]
```

```yaml
# deploy/k8s/canary-istio/destinationrule.yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: rio-api
spec:
  host: rio-api
  subsets:
    - name: stable
      labels: { version: stable }
    - name: canary
      labels: { version: canary }
```

```yaml
# deploy/k8s/canary-istio/virtualservice.yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: rio-api
spec:
  hosts: ["*"]
  gateways: [rio-api-gateway]
  http:
    - route:
        - destination: { host: rio-api, subset: stable }
          weight: 90
        - destination: { host: rio-api, subset: canary }
          weight: 10 # à monter progressivement (10 → 50 → 100) une fois le canary validé
```

```yaml
# deploy/k8s/canary-istio/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: rio-api
spec:
  selector: { app: rio-api } # sélectionne stable ET canary — Istio fait la distinction, pas ce Service
  ports:
    - { port: 80, targetPort: 8080 }
```

```yaml
# deploy/k8s/canary-istio/deployment-stable.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-api-stable
spec:
  replicas: 3
  selector:
    matchLabels: { app: rio-api, version: stable }
  template:
    metadata:
      labels: { app: rio-api, version: stable }
    spec:
      containers:
        - name: api
          image: rio-api:v1.0.0
          ports: [{ containerPort: 8080 }]
```

```yaml
# deploy/k8s/canary-istio/deployment-canary.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-api-canary
spec:
  replicas: 1
  selector:
    matchLabels: { app: rio-api, version: canary }
  template:
    metadata:
      labels: { app: rio-api, version: canary }
    spec:
      containers:
        - name: api
          image: rio-api:v1.1.0-rc1
          ports: [{ containerPort: 8080 }]
```

- [ ] **Step 8: Blue-green — deux Deployments, un Service qui bascule intégralement**

Stratégie 2/2, alternative à l'étape précédente (pas combinée avec Istio) : pas de répartition en
pourcentage, le `Service` pointe entièrement sur `blue` ou entièrement sur `green` — plus simple, pas
besoin de maillage de service.

```yaml
# deploy/k8s/blue-green/deployment-blue.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-api-blue
spec:
  replicas: 3
  selector:
    matchLabels: { app: rio-api, slot: blue }
  template:
    metadata:
      labels: { app: rio-api, slot: blue }
    spec:
      containers:
        - name: api
          image: rio-api:v1.0.0
          ports: [{ containerPort: 8080 }]
```

```yaml
# deploy/k8s/blue-green/deployment-green.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rio-api-green
spec:
  replicas: 3
  selector:
    matchLabels: { app: rio-api, slot: green }
  template:
    metadata:
      labels: { app: rio-api, slot: green }
    spec:
      containers:
        - name: api
          image: rio-api:v1.1.0
          ports: [{ containerPort: 8080 }]
```

```yaml
# deploy/k8s/blue-green/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: rio-api
spec:
  selector: { app: rio-api, slot: blue } # bascule : kubectl patch service rio-api -p '{"spec":{"selector":{"slot":"green"}}}'
  ports:
    - { port: 80, targetPort: 8080 }
```

- [ ] **Step 9: Karpenter — exemple de `NodePool` (documenté, pas déployé)**

Illustre le scaling de **nœuds** EC2 (différent de HPA/KEDA, qui scalent des **pods**) — pertinent
seulement une fois un vrai cluster EKS monté, pas avant.

```yaml
# deploy/k8s/karpenter-nodepool-example.yaml
# Exemple illustratif — nécessite Karpenter installé sur un vrai cluster EKS, pas déployé ici.
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: rio-backend-nodepool
spec:
  template:
    spec:
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["on-demand"]
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: rio-backend
  limits:
    cpu: "100"
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 30s
---
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: rio-backend
spec:
  amiFamily: AL2023
  subnetSelectorTerms:
    - tags: { karpenter.sh/discovery: rio-audio-guide }
  securityGroupSelectorTerms:
    - tags: { karpenter.sh/discovery: rio-audio-guide }
```

- [ ] **Step 10: Valider la syntaxe des manifests bruts sans déployer**

Run: `kubectl apply --dry-run=client -f deploy/k8s/canary-istio/ -f deploy/k8s/blue-green/` (nécessite
un contexte kubectl configuré, même vide/local type `kind` — valide juste la syntaxe YAML/schema, ne
contacte pas de vrai cluster de prod ; `deploy/k8s/karpenter-nodepool-example.yaml` est exclu de cette
validation puisque ses CRDs `karpenter.sh`/`karpenter.k8s.aws` ne sont installées que sur un vrai
cluster EKS avec Karpenter). Expected : pas d'erreur de syntaxe. Si aucun contexte kubectl n'est
disponible, `kubectl apply --dry-run=client --validate=false -f ... -o yaml` ou un linter YAML suffit
pour cette étape.

- [ ] **Step 11: Commit**

```bash
git add deploy/
git commit -m "Add Helm chart (API HPA, worker KEDA), canary-istio and blue-green manifests, Karpenter example — not deployed"
```

---

### Task 11: Déploiement réel sur un cluster Kubernetes (ajoutée le 2026-08-16, décision reprise en session)

Initialement hors scope de ce plan (voir la note "Ce que ce plan ne couvre pas" plus bas, écrite avant
cette tâche) : monter un vrai cluster EKS coûte de l'argent et du temps de provisioning, et le spec
associé recommandait explicitement d'éviter toute infra live sans supervision. L'autrice a choisi en
connaissance de cause de le faire quand même ce soir, pour une pratique concrète Docker/K8s/Helm.

**Pivot EKS → `kind`, décidé en session :** la première tentative (`eksctl create cluster --name
rio-audio-guide --region us-east-1`) a échoué immédiatement — le rôle IAM du compte utilisé (un AWS
Academy/Vocareum Learner Lab, `assumed-role/voclabs/...`) n'a pas la permission `iam:CreateRole`,
nécessaire à la création du rôle de service EKS. Restriction structurelle du compte lab (anti-privilège-
escalation côté AWS Academy), pas un problème de configuration côté `eksctl` — confirmé par la stack
CloudFormation `eksctl-rio-audio-guide-cluster`, `ROLLBACK_COMPLETE` avec `CREATE_FAILED` sur
`AWS::IAM::Role/ServiceRole` (`UnauthorizedTaggingOperation`), aucune ressource orpheline restée
derrière (`eks list-clusters` vide après coup). Pas de solution dans ce compte lab ; option restante
aurait été un vrai compte AWS personnel (carte réelle, coûts EKS + NAT gateway réels même pour
quelques heures) — écartée pour rester dans le temps disponible ce soir.

**Remplacé par `kind`** (Kubernetes-in-Docker) : cluster Kubernetes réel et conforme, tournant en local
via Docker, sans les restrictions IAM du compte lab, sans coût, provisionné en ~15 secondes contre
~15-20 minutes pour EKS. Toutes les commandes ci-dessous ont été exécutées et vérifiées en clair contre
ce cluster `kind`, pas seulement écrites — voir le résultat de chaque étape.

**Deux bugs réels trouvés et corrigés pendant l'exécution, pas seulement pendant l'écriture :**
1. Le chart Helm Bitnami `rabbitmq` (Step 3 initial) a échoué en `ImagePullBackOff` — Bitnami restreint
   depuis fin août 2025 l'accès gratuit à la plupart de ses images versionnées (même mur de licence déjà
   rencontré avec LocalStack, cf. `2026-08-16-backend-mvp-completion-design.md` sous-système 2).
   Remplacé par un Deployment+Service RabbitMQ minimal utilisant `rabbitmq:3-management` directement
   (même image que celle déjà utilisée dans les `services:` de `backend-ci.yml`).
2. Le template `worker-scaledobject.yaml` (Tâche 10) référence
   `authenticationRef: { name: {{ .Release.Name }}-rabbitmq-auth }` mais aucun objet
   `TriggerAuthentication` de ce nom n'était jamais créé — bug de conception passé inaperçu à l'écriture
   du chart car jamais testé contre un cluster réel avant ce soir. Corrigé en ajoutant la
   `TriggerAuthentication` manquante. Deuxième bug découvert au passage : son
   `secretTargetRef` pointait vers `rabbitmq-url` avec un nom de service DNS court
   (`demo-rabbitmq`), qui ne se résout que dans le namespace `default` — le `keda-operator` tourne dans
   le namespace `keda` et ne pouvait pas le résoudre (`no such host`). Corrigé en utilisant le nom DNS
   pleinement qualifié (`demo-rabbitmq.default.svc.cluster.local`).

**Prérequis côté autrice, pas exécutables par Claude :** identifiants AWS valides dans l'environnement
(`aws sts get-caller-identity` doit réussir), `eksctl` installé (`brew install eksctl`).

**Files:** aucun nouveau fichier — commandes d'infrastructure uniquement.

- [x] **Step 1: Créer les deux dépôts ECR**

Run:
```bash
aws ecr create-repository --repository-name rio-api --region us-east-1
aws ecr create-repository --repository-name rio-worker --region us-east-1
```
**Fait.** Deux dépôts créés (`424495842167.dkr.ecr.us-east-1.amazonaws.com/rio-api` et `/rio-worker`),
`docker login` vers ECR réussi. Restent vides — le push réel n'a pas eu lieu, la démo finale utilise
`kind` (Step 2 révisé) et charge les images directement, sans passer par un registre. Ces dépôts
serviront le jour où `docker-build.yml` (Tâche 9) tourne pour de vrai.

- [x] **Step 2 (révisé) : Créer le cluster — `kind` à la place d'EKS**

Tentative initiale `eksctl create cluster --name rio-audio-guide --region us-east-1 --nodes 2
--node-type t3.medium --managed` : **échec immédiat**, `iam:CreateRole` refusé au rôle `voclabs` du
compte Learner Lab — restriction structurelle du compte, pas corrigible. Stack CloudFormation
`ROLLBACK_COMPLETE`, aucune ressource restée derrière (vérifié via `aws eks list-clusters` → vide).

Run (remplacement) :
```bash
brew install kind
kind create cluster --name rio-audio-guide
kubectl get nodes   # attendre Ready, ~30s
```
**Fait.** Cluster prêt en ~15-20 secondes contre les ~15-20 minutes annoncées pour EKS, sans les
restrictions IAM du lab, sans coût.

- [x] **Step 3 (révisé) : Installer Postgres et RabbitMQ dans le cluster**

Run :
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
helm install demo-postgres bitnami/postgresql --set auth.postgresPassword=postgres --set auth.database=postgres
```
Postgres : fonctionne directement (image `bitnami/postgresql:latest`, tag rolling, pas concerné par le
mur de licence Bitnami).

RabbitMQ via le chart Bitnami a échoué (`ImagePullBackOff` sur
`docker.io/bitnami/rabbitmq:4.1.3-debian-12-r1` — mur de licence Bitnami rencontré, même famille de
problème que LocalStack plus tôt dans le projet). Remplacé par un Deployment+Service minimal :
```bash
helm uninstall demo-rabbitmq
kubectl apply -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata: { name: demo-rabbitmq }
spec:
  replicas: 1
  selector: { matchLabels: { app: demo-rabbitmq } }
  template:
    metadata: { labels: { app: demo-rabbitmq } }
    spec:
      containers:
        - name: rabbitmq
          image: rabbitmq:3-management
          ports: [{ containerPort: 5672, name: amqp }, { containerPort: 15672, name: management }]
---
apiVersion: v1
kind: Service
metadata: { name: demo-rabbitmq }
spec:
  selector: { app: demo-rabbitmq }
  ports:
    - { name: amqp, port: 5672, targetPort: 5672 }
    - { name: management, port: 15672, targetPort: 15672 }
EOF
```
**Fait.** `kubectl get pods` confirme `demo-postgres-postgresql-0` et `demo-rabbitmq-<hash>` en
`1/1 Running`.

- [x] **Step 4: Charger le schéma Postgres**

Run (adapté — `kubectl cp` + `kubectl exec` plutôt qu'un pod éphémère, plus fiable sur `kind`) :
```bash
kubectl cp internal/adapters/postgres/schema.sql demo-postgres-postgresql-0:/tmp/schema.sql
kubectl exec demo-postgres-postgresql-0 -- env PGPASSWORD=postgres psql -U postgres -d postgres -f /tmp/schema.sql
```
**Fait.** `CREATE EXTENSION`, `CREATE TABLE` × 3, `CREATE INDEX` confirmés.

- [x] **Step 5: Créer le Secret Kubernetes `rio-backend-secrets`**

Run :
```bash
kubectl create secret generic rio-backend-secrets \
  --from-literal=database-url="postgresql://postgres:postgres@demo-postgres-postgresql:5432/postgres" \
  --from-literal=rabbitmq-url="amqp://guest:guest@demo-rabbitmq.default.svc.cluster.local:5672/"
```
**Fait — avec un bug trouvé et corrigé.** Premier essai avec le nom court `demo-rabbitmq` (sans
suffixe) : le worker (namespace `default`) s'en accommode, mais le `keda-operator` (namespace `keda`,
Step 7) ne pouvait pas le résoudre (`no such host`) — un nom DNS court ne se résout que dans le
namespace du pod appelant. Corrigé avec le nom pleinement qualifié
`demo-rabbitmq.default.svc.cluster.local`. Jamais commité — créé directement sur le cluster.

- [x] **Step 6 (révisé) : Charger les images locales dans `kind` (pas de push registre nécessaire)**

Run :
```bash
kind load docker-image rio-api:local --name rio-audio-guide
kind load docker-image rio-worker:local --name rio-audio-guide
```
**Fait.** `docker-build.yml`/ECR restent la voie prévue pour un vrai déploiement EKS plus tard — sur
`kind`, les images buildées en local (Tâche 10, Step 2) sont chargées directement sur le nœud, sans
registre intermédiaire.

- [x] **Step 7: Déployer avec Helm — bug de conception trouvé et corrigé**

Run :
```bash
helm upgrade --install rio deploy/helm/rio-backend \
  --set api.image.repository=rio-api --set api.image.tag=local \
  --set worker.image.repository=rio-worker --set worker.image.tag=local \
  --set api.replicas=1 --set api.minReplicas=1
```
Premier essai : échec, `no matches for kind "ScaledObject" in version "keda.sh/v1alpha1"` — KEDA n'est
pas un composant K8s natif, ses CRDs doivent être installées séparément (vrai sur `kind` comme sur EKS,
pas spécifique à `kind`) :
```bash
helm repo add kedacore https://kedacore.github.io/charts
helm install keda kedacore/keda --namespace keda --create-namespace
kubectl wait --for=condition=ready pod -l app=keda-operator -n keda --timeout=90s
```
Deuxième essai, l'install Helm passe, mais `kubectl get scaledobject` reste `READY=False` — bug de
conception du chart (Tâche 10) trouvé en le testant pour de vrai : `worker-scaledobject.yaml` référence
`authenticationRef: { name: rio-rabbitmq-auth }` mais ce `TriggerAuthentication` n'existait nulle part,
ni dans le chart ni créé à la main. Corrigé en l'ajoutant :
```bash
kubectl apply -f - <<'EOF'
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata: { name: rio-rabbitmq-auth }
spec:
  secretTargetRef:
    - { parameter: host, name: rio-backend-secrets, key: rabbitmq-url }
EOF
```
**Fait, après les deux corrections ci-dessus.** `kubectl get scaledobject` confirme `READY=True`.

- [x] **Step 8: Vérifier que ça tourne réellement**

Run :
```bash
kubectl get pods
kubectl port-forward svc/rio-api 18080:80 &
curl http://localhost:18080/places
```
**Fait.** `rio-api` et `rio-worker` en `1/1 Running`, `curl` renvoie `[]` avec `HTTP 200` — liste vide
attendue (Tâche 8, l'import de données, pas encore faite), mais preuve que l'API tourne réellement,
connectée à un vrai Postgres, servant une vraie requête HTTP sur un vrai cluster Kubernetes. Bonus
observé sans rien avoir eu à déclencher : KEDA a lui-même redescendu `rio-worker` de 1 à 0 réplique
(`KEDAScaleTargetDeactivated`) car la queue `tts_jobs` est vide — le scale-to-zero fonctionne
réellement, pas juste configuré sur le papier.

- [ ] **Step 9: Détruire le cluster une fois la soirée de démo terminée**

Run :
```bash
kind delete cluster --name rio-audio-guide
```
Expected : cluster supprimé (`kind get clusters` ne le liste plus). Moins critique qu'avec EKS — `kind`
ne facture rien tant qu'il tourne — mais reste une bonne pratique de ne pas laisser tourner un cluster
inutilisé, et libère les ressources Docker locales (RAM/CPU) pour le reste de la soirée.

---

## Ce que ce plan ne couvre pas

- Vraie intégration TTS (ElevenLabs) — le stub reste un stub à ce stade ; si l'autrice fournit une clé
  API ce soir, ce sera une tâche à concevoir et documenter séparément (nouveau port `TTSGenerator`,
  nouvel adaptateur), pas ajoutée ici tant qu'elle n'est pas commencée pour éviter une doc spéculative.
- Authentification sur l'API HTTP.
- Secrets réels (`rio-backend-secrets` référencé mais jamais créé) — à faire au moment du déploiement réel.
