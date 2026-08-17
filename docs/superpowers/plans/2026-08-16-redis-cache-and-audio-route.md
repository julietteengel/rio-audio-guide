# Route Audio + Cache Redis + Classification Erreurs S3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fermer le scénario "je suis près d'un lieu, je peux écouter son histoire" via une vraie route
HTTP (URL présignée S3, pas de lien `s3://` brut), ajouter un cache Redis devant les lectures chaudes
(déclencheur mesuré), et corriger la boucle de retry infinie sur les erreurs S3 permanentes.

**Architecture:** Ordonné en tracer bullets — Tâches 1 à 4 construisent d'abord un chemin vertical mince
mais complet et fonctionnel (lookup → présignation → route HTTP, testable de bout en bout, sans cache) ;
Tâches 5 à 7 ajoutent le cache Redis par-dessus ce chemin déjà prouvé ; Tâche 8 est un correctif de
robustesse indépendant.

**Tech Stack:** Go 1.25, `github.com/aws/aws-sdk-go-v2/service/s3` (déjà présent — la présignation ne
nécessite aucune nouvelle dépendance), `github.com/redis/go-redis/v9` (nouvelle dépendance, Tâche 5).

**Spec:** `docs/superpowers/specs/2026-08-16-redis-cache-and-audio-route-design.md`

## Global Constraints

- Go 1.25.0, module `rioaudioguide/backend`.
- TTL Redis fixe 5 minutes sur les deux routes cachées, pas d'invalidation active.
- Toute erreur Redis est un cache miss (fail-open) — jamais une erreur de requête.
- URLs présignées S3 : expiration 15 minutes.
- Réutiliser `ports.PermanentError` (déjà créé pour ElevenLabs) pour la classification des erreurs S3 —
  pas de nouveau mécanisme.
- `internal/ports/` : les 4 méthodes déjà ajoutées (`FindByPlaceIDAndLanguage`, `FindByScriptID`,
  `PresignURL`, `Cache`) l'ont été en pair programming avec la fondatrice — déjà committées, ne pas les
  re-modifier sans raison dans ce plan.

---

### Task 1: Postgres — `ScriptRepository.FindByPlaceIDAndLanguage`

**Files:**
- Modify: `internal/adapters/postgres/script_repository.go`
- Modify: `internal/adapters/postgres/script_repository_test.go`

**Interfaces:**
- Consumes: `ports.ScriptRepository.FindByPlaceIDAndLanguage(ctx, placeID, language string)
  (*domain.Script, error)` — déjà déclarée dans `internal/ports/script_repository.go`.
- Produces: implémentation Postgres de cette méthode.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/postgres/script_repository_test.go`:

```go
func TestScriptRepository_FindByPlaceIDAndLanguage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Theatro Municipal")
	coords, _ := domain.NewCoordinates(-22.9105, -43.1765)
	place := domain.NewPlace(placeName, "monument", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	text, _ := domain.NewScriptText("Voici le Theatro Municipal...")
	script := domain.NewScript(place.ID(), domain.LanguagePT, text, "source text")
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save script fixture: %v", err)
	}

	found, err := scriptRepo.FindByPlaceIDAndLanguage(ctx, place.ID(), "pt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != script.ID() {
		t.Fatalf("got ID %q, want %q", found.ID(), script.ID())
	}

	if _, err := scriptRepo.FindByPlaceIDAndLanguage(ctx, place.ID(), "es"); err == nil {
		t.Fatal("expected an error for a language with no script, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestScriptRepository_FindByPlaceIDAndLanguage -v`
Expected: FAIL — compilation error, `FindByPlaceIDAndLanguage` undefined on `*ScriptRepository`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/adapters/postgres/script_repository.go`, right after `FindByID`:

```go
func (r *ScriptRepository) FindByPlaceIDAndLanguage(ctx context.Context, placeID, language string) (*domain.Script, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, place_id, language, text, COALESCE(source_text, ''), status,
		       COALESCE(reviewer, ''), reviewed_at, published_at
		FROM scripts WHERE place_id = $1 AND language = $2
	`, placeID, language)

	var scriptID, placeIDCol, languageRaw, textRaw, sourceText, status, reviewer string
	var reviewedAt, publishedAt *time.Time
	if err := row.Scan(&scriptID, &placeIDCol, &languageRaw, &textRaw, &sourceText, &status,
		&reviewer, &reviewedAt, &publishedAt); err != nil {
		return nil, err
	}

	language2, err := domain.NewLanguage(languageRaw)
	if err != nil {
		return nil, err
	}
	text, err := domain.NewScriptText(textRaw)
	if err != nil {
		return nil, err
	}

	var reviewedAtVal, publishedAtVal time.Time
	if reviewedAt != nil {
		reviewedAtVal = *reviewedAt
	}
	if publishedAt != nil {
		publishedAtVal = *publishedAt
	}

	return domain.ReconstructScript(scriptID, placeIDCol, language2, text, sourceText, domain.ScriptStatus(status),
		reviewer, reviewedAtVal, publishedAtVal), nil
}
```

(Mirrors `FindByID` exactly — same scan shape, only the `WHERE` clause and its two params differ. The
`UNIQUE (place_id, language)` constraint on `scripts` guarantees at most one row, so no `ORDER BY`/`LIMIT`
ambiguity to resolve here, unlike `PlaceRepository.FindByName`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: PASS (all tests in the package)

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/postgres/script_repository.go internal/adapters/postgres/script_repository_test.go
git commit -m "Implement ScriptRepository.FindByPlaceIDAndLanguage"
```

---

### Task 2: Postgres — `AudioFileRepository.FindByScriptID`

**Files:**
- Modify: `internal/adapters/postgres/audiofile_repository.go`
- Modify: `internal/adapters/postgres/audiofile_repository_test.go`

**Interfaces:**
- Consumes: `ports.AudioFileRepository.FindByScriptID(ctx, scriptID string) (*domain.AudioFile, error)` —
  déjà déclarée.
- Produces: implémentation Postgres.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/postgres/audiofile_repository_test.go`:

```go
func TestAudioFileRepository_FindByScriptID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeName, _ := domain.NewPlaceName("Real Gabinete")
	coords, _ := domain.NewCoordinates(-22.9, -43.18)
	place := domain.NewPlace(placeName, "museum", coords, "", "overture", "correct")
	placeRepo := NewPlaceRepository(pool)
	_ = placeRepo.Save(ctx, place)

	scriptText, _ := domain.NewScriptText("Text")
	script := domain.NewScript(place.ID(), domain.LanguageEN, scriptText, "source")
	scriptRepo := NewScriptRepository(pool)
	_ = scriptRepo.Save(ctx, script)

	audioRepo := NewAudioFileRepository(pool)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	if err := audioRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := audioRepo.FindByScriptID(ctx, script.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.ID() != audioFile.ID() {
		t.Fatalf("got ID %q, want %q", found.ID(), audioFile.ID())
	}

	if _, err := audioRepo.FindByScriptID(ctx, "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown script ID, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestAudioFileRepository_FindByScriptID -v`
Expected: FAIL — compilation error, `FindByScriptID` undefined.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/adapters/postgres/audiofile_repository.go`, right after `FindByID`:

```go
// FindByScriptID renvoie l'AudioFile le plus récent pour ce script. Pas de
// contrainte UNIQUE sur script_id en base (contrairement à scripts.place_id+
// language) — le flux normal (ReviewAndRequestAudio, appelé une fois par
// script grâce à la garde MarkReviewed) ne crée jamais qu'une ligne, mais
// ORDER BY + LIMIT rend le choix déterministe si ça arrivait quand même.
func (r *AudioFileRepository) FindByScriptID(ctx context.Context, scriptID string) (*domain.AudioFile, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, script_id, voice_id, status, COALESCE(storage_url, ''),
		       COALESCE(timestamps_url, ''), COALESCE(duration_ms, 0), COALESCE(failure_reason, '')
		FROM audio_files WHERE script_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, scriptID)

	var audioFileID, scriptIDCol, voiceID, status, storageURL, timestampsURL, failureReason string
	var durationMs int64
	if err := row.Scan(&audioFileID, &scriptIDCol, &voiceID, &status, &storageURL, &timestampsURL,
		&durationMs, &failureReason); err != nil {
		return nil, err
	}

	var audio domain.GeneratedAudio
	if storageURL != "" {
		var err error
		audio, err = domain.NewGeneratedAudio(storageURL, timestampsURL, time.Duration(durationMs)*time.Millisecond)
		if err != nil {
			return nil, err
		}
	}

	return domain.ReconstructAudioFile(audioFileID, scriptIDCol, voiceID, domain.AudioFileStatus(status), audio, failureReason), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/postgres/audiofile_repository.go internal/adapters/postgres/audiofile_repository_test.go
git commit -m "Implement AudioFileRepository.FindByScriptID"
```

---

### Task 3: S3 — `AudioStorage.PresignURL`

**Files:**
- Modify: `internal/adapters/s3/audio_storage.go`
- Modify: `internal/adapters/s3/audio_storage_test.go`

**Interfaces:**
- Consumes: `ports.AudioStorage.PresignURL(ctx, key string, expiry time.Duration) (string, error)` — déjà
  déclarée.
- Produces: implémentation via `s3.PresignClient` (présignation, pas d'appel réseau — calcul de signature
  entièrement côté client, testable sans vrai bucket).

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/s3/audio_storage_test.go — append
func TestAudioStorage_PresignURL(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
	}
	client := s3.NewFromConfig(cfg)
	storage := NewAudioStorage(client, "rio-audio-guide")

	url, err := storage.PresignURL(context.Background(), "abc123.mp3", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(url, "rio-audio-guide") || !strings.Contains(url, "abc123.mp3") {
		t.Fatalf("got url %q, want it to reference the bucket and key", url)
	}
	if !strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("got url %q, want a signed URL (X-Amz-Signature query param)", url)
	}
}
```

Add `"strings"`, `"time"`, `"github.com/aws/aws-sdk-go-v2/aws"`, `"github.com/aws/aws-sdk-go-v2/credentials"`
to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/s3/... -run TestAudioStorage_PresignURL -v`
Expected: FAIL — compilation error, `PresignURL` undefined on `*AudioStorage`.

- [ ] **Step 3: Write minimal implementation**

Replace `internal/adapters/s3/audio_storage.go`'s struct/constructor and add the new method:

```go
package s3

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AudioStorage struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

func NewAudioStorage(client *s3.Client, bucket string) *AudioStorage {
	return &AudioStorage{client: client, presignClient: s3.NewPresignClient(client), bucket: bucket}
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

// PresignURL rend le fichier stocké chargeable directement par un client HTTP
// (navigateur, app mobile) — storage_url (s3://bucket/clé) n'est pas une URL
// HTTP utilisable telle quelle. Ne fait aucun appel réseau : la signature est
// calculée localement à partir des credentials du client.
func (a *AudioStorage) PresignURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	req, err := a.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/s3/... -v`
Expected: PASS. Note: `TestAudioStorage_Upload` (pre-existing) still requires `S3_TEST_BUCKET` and skips
cleanly without it — unaffected by this change.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/s3/audio_storage.go internal/adapters/s3/audio_storage_test.go
git commit -m "Implement AudioStorage.PresignURL"
```

---

### Task 4: HTTP route `GET /places/:id/audio` (fin du premier tracer bullet)

**Files:**
- Create: `internal/adapters/http/audio_handler.go`
- Create: `internal/adapters/http/audio_handler_test.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/server_test.go`
- Modify: `cmd/api/main.go`

**Interfaces:**
- Consumes: Task 1's `FindByPlaceIDAndLanguage`, Task 2's `FindByScriptID`, Task 3's `PresignURL`.
- Produces: `GET /places/:id/audio?language=xx` — 404 (script absent), 404 (audio jamais demandé), 202
  `{"status": "..."}` (pas prêt), 200 `{"url": "..."}` (prêt, URL présignée).

Ce Task termine le premier tracer bullet : à la fin, tout le chemin (lookup → présignation → HTTP) est
prouvé de bout en bout, sans cache — le cache est une couche ajoutée par-dessus dans les Tâches 5-7.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/http/audio_handler_test.go`:

```go
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type fakeAudioStorage struct{}

func (fakeAudioStorage) Upload(_ context.Context, _ string, _ []byte, _ string) (string, error) {
	return "", errNotImplementedInFake
}
func (fakeAudioStorage) PresignURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://presigned.example.com/" + key + "?X-Amz-Signature=fake", nil
}

func TestGetPlaceAudio_Ready(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(placeName, "monument", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")

	audio, _ := domain.NewGeneratedAudio("s3://rio-audio-guide/abc123.mp3", "", 30*time.Second)
	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady(audio)

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo,
		&fakePublisher{}, fakeAudioStorage{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "presigned.example.com/abc123.mp3") {
		t.Fatalf("expected a presigned URL in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_NotReadyYet(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Cais do Valongo")
	coords, _ := domain.NewCoordinates(-22.8966, -43.1871)
	place := domain.NewPlace(placeName, "landmark", coords, "", "wikidata", "rich")

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript(place.ID(), domain.LanguageFR, text, "source")

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFile.MarkGenerating()

	scriptRepo := &fakeScriptRepo{scripts: map[string]*domain.Script{script.ID(): script}}
	audioFileRepo := &fakeAudioFileRepo{files: map[string]*domain.AudioFile{audioFile.ID(): audioFile}}
	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}}, scriptRepo, audioFileRepo,
		&fakePublisher{}, fakeAudioStorage{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "generating") {
		t.Fatalf("expected status \"generating\" in the response, got %s", rec.Body.String())
	}
}

func TestGetPlaceAudio_NoScriptForLanguage(t *testing.T) {
	placeName, _ := domain.NewPlaceName("Quinta da Boa Vista")
	coords, _ := domain.NewCoordinates(-22.9058, -43.2244)
	place := domain.NewPlace(placeName, "park", coords, "", "overture", "correct")

	server := NewServer(&fakePlaceRepo{places: []*domain.Place{place}},
		&fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{})

	req := httptest.NewRequest(http.MethodGet, "/places/"+place.ID()+"/audio?language=fr", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/... -v`
Expected: FAIL — compilation errors: `NewServer` called with 5 args but currently takes 4;
`fakeScriptRepo`/`fakeAudioFileRepo` don't implement the widened interfaces; `errNotImplementedInFake`
undefined; `getPlaceAudio` doesn't exist.

- [ ] **Step 3: Write minimal implementation**

First, update `internal/adapters/http/server.go` to add the `storage` field and thread it through:

```go
package http

import (
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/ports"
)

// Server regroupe l'instance Echo et les cinq ports dont l'API a besoin (les adaptateurs ne doivent jamais se connaître entre eux, seulement connaître les ports.)
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		publisher:     publisher,
		storage:       storage,
	}
	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.POST("/scripts/:id/review", s.reviewScript)
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
```

Create `internal/adapters/http/audio_handler.go`:

```go
package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// presignExpiry : durée de vie de l'URL présignée renvoyée au client — assez
// long pour un téléchargement immédiat, pas fait pour être un lien partageable
// durablement.
const presignExpiry = 15 * time.Minute

type audioResponse struct {
	URL string `json:"url"`
}

type audioNotReadyResponse struct {
	Status string `json:"status"`
}

func (s *Server) getPlaceAudio(c echo.Context) error {
	placeID := c.Param("id")
	language := c.QueryParam("language")
	if language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "language query param is required"})
	}

	script, err := s.scriptRepo.FindByPlaceIDAndLanguage(c.Request().Context(), placeID, language)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no script for this place/language"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	audioFile, err := s.audioFileRepo.FindByScriptID(c.Request().Context(), script.ID())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no audio ever requested for this script"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	if audioFile.Status() != "ready" {
		return c.JSON(http.StatusAccepted, audioNotReadyResponse{Status: string(audioFile.Status())})
	}

	key, err := parseS3Key(audioFile.Audio().StorageURL())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	url, err := s.storage.PresignURL(c.Request().Context(), key, presignExpiry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, audioResponse{URL: url})
}

// parseS3Key extrait la clé d'objet d'un storage_url au format s3://bucket/clé
// — c'est ce que domain.AudioFile.Audio().StorageURL() contient toujours,
// c'est le seul format que le worker écrit (internal/adapters/s3.Upload).
//
// getPlaceAudio distingue explicitement "vraiment absent" (pgx.ErrNoRows,
// 404) de toute autre erreur (panne DB transitoire, 500) — traiter toute
// erreur comme un 404 masquerait une vraie panne derrière une réponse "ça
// n'existe pas", la même classe de bug que l'erreur avalée trouvée dans
// cmd/import pendant le chantier ElevenLabs.
func parseS3Key(storageURL string) (string, error) {
	rest, ok := strings.CutPrefix(storageURL, "s3://")
	if !ok {
		return "", errors.New("storage URL is not an s3:// URL")
	}
	_, key, ok := strings.Cut(rest, "/")
	if !ok || key == "" {
		return "", errors.New("storage URL has no object key")
	}
	return key, nil
}
```

Note: `audioFile.Status() != "ready"` compares against the string form since `domain.AudioFileStatus` is
a `string` type — using the raw literal here (rather than importing `domain` into the http package just
for this constant) matches this file's existing pattern of not leaking domain internals into the
adapter's JSON layer; if this reads awkwardly, `domain.AudioFileStatusReady` is equally valid, just adds
an import.

Then update the two `fake*Repo` types in `server_test.go` (both existing tests must keep compiling) and
`cmd/api/main.go`:

In `internal/adapters/http/server_test.go`, add `"github.com/jackc/pgx/v5"` to the imports, and add to
`fakeScriptRepo`:
```go
func (f *fakeScriptRepo) FindByPlaceIDAndLanguage(_ context.Context, placeID, language string) (*domain.Script, error) {
	for _, s := range f.scripts {
		if s.PlaceID() == placeID && s.Language().String() == language {
			return s, nil
		}
	}
	return nil, pgx.ErrNoRows
}
```
and to `fakeAudioFileRepo`:
```go
func (f *fakeAudioFileRepo) FindByScriptID(_ context.Context, scriptID string) (*domain.AudioFile, error) {
	for _, a := range f.files {
		if a.ScriptID() == scriptID {
			return a, nil
		}
	}
	return nil, pgx.ErrNoRows
}
```
Unlike the other fake methods on these types (which return a plain `errNotImplementedInFake` because
nothing in the existing test suite exercises their not-found path), these two must return the *real*
sentinel (`pgx.ErrNoRows`) on a genuine miss — `getPlaceAudio` (above) specifically distinguishes
`pgx.ErrNoRows` (404) from any other error (500), so a fake returning the wrong error type here would
make `TestGetPlaceAudio_NoScriptForLanguage` assert the wrong status code.

Replace every existing `errors.New("not implemented in fake")` / `errors.New("not found")` literal used
across `fakePlaceRepo`/`fakeScriptRepo`/`fakeAudioFileRepo` in this file with a single shared
`var errNotImplementedInFake = errors.New("not implemented in fake")` declared once near the top of the
file, and update the two existing `TestListPlaces`/`TestReviewScript` calls to `NewServer(...)` to pass a
5th argument, `fakeAudioStorage{}` (already defined in `audio_handler_test.go`, same package).

In `cmd/api/main.go`, add AWS/S3 wiring (mirrors `cmd/worker/main.go`'s existing pattern) and thread
`storage` into `NewServer`:

```go
	// add to imports:
	// "github.com/aws/aws-sdk-go-v2/config"
	// awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	// "rioaudioguide/backend/internal/adapters/s3"

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audioguide-bucket"))

	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, publisher, storage)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS (all tests, including the 2 pre-existing ones with the updated `NewServer` call)

Run: `go build ./...`
Expected: clean (confirms `cmd/api/main.go` compiles).

- [ ] **Step 5: Manual end-to-end verification (this is the tracer bullet)**

Against the `kind` cluster (or local Postgres/S3), with a `published` script that has a `ready` AudioFile
(one already exists from tonight's manual test — script `238b7421-b80d-46c0-860b-64d59b9f789e`, place
`a1eda049-e7b3-4183-a371-fff0c52738c1`... actually use the `place_id` for that script, not the script ID,
since the route takes a place ID):

```bash
curl "localhost:8081/places/<place-id>/audio?language=fr"
```

Expected: `200 {"url": "https://rio-audio-guide.s3.amazonaws.com/...&X-Amz-Signature=..."}` — download
that URL directly (`curl -o test.mp3 "<url>"`) and confirm it's a playable MP3, exactly like the manual
`aws s3 cp` test from earlier tonight, but through the real HTTP route this time.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/audio_handler.go internal/adapters/http/audio_handler_test.go \
        internal/adapters/http/server.go internal/adapters/http/server_test.go cmd/api/main.go
git commit -m "Add GET /places/:id/audio route with presigned S3 URLs"
```

---

### Task 5: Redis adapter

**Files:**
- Modify: `go.mod`, `go.sum` (new dependency)
- Create: `internal/adapters/redis/cache.go`
- Create: `internal/adapters/redis/cache_test.go`

**Interfaces:**
- Consumes: `ports.Cache` (already declared, Get/Set with TTL).
- Produces: `redis.NewCache(client *goredis.Client) *Cache` implementing `ports.Cache`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/redis/go-redis/v9`

- [ ] **Step 2: Write the failing test**

```go
// internal/adapters/redis/cache_test.go
//go:build integration

package redis

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) *goredis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCache_SetAndGet(t *testing.T) {
	client := testClient(t)
	cache := NewCache(client)
	ctx := context.Background()

	if err := cache.Set(ctx, "test:key", "test-value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}

	value, found, err := cache.Get(ctx, "test:key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found || value != "test-value" {
		t.Fatalf("got (%q, %v), want (\"test-value\", true)", value, found)
	}
}

func TestCache_GetMissingKey(t *testing.T) {
	client := testClient(t)
	cache := NewCache(client)

	_, found, err := cache.Get(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for a missing key")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -tags=integration ./internal/adapters/redis/... -v`
Expected: FAIL — compilation error, `NewCache` undefined.

- [ ] **Step 4: Write minimal implementation**

```go
// internal/adapters/redis/cache.go
package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Cache struct {
	client *goredis.Client
}

func NewCache(client *goredis.Client) *Cache {
	return &Cache{client: client}
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `docker run --rm -d --name rio-redis -p 6379:6379 redis:7-alpine` (if not already running), then:
Run: `go test -tags=integration ./internal/adapters/redis/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/redis/cache.go internal/adapters/redis/cache_test.go
git commit -m "Add Redis cache adapter"
```

---

### Task 6: Cache-aside sur `GET /places` et `GET /places/:id/audio`

**Files:**
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/places_handler.go`
- Modify: `internal/adapters/http/audio_handler.go`
- Modify: `internal/adapters/http/server_test.go`
- Modify: `cmd/api/main.go`

**Interfaces:**
- Consumes: `ports.Cache` (Task 5's adapter, wired via `main.go`).
- Produces: both routes check the cache first, fall through to the existing logic on miss, populate the
  cache on the way out. Any `Cache` error is a miss (fail-open) — never surfaces to the client.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/http/server_test.go` (needs a fake `Cache` — add it near the other fakes):

```go
type fakeCache struct {
	data map[string]string
	sets int
}

func newFakeCache() *fakeCache { return &fakeCache{data: map[string]string{}} }

func (f *fakeCache) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.data[key] = value
	f.sets++
	return nil
}

type erroringCache struct{}

func (erroringCache) Get(_ context.Context, _ string) (string, bool, error) {
	return "", false, errors.New("redis unavailable")
}
func (erroringCache) Set(_ context.Context, _, _ string, _ time.Duration) error {
	return errors.New("redis unavailable")
}

func TestListPlaces_CachesOnSecondCall(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	cache := newFakeCache()
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, cache)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/places", nil)
		rec := httptest.NewRecorder()
		server.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: got status %d, want 200", i, rec.Code)
		}
	}

	if cache.sets != 1 {
		t.Fatalf("got %d cache writes, want exactly 1 (second call should have hit the cache)", cache.sets)
	}
}

func TestListPlaces_FailsOpenWhenCacheErrors(t *testing.T) {
	name, _ := domain.NewPlaceName("Cristo Redentor")
	coords, _ := domain.NewCoordinates(-22.9519, -43.2105)
	place := domain.NewPlace(name, "monument", coords, "", "wikidata", "rich")

	placeRepo := &fakePlaceRepo{places: []*domain.Place{place}}
	server := NewServer(placeRepo, &fakeScriptRepo{scripts: map[string]*domain.Script{}},
		&fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}, &fakePublisher{}, fakeAudioStorage{}, erroringCache{})

	req := httptest.NewRequest(http.MethodGet, "/places", nil)
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 — a cache error must never fail the request", rec.Code)
	}
}
```

Update the 2 pre-existing `NewServer(...)` calls in this file (`TestListPlaces`, `TestReviewScript`) and
the 3 new ones in `audio_handler_test.go` (Task 4) to pass a 6th argument — `newFakeCache()` (or
`erroringCache{}` where the test is specifically about cache behavior).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/... -v`
Expected: FAIL — compilation error, `NewServer` called with 6 args but takes 5.

- [ ] **Step 3: Write minimal implementation**

Update `internal/adapters/http/server.go`:

```go
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
	cache         ports.Cache
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
	}
	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.POST("/scripts/:id/review", s.reviewScript)
	return s
}
```

Add a small shared helper at the bottom of `server.go` (used by both handlers below):

```go
const cacheTTL = 5 * time.Minute

// cachedJSON essaie le cache d'abord ; sur miss ou erreur (fail-open), appelle
// compute, sert le résultat, et tente de le mettre en cache pour la prochaine
// fois — une erreur d'écriture cache est loguée mais ne fait jamais échouer la
// requête.
func (s *Server) cachedJSON(c echo.Context, key string, compute func() (any, int, error)) error {
	if cached, found, err := s.cache.Get(c.Request().Context(), key); err == nil && found {
		return c.JSONBlob(http.StatusOK, []byte(cached))
	}

	value, status, err := compute()
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return c.JSON(status, value)
	}

	body, err := json.Marshal(value)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	_ = s.cache.Set(c.Request().Context(), key, string(body), cacheTTL) // fail-open: erreur ignorée
	return c.JSONBlob(http.StatusOK, body)
}
```

Add `"encoding/json"`, `"net/http"`, `"time"` to `server.go`'s imports.

Update `internal/adapters/http/places_handler.go`'s `listPlaces` to route through it:

```go
func (s *Server) listPlaces(c echo.Context) error {
	key := fmt.Sprintf("places:%v:%v:%v:%v", rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
	return s.cachedJSON(c, key, func() (any, int, error) {
		places, err := s.placeRepo.FindActiveInBoundingBox(c.Request().Context(), rioMinLat, rioMinLon, rioMaxLat, rioMaxLon)
		if err != nil {
			return echo.Map{"error": err.Error()}, http.StatusInternalServerError, nil
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
		return resp, http.StatusOK, nil
	})
}
```

(The bounding box is currently fixed constants, not request params — see the existing `rioMinLat` etc. —
so the cache key is fixed too for now; if the route ever accepts real bounding-box query params, the key
must include them. Add `"fmt"` to this file's imports.)

Update `internal/adapters/http/audio_handler.go`'s `getPlaceAudio` similarly — cache only the `200 ready`
response (a 404/202 reflects transient generation state, not worth caching):

```go
func (s *Server) getPlaceAudio(c echo.Context) error {
	placeID := c.Param("id")
	language := c.QueryParam("language")
	if language == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "language query param is required"})
	}

	key := "audio:" + placeID + ":" + language
	if cached, found, err := s.cache.Get(c.Request().Context(), key); err == nil && found {
		return c.JSONBlob(http.StatusOK, []byte(cached))
	}

	script, err := s.scriptRepo.FindByPlaceIDAndLanguage(c.Request().Context(), placeID, language)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no script for this place/language"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	audioFile, err := s.audioFileRepo.FindByScriptID(c.Request().Context(), script.ID())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusNotFound, echo.Map{"error": "no audio ever requested for this script"})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	if audioFile.Status() != "ready" {
		return c.JSON(http.StatusAccepted, audioNotReadyResponse{Status: string(audioFile.Status())})
	}

	s3Key, err := parseS3Key(audioFile.Audio().StorageURL())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	url, err := s.storage.PresignURL(c.Request().Context(), s3Key, presignExpiry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	body, err := json.Marshal(audioResponse{URL: url})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	_ = s.cache.Set(c.Request().Context(), key, string(body), cacheTTL)
	return c.JSONBlob(http.StatusOK, body)
}
```

(This handler is simple enough to inline the cache check rather than route through `cachedJSON` — the
early not-ready/not-found returns don't fit that helper's single-success-path shape cleanly. Add
`"encoding/json"` to this file's imports.)

Update `cmd/api/main.go` to wire Redis:

```go
	// add to imports: goredis "github.com/redis/go-redis/v9", "rioaudioguide/backend/internal/adapters/redis"

	redisClient := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_URL", "localhost:6379")})
	cache := redis.NewCache(redisClient)

	server := httpadapter.NewServer(placeRepo, scriptRepo, audioFileRepo, publisher, storage, cache)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS (all tests, including the two new cache-behavior tests)

Run: `go build ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/server.go internal/adapters/http/places_handler.go \
        internal/adapters/http/audio_handler.go internal/adapters/http/server_test.go \
        internal/adapters/http/audio_handler_test.go cmd/api/main.go
git commit -m "Add cache-aside to GET /places and GET /places/:id/audio"
```

---

### Task 7: Helm — Redis dans le chart, et fermer un vrai trou trouvé pendant la Tâche 4

**Files:**
- Modify: `deploy/helm/rio-backend/templates/api-deployment.yaml`
- Modify: `cmd/api/main.go`, `cmd/worker/main.go` (nom de bucket par défaut incohérent, trouvé pendant
  la Tâche 4)

**Interfaces:**
- Consumes: rien de nouveau côté code pour Redis — uniquement le chart.
- Produces: `REDIS_URL`, `S3_BUCKET`, et les credentials AWS optionnels disponibles sur `rio-api` (pas
  `REDIS_URL` sur `rio-worker`, qui ne sert pas de lectures — mais `rio-worker` a déjà `S3_BUCKET`/AWS).

**Contexte** : `Chart.yaml` n'a aucune dépendance déclarée (vérifié — pas de section `dependencies`) ;
Postgres et RabbitMQ sur `kind` sont des releases Bitnami séparées (`helm install demo-postgres
bitnami/postgresql`), pas des dépendances de chart formelles — Redis suit le même pattern, documenté
plutôt que déclaré dans `Chart.yaml`.

**Trou trouvé pendant la Tâche 4** (pas anticipé au moment d'écrire ce plan) : `api-deployment.yaml`
n'a aujourd'hui que `DATABASE_URL`/`RABBITMQ_URL` — aucun `S3_BUCKET` ni credentials AWS, alors que
`cmd/api/main.go` (Tâche 4) charge maintenant la config AWS et crée un client S3 pour la présignation.
Sans ça, chaque appel réel à la nouvelle route échouerait en 500 sur un vrai déploiement (aucune
credential résolvable au moment de signer). `rio-worker` a déjà ce câblage — on le reproduit ici.

- [ ] **Step 1: Add Redis, S3_BUCKET, and AWS creds to the API deployment**

In `deploy/helm/rio-backend/templates/api-deployment.yaml`, add after the existing `RABBITMQ_URL` entry
(mirrors `worker-deployment.yaml`'s existing `S3_BUCKET`/AWS block exactly, plus the new `REDIS_URL`):

```yaml
            - name: REDIS_URL
              value: "{{ .Values.redis.url | default "demo-redis-master:6379" }}"
            - name: S3_BUCKET
              value: "{{ .Values.s3.bucket }}"
            - name: AWS_ACCESS_KEY_ID
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: aws-access-key-id, optional: true } }
            - name: AWS_SECRET_ACCESS_KEY
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: aws-secret-access-key, optional: true } }
            - name: AWS_SESSION_TOKEN
              valueFrom: { secretKeyRef: { name: {{ .Values.secrets.name }}, key: aws-session-token, optional: true } }
            - name: AWS_REGION
              value: "{{ .Values.s3.region | default "us-east-1" }}"
```

- [ ] **Step 2: Verify the chart renders**

Run: `helm template deploy/helm/rio-backend`
Expected: no errors; `REDIS_URL`, `S3_BUCKET`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
`AWS_SESSION_TOKEN`, `AWS_REGION` all appear in the rendered `rio-api` Deployment's env; `REDIS_URL` is
absent from `rio-worker`'s (it doesn't serve reads); `rio-worker`'s pre-existing `S3_BUCKET`/AWS block is
unchanged.

- [ ] **Step 3: Fix the mismatched bucket-name fallback default**

Found alongside the gap above: `cmd/worker/main.go` and (Task 4's new) `cmd/api/main.go` both fall back to
`envOr("S3_BUCKET", "rio-audioguide-bucket")` — but the real bucket (created tonight, in
`values.yaml`'s `s3.bucket`) is `rio-audio-guide` (with hyphens). Low real risk today (the Helm chart
always sets `S3_BUCKET` explicitly, so the fallback is dead code in practice) but confusing and worth
fixing while touching this area. In both files, change the fallback string from `"rio-audioguide-bucket"`
to `"rio-audio-guide"`.

- [ ] **Step 4: Verify**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/rio-backend/templates/api-deployment.yaml cmd/api/main.go cmd/worker/main.go
git commit -m "Wire Redis/S3/AWS into the API deployment, fix mismatched bucket fallback"
```

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/rio-backend/
git commit -m "Add Redis to the Helm chart, wired to the API deployment only"
```

---

### Task 8: Classification transitoire/permanent des erreurs S3

**Files:**
- Modify: `internal/adapters/s3/audio_storage.go`
- Modify: `internal/adapters/s3/audio_storage_test.go`
- Modify: `internal/adapters/rabbitmq/worker.go`
- Modify: `internal/adapters/rabbitmq/worker_test.go`

**Interfaces:**
- Consumes: `ports.PermanentError` (déjà créé pour ElevenLabs, `internal/ports/tts_generator.go`).
- Produces: `AudioStorage.Upload` renvoie un `*ports.PermanentError` sur les codes non-récupérables ;
  `worker.go` applique la même règle sur la branche upload que sur la branche TTS.

- [ ] **Step 1: Write the failing test**

Append to `internal/adapters/s3/audio_storage_test.go`:

```go
func TestAudioStorage_Upload_ClassifiesInvalidCredentialsAsPermanent(t *testing.T) {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("invalid-key-that-does-not-exist", "invalid-secret", ""),
	}
	client := s3.NewFromConfig(cfg)
	storage := NewAudioStorage(client, "rio-audio-guide")

	_, err := storage.Upload(context.Background(), "test.mp3", []byte("data"), "audio/mpeg")

	var permErr *ports.PermanentError
	if !errors.As(err, &permErr) {
		t.Fatalf("got error %v, want a *ports.PermanentError (InvalidAccessKeyId is never resolved by retrying)", err)
	}
}
```

Add `"errors"` and `"rioaudioguide/backend/internal/ports"` to that file's imports. This test makes a
real (failing) network call to AWS — no `S3_TEST_BUCKET` skip needed since invalid credentials fail before
reaching the bucket-existence question, but note in the commit if this proves flaky in CI without network
access and needs an `integration` build tag added retroactively.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/s3/... -run TestAudioStorage_Upload_ClassifiesInvalidCredentialsAsPermanent -v`
Expected: FAIL — `Upload` currently returns a plain error, not `*ports.PermanentError`.

- [ ] **Step 3: Write minimal implementation**

Update `Upload` in `internal/adapters/s3/audio_storage.go`:

```go
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"rioaudioguide/backend/internal/ports"
)

func (a *AudioStorage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && isPermanentS3Error(apiErr.ErrorCode()) {
			return "", &ports.PermanentError{StatusCode: 0, Body: apiErr.ErrorMessage()}
		}
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", a.bucket, key), nil
}

// isPermanentS3Error : codes qui ne se résoudront jamais en réessayant la même
// requête — credentials invalides, permissions refusées, bucket absent.
// Tout le reste (SlowDown, InternalError, ServiceUnavailable, timeouts réseau)
// reste transitoire, géré par le Nack(requeue=true) déjà en place.
func isPermanentS3Error(code string) bool {
	switch code {
	case "InvalidAccessKeyId", "AccessDenied", "SignatureDoesNotMatch", "NoSuchBucket":
		return true
	default:
		return false
	}
}
```

`github.com/aws/smithy-go` is already an indirect dependency (pulled in by `aws-sdk-go-v2`) — no new
`go.mod` entry expected, only a possible `// indirect` → direct comment update via `go mod tidy`.

`ports.PermanentError.StatusCode` is set to `0` here (S3 errors don't carry an HTTP status the same way
ElevenLabs's do) — the field stays meaningful for the ElevenLabs case; `0` is an acceptable placeholder
for S3 since only `errors.As` type-matching is used to detect permanence, not the status code's value.

Update `internal/adapters/rabbitmq/worker.go`'s upload-failure branch in `handle()` to match the TTS
branch's pattern:

```go
	storageURL, err := w.storage.Upload(ctx, job.AudioFileID+".mp3", audioBytes, "audio/mpeg")
	if err != nil {
		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			log.Printf("tts worker: permanent S3 error for %s, marking failed: %v", job.AudioFileID, err)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			_ = msg.Ack(false)
			return
		}
		log.Printf("tts worker: upload failed for %s: %v", job.AudioFileID, err)
		time.Sleep(requeueDelay)
		_ = msg.Nack(false, true)
		return
	}
```

- [ ] **Step 4: Add a worker-level integration test for the permanent-upload path**

Append to `internal/adapters/rabbitmq/worker_test.go` — mirrors
`TestWorker_PermanentTTSError_MarksAudioFileFailedAndAcks` but on the storage side:

```go
type failingStorage struct{ err error }

func (f failingStorage) Upload(_ context.Context, _ string, _ []byte, _ string) (string, error) {
	return "", f.err
}
func (f failingStorage) PresignURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("not implemented in fake")
}

func TestWorker_PermanentS3Error_MarksAudioFileFailedAndAcks(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	permErr := &ports.PermanentError{StatusCode: 0, Body: "InvalidAccessKeyId"}
	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, failingStorage{err: permErr}, fakeTTSGenerator{})
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

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -tags=integration ./internal/adapters/s3/... ./internal/adapters/rabbitmq/... -v`
Expected: PASS (requires local RabbitMQ; the new S3 permanent-error test makes a real network call to
AWS with invalid credentials — requires network access, not a live bucket)

Run: `go build ./... && go vet ./... && gofmt -l .`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/s3/audio_storage.go internal/adapters/s3/audio_storage_test.go \
        internal/adapters/rabbitmq/worker.go internal/adapters/rabbitmq/worker_test.go
git commit -m "Classify permanent S3 upload errors, stop the infinite retry loop"
```
