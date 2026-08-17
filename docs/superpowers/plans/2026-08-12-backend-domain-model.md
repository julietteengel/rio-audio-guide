# Backend Domain Model Implementation Plan

> **Note — this plan is not for agentic execution.** The backend is explicitly hand-written, not
> AI-generated (`mission.md`). This is a guide for the author to follow herself, task by task, at her
> own pace across sessions. Claude's role is limited to explaining concepts before a task, answering
> compile/debug questions, and reviewing the code once written — never writing `.go` files. The one
> exception already exercised: the empty package skeleton (folders, package-declaration-only files,
> `go.mod`), committed once on the `backend` branch (`76c9e68`). Steps use checkbox (`- [ ]`) syntax to
> track your own progress, not an agent's.

**Goal:** Implement the domain model and Postgres persistence for the content-publishing workflow
(Place, Script, AudioFile) in Go, hexagonal architecture, as the foundation the rest of the backend
builds on.

**Architecture:** Pure domain entities with invariants in `internal/domain` (zero dependencies).
Application use cases in `internal/application` orchestrate the domain through port interfaces defined
in `internal/ports`. Concrete Postgres and RabbitMQ adapters in `internal/adapters` implement those
ports. `cmd/api` wires it all together. Matches
`docs/superpowers/specs/2026-08-12-backend-domain-model-design.md`.

**Tech Stack:** Go 1.24. Standard library `testing` package with table-driven tests — no assertion
library, to keep dependencies minimal and the code idiomatic. `github.com/jackc/pgx/v5` for Postgres
(recommended: actively maintained, idiomatic `context`-first API; swap it if you'd rather use
`database/sql` + `lib/pq`, the plan's adapter code will need adjusting but the ports stay the same).
`github.com/rabbitmq/amqp091-go` for RabbitMQ (the maintained successor to `streadway/amqp`).
Integration tests (real Postgres/RabbitMQ) use the `//go:build integration` tag, mirroring the Python
pipeline's `pytest -m "not integration"` convention — `go test ./...` skips them, `go test
-tags=integration ./...` runs them.

## Global Constraints

- Hexagonal layering: `internal/domain` has zero imports outside the standard library — no `pgx`, no
  `amqp091-go`, nothing. If a domain file needs an import beyond `errors`, `time`, `crypto/rand`,
  `fmt`, stop and reconsider — that logic likely belongs in `application` or `adapters`.
- Place: near-read-only from the pipeline import; editable only through a narrow command (name,
  category, coordinates, `wikidata_qid`); soft-delete with a reason, not physical deletion; no version
  history; no reconciliation logic with a future pipeline reimport (deferred).
- Script: `draft → reviewed → published`; `published` is a stored field transitioned by the application
  layer when the associated AudioFile becomes ready — never a computed condition. Publication is per
  language variant, not per place.
- AudioFile: separate aggregate, `queued → generating → ready` or `failed`, with a `Retry` transition
  back to `queued` (RabbitMQ retries/DLQ are decided in `2026-08-04-backend-stack-decision.md`; this
  plan models the domain-side state, not the DLQ topology itself — see "What this plan doesn't cover").
- Schema: `places`, `scripts`, `audio_files` as specified in the design doc, with `places.geom` as
  `geography(Point,4326)` and a GIST index — PostGIS is used for bounding-box queries (the app's
  offline-download bundle, the admin map), not live nearest-neighbor, since proximity detection runs
  client-side (`2026-08-12-guide-runtime-v1-scope-design.md`).

---

## File Structure

Already scaffolded (empty, package-declaration only) on the `backend` branch, commit `76c9e68`
(flattened to sit at the worktree root in `fe1615e` — paths below are relative to
`.worktrees/`, no `` prefix):

```
.worktrees/  (worktree root — also has the inherited docs/, mission.md, CLAUDE.md)
  go.mod
  internal/
    domain/
      place.go            place_test.go
      script.go           script_test.go
      audiofile.go        audiofile_test.go
    application/
      request_audio_generation.go   request_audio_generation_test.go
      publish_script.go             publish_script_test.go
    ports/
      place_repository.go
      script_repository.go
      audiofile_repository.go
      audio_job_publisher.go
    adapters/
      postgres/
        place_repository.go       place_repository_test.go
        script_repository.go      script_repository_test.go
        audiofile_repository.go   audiofile_repository_test.go
      rabbitmq/
        audio_job_publisher.go    audio_job_publisher_test.go
  cmd/
    api/
      main.go
```

Files you'll create beyond this skeleton (`schema.sql`, any small helper files) are just normal
development — the one-time exception was the initial skeleton only.

---

### Task 1: `Place` domain entity

**Files:**
- Modify: `internal/domain/place.go`
- Test: `internal/domain/place_test.go`

**Interfaces:**
- Produces: `Place` struct (`ID, Name, Category, Lat, Lon, WikidataQID, Source, SourceRichness string;
  Status PlaceStatus; RemovedReason string`), `PlaceStatus` (`PlaceStatusActive`, `PlaceStatusRemoved`),
  `NewPlace(name, category string, lat, lon float64, wikidataQID, source, sourceRichness string) (*Place,
  error)`, `(*Place).Edit(name, category string, lat, lon float64, wikidataQID string) error`,
  `(*Place).Remove(reason string) error`. Sentinel errors: `ErrPlaceNameRequired`,
  `ErrPlaceInvalidCoords`, `ErrPlaceAlreadyRemoved`, `ErrPlaceRemoved`. Also produces the package-level
  `newID()` helper (stdlib-only UUID v4, no dependency) that `Script` and `AudioFile` will reuse in
  Tasks 2–3.

- [ ] **Step 1: Write the failing tests**

```go
// internal/domain/place_test.go
package domain

import (
	"errors"
	"testing"
)

func TestNewPlace(t *testing.T) {
	tests := []struct {
		name     string
		place    string
		category string
		lat, lon float64
		wantErr  error
	}{
		{name: "valid place", place: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, wantErr: nil},
		{name: "empty name", place: "", category: "monument", lat: -22.95, lon: -43.21, wantErr: ErrPlaceNameRequired},
		{name: "invalid latitude", place: "Test", category: "monument", lat: 200, lon: -43.21, wantErr: ErrPlaceInvalidCoords},
		{name: "invalid longitude", place: "Test", category: "monument", lat: -22.95, lon: 200, wantErr: ErrPlaceInvalidCoords},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPlace(tt.place, tt.category, tt.lat, tt.lon, "", "overture", "rich")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID == "" {
				t.Fatal("expected a generated ID, got empty string")
			}
			if p.Status != PlaceStatusActive {
				t.Fatalf("got status %v, want active", p.Status)
			}
		})
	}
}

func TestPlace_Remove(t *testing.T) {
	p, err := NewPlace("Test", "monument", -22.95, -43.21, "", "overture", "rich")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	if err := p.Remove("hors périmètre municipal"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != PlaceStatusRemoved {
		t.Fatalf("got status %v, want removed", p.Status)
	}
	if p.RemovedReason != "hors périmètre municipal" {
		t.Fatalf("got reason %q, want %q", p.RemovedReason, "hors périmètre municipal")
	}
	if err := p.Remove("again"); !errors.Is(err, ErrPlaceAlreadyRemoved) {
		t.Fatalf("got error %v, want ErrPlaceAlreadyRemoved", err)
	}
}

func TestPlace_Edit(t *testing.T) {
	p, _ := NewPlace("Test", "monument", -22.95, -43.21, "", "overture", "rich")

	if err := p.Edit("New Name", "museum", -22.90, -43.20, "Q123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "New Name" || p.Category != "museum" || p.WikidataQID != "Q123" {
		t.Fatalf("edit did not apply: %+v", p)
	}

	if err := p.Edit("", "museum", -22.90, -43.20, ""); !errors.Is(err, ErrPlaceNameRequired) {
		t.Fatalf("got error %v, want ErrPlaceNameRequired", err)
	}
}

func TestPlace_Edit_RejectsWhenRemoved(t *testing.T) {
	p, _ := NewPlace("Test", "monument", -22.95, -43.21, "", "overture", "rich")
	_ = p.Remove("test")
	if err := p.Edit("New Name", "monument", -22.95, -43.21, ""); !errors.Is(err, ErrPlaceRemoved) {
		t.Fatalf("got error %v, want ErrPlaceRemoved", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd backend && go test ./internal/domain/... -run TestPlace -v`
Expected: build failure (`Place`, `NewPlace` etc. undefined) — that's the correct starting point.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/domain/place.go
package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
)

type PlaceStatus string

const (
	PlaceStatusActive  PlaceStatus = "active"
	PlaceStatusRemoved PlaceStatus = "removed"
)

var (
	ErrPlaceNameRequired   = errors.New("place: name is required")
	ErrPlaceInvalidCoords  = errors.New("place: coordinates out of range")
	ErrPlaceAlreadyRemoved = errors.New("place: already removed")
	ErrPlaceRemoved        = errors.New("place: cannot edit a removed place")
)

type Place struct {
	ID             string
	Name           string
	Category       string
	Lat            float64
	Lon            float64
	WikidataQID    string
	Source         string
	SourceRichness string
	Status         PlaceStatus
	RemovedReason  string
}

func NewPlace(name, category string, lat, lon float64, wikidataQID, source, sourceRichness string) (*Place, error) {
	if name == "" {
		return nil, ErrPlaceNameRequired
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, ErrPlaceInvalidCoords
	}
	return &Place{
		ID:             newID(),
		Name:           name,
		Category:       category,
		Lat:            lat,
		Lon:            lon,
		WikidataQID:    wikidataQID,
		Source:         source,
		SourceRichness: sourceRichness,
		Status:         PlaceStatusActive,
	}, nil
}

func (p *Place) Edit(name, category string, lat, lon float64, wikidataQID string) error {
	if p.Status == PlaceStatusRemoved {
		return ErrPlaceRemoved
	}
	if name == "" {
		return ErrPlaceNameRequired
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return ErrPlaceInvalidCoords
	}
	p.Name = name
	p.Category = category
	p.Lat = lat
	p.Lon = lon
	p.WikidataQID = wikidataQID
	return nil
}

func (p *Place) Remove(reason string) error {
	if p.Status == PlaceStatusRemoved {
		return ErrPlaceAlreadyRemoved
	}
	p.Status = PlaceStatusRemoved
	p.RemovedReason = reason
	return nil
}

// newID generates a stdlib-only UUID v4, shared by Place, Script and AudioFile —
// avoids pulling a dependency into a package that must stay dependency-free.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/... -run TestPlace -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/place.go internal/domain/place_test.go
git commit -m "Add Place domain entity with edit/remove invariants"
```

---

### Task 2: `Script` domain entity

**Files:**
- Modify: `internal/domain/script.go`
- Test: `internal/domain/script_test.go`

**Interfaces:**
- Consumes: `newID()` from Task 1 (same package, no import needed).
- Produces: `Script` struct (`ID, PlaceID, Text, SourceText, Reviewer string; Language Language; Status
  ScriptStatus; ReviewedAt, PublishedAt time.Time`), `Language` (`LanguageEN`, `LanguageFR`,
  `LanguagePT`, `LanguageES`), `ScriptStatus` (`ScriptStatusDraft`, `ScriptStatusReviewed`,
  `ScriptStatusPublished`), `NewScript(placeID string, language Language, text, sourceText string)
  (*Script, error)`, `(*Script).MarkReviewed(reviewer string) error`, `(*Script).Publish() error`.
  Sentinel errors: `ErrScriptTextRequired`, `ErrScriptInvalidLanguage`, `ErrScriptNotDraft`,
  `ErrScriptNotReviewed`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/domain/script_test.go
package domain

import (
	"errors"
	"testing"
)

func TestNewScript(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		text     string
		wantErr  error
	}{
		{name: "valid FR script", language: LanguageFR, text: "Le Cristo Redentor...", wantErr: nil},
		{name: "empty text", language: LanguageFR, text: "", wantErr: ErrScriptTextRequired},
		{name: "unsupported language", language: Language("de"), text: "Text", wantErr: ErrScriptInvalidLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScript("place-1", tt.language, tt.text, "source text")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Status != ScriptStatusDraft {
				t.Fatalf("got status %v, want draft", s.Status)
			}
		})
	}
}

func TestScript_MarkReviewedThenPublish(t *testing.T) {
	s, _ := NewScript("place-1", LanguageFR, "Text", "source")

	if err := s.Publish(); !errors.Is(err, ErrScriptNotReviewed) {
		t.Fatalf("got error %v, want ErrScriptNotReviewed (can't publish a draft)", err)
	}

	if err := s.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != ScriptStatusReviewed || s.Reviewer != "julie" {
		t.Fatalf("review did not apply: %+v", s)
	}

	if err := s.MarkReviewed("julie"); !errors.Is(err, ErrScriptNotDraft) {
		t.Fatalf("got error %v, want ErrScriptNotDraft (already reviewed)", err)
	}

	if err := s.Publish(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != ScriptStatusPublished {
		t.Fatalf("got status %v, want published", s.Status)
	}
	if s.PublishedAt.IsZero() {
		t.Fatal("expected PublishedAt to be set")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/... -run TestScript -v`
Expected: build failure — `Script` etc. undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/domain/script.go
package domain

import (
	"errors"
	"time"
)

type ScriptStatus string

const (
	ScriptStatusDraft     ScriptStatus = "draft"
	ScriptStatusReviewed  ScriptStatus = "reviewed"
	ScriptStatusPublished ScriptStatus = "published"
)

type Language string

const (
	LanguageEN Language = "en"
	LanguageFR Language = "fr"
	LanguagePT Language = "pt"
	LanguageES Language = "es"
)

func (l Language) valid() bool {
	switch l {
	case LanguageEN, LanguageFR, LanguagePT, LanguageES:
		return true
	}
	return false
}

var (
	ErrScriptTextRequired    = errors.New("script: text is required")
	ErrScriptInvalidLanguage = errors.New("script: unsupported language")
	ErrScriptNotDraft        = errors.New("script: must be draft to be reviewed")
	ErrScriptNotReviewed     = errors.New("script: must be reviewed to be published")
)

type Script struct {
	ID          string
	PlaceID     string
	Language    Language
	Text        string
	SourceText  string
	Status      ScriptStatus
	Reviewer    string
	ReviewedAt  time.Time
	PublishedAt time.Time
}

func NewScript(placeID string, language Language, text, sourceText string) (*Script, error) {
	if text == "" {
		return nil, ErrScriptTextRequired
	}
	if !language.valid() {
		return nil, ErrScriptInvalidLanguage
	}
	return &Script{
		ID:         newID(),
		PlaceID:    placeID,
		Language:   language,
		Text:       text,
		SourceText: sourceText,
		Status:     ScriptStatusDraft,
	}, nil
}

func (s *Script) MarkReviewed(reviewer string) error {
	if s.Status != ScriptStatusDraft {
		return ErrScriptNotDraft
	}
	s.Status = ScriptStatusReviewed
	s.Reviewer = reviewer
	s.ReviewedAt = time.Now()
	return nil
}

func (s *Script) Publish() error {
	if s.Status != ScriptStatusReviewed {
		return ErrScriptNotReviewed
	}
	s.Status = ScriptStatusPublished
	s.PublishedAt = time.Now()
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/... -run TestScript -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/script.go internal/domain/script_test.go
git commit -m "Add Script domain entity with draft/reviewed/published lifecycle"
```

---

### Task 3: `AudioFile` domain entity

**Files:**
- Modify: `internal/domain/audiofile.go`
- Test: `internal/domain/audiofile_test.go`

**Interfaces:**
- Consumes: `newID()` from Task 1.
- Produces: `AudioFile` struct (`ID, ScriptID, VoiceID, StorageURL, TimestampsURL, FailureReason string;
  Status AudioFileStatus; Duration time.Duration`), `AudioFileStatus` (`AudioFileStatusQueued`,
  `AudioFileStatusGenerating`, `AudioFileStatusReady`, `AudioFileStatusFailed`),
  `NewAudioFile(scriptID, voiceID string) (*AudioFile, error)`, `(*AudioFile).MarkGenerating() error`,
  `(*AudioFile).MarkReady(storageURL, timestampsURL string, duration time.Duration) error`,
  `(*AudioFile).MarkFailed(reason string) error`, `(*AudioFile).Retry() error`. Sentinel errors:
  `ErrAudioFileScriptIDRequired`, `ErrAudioFileNotQueued`, `ErrAudioFileNotGenerating`,
  `ErrAudioFileNotFailed`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/domain/audiofile_test.go
package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewAudioFile(t *testing.T) {
	a, err := NewAudioFile("script-1", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AudioFileStatusQueued {
		t.Fatalf("got status %v, want queued", a.Status)
	}

	if _, err := NewAudioFile("", "voice-1"); !errors.Is(err, ErrAudioFileScriptIDRequired) {
		t.Fatalf("got error %v, want ErrAudioFileScriptIDRequired", err)
	}
}

func TestAudioFile_FullLifecycle(t *testing.T) {
	a, _ := NewAudioFile("script-1", "voice-1")

	if err := a.MarkReady("s3://bucket/audio.mp3", "s3://bucket/audio.json", 42*time.Second); !errors.Is(err, ErrAudioFileNotGenerating) {
		t.Fatalf("got error %v, want ErrAudioFileNotGenerating (still queued)", err)
	}

	if err := a.MarkGenerating(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := a.MarkGenerating(); !errors.Is(err, ErrAudioFileNotQueued) {
		t.Fatalf("got error %v, want ErrAudioFileNotQueued (already generating)", err)
	}

	if err := a.MarkReady("s3://bucket/audio.mp3", "s3://bucket/audio.json", 42*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AudioFileStatusReady || a.StorageURL == "" {
		t.Fatalf("mark ready did not apply: %+v", a)
	}
}

func TestAudioFile_FailAndRetry(t *testing.T) {
	a, _ := NewAudioFile("script-1", "voice-1")
	_ = a.MarkGenerating()

	if err := a.MarkFailed("TTS quota exceeded"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AudioFileStatusFailed || a.FailureReason != "TTS quota exceeded" {
		t.Fatalf("mark failed did not apply: %+v", a)
	}

	if err := a.Retry(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status != AudioFileStatusQueued || a.FailureReason != "" {
		t.Fatalf("retry did not reset state: %+v", a)
	}

	if err := a.Retry(); !errors.Is(err, ErrAudioFileNotFailed) {
		t.Fatalf("got error %v, want ErrAudioFileNotFailed (not failed)", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/domain/... -run TestAudioFile -v`
Expected: build failure — `AudioFile` etc. undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/domain/audiofile.go
package domain

import (
	"errors"
	"time"
)

type AudioFileStatus string

const (
	AudioFileStatusQueued     AudioFileStatus = "queued"
	AudioFileStatusGenerating AudioFileStatus = "generating"
	AudioFileStatusReady      AudioFileStatus = "ready"
	AudioFileStatusFailed     AudioFileStatus = "failed"
)

var (
	ErrAudioFileScriptIDRequired = errors.New("audio file: script id is required")
	ErrAudioFileNotQueued        = errors.New("audio file: must be queued to start generating")
	ErrAudioFileNotGenerating    = errors.New("audio file: must be generating to complete or fail")
	ErrAudioFileNotFailed        = errors.New("audio file: must be failed to retry")
)

type AudioFile struct {
	ID            string
	ScriptID      string
	VoiceID       string
	Status        AudioFileStatus
	StorageURL    string
	TimestampsURL string
	Duration      time.Duration
	FailureReason string
}

func NewAudioFile(scriptID, voiceID string) (*AudioFile, error) {
	if scriptID == "" {
		return nil, ErrAudioFileScriptIDRequired
	}
	return &AudioFile{
		ID:       newID(),
		ScriptID: scriptID,
		VoiceID:  voiceID,
		Status:   AudioFileStatusQueued,
	}, nil
}

func (a *AudioFile) MarkGenerating() error {
	if a.Status != AudioFileStatusQueued {
		return ErrAudioFileNotQueued
	}
	a.Status = AudioFileStatusGenerating
	return nil
}

func (a *AudioFile) MarkReady(storageURL, timestampsURL string, duration time.Duration) error {
	if a.Status != AudioFileStatusGenerating {
		return ErrAudioFileNotGenerating
	}
	a.Status = AudioFileStatusReady
	a.StorageURL = storageURL
	a.TimestampsURL = timestampsURL
	a.Duration = duration
	return nil
}

func (a *AudioFile) MarkFailed(reason string) error {
	if a.Status != AudioFileStatusGenerating {
		return ErrAudioFileNotGenerating
	}
	a.Status = AudioFileStatusFailed
	a.FailureReason = reason
	return nil
}

func (a *AudioFile) Retry() error {
	if a.Status != AudioFileStatusFailed {
		return ErrAudioFileNotFailed
	}
	a.Status = AudioFileStatusQueued
	a.FailureReason = ""
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/domain/... -v`
Expected: PASS — every test in the `domain` package, not just `AudioFile`. Good moment to confirm
Tasks 1–3 all still pass together.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/audiofile.go internal/domain/audiofile_test.go
git commit -m "Add AudioFile domain entity with queued/generating/ready/failed lifecycle"
```

---

### Task 4: Ports (repository and publisher interfaces)

**Files:**
- Modify: `internal/ports/place_repository.go`
- Modify: `internal/ports/script_repository.go`
- Modify: `internal/ports/audiofile_repository.go`
- Modify: `internal/ports/audio_job_publisher.go`

**Interfaces:**
- Consumes: `domain.Place`, `domain.Script`, `domain.AudioFile` from Tasks 1–3.
- Produces: `PlaceRepository`, `ScriptRepository`, `AudioFileRepository`, `AudioJobPublisher`
  interfaces, consumed by the application layer (Tasks 5–6) and implemented by the adapters (Tasks
  7–10).

Interfaces have no behavior of their own to red/green test — this task is definition + a compile check
instead of the usual TDD cycle.

- [ ] **Step 1: Write the interfaces**

```go
// internal/ports/place_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type PlaceRepository interface {
	Save(ctx context.Context, place *domain.Place) error
	FindByID(ctx context.Context, id string) (*domain.Place, error)
	FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error)
}
```

```go
// internal/ports/script_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type ScriptRepository interface {
	Save(ctx context.Context, script *domain.Script) error
	FindByID(ctx context.Context, id string) (*domain.Script, error)
}
```

```go
// internal/ports/audiofile_repository.go
package ports

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type AudioFileRepository interface {
	Save(ctx context.Context, audioFile *domain.AudioFile) error
	FindByID(ctx context.Context, id string) (*domain.AudioFile, error)
}
```

```go
// internal/ports/audio_job_publisher.go
package ports

import "context"

// AudioJobPublisher is the outbound port to the TTS job queue (RabbitMQ, Task 10).
type AudioJobPublisher interface {
	PublishTTSJob(ctx context.Context, audioFileID, scriptID, text, language, voiceID string) error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/ports/...`
Expected: no output, exit code 0.

- [ ] **Step 3: Commit**

```bash
git add internal/ports/
git commit -m "Define repository and audio job publisher ports"
```

---

### Task 5: Application — `ReviewAndRequestAudio` use case

**Files:**
- Modify: `internal/application/request_audio_generation.go`
- Modify: `internal/application/request_audio_generation_test.go`

**Interfaces:**
- Consumes: `ports.ScriptRepository`, `ports.AudioFileRepository`, `ports.AudioJobPublisher` (Task 4);
  `domain.NewAudioFile` (Task 3).
- Produces: `ReviewAndRequestAudio(ctx context.Context, scriptRepo ports.ScriptRepository,
  audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher, scriptID, reviewer,
  voiceID string) error`. Also produces the in-memory fake repositories/publisher used across this
  task's tests and Task 6's — they live in this task's `_test.go` file but Go shares types across every
  `_test.go` file in the same package, so Task 6 reuses them without redefining.

This use case marks a Script reviewed and, in the same call, creates its AudioFile and publishes the
TTS job — reviewing and requesting audio happen together because there's no meaningful in-between state
for a script that's reviewed but has no audio job in flight yet.

- [ ] **Step 1: Write the failing test (including the fakes)**

```go
// internal/application/request_audio_generation_test.go
package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

type fakeScriptRepo struct {
	scripts map[string]*domain.Script
}

func newFakeScriptRepo() *fakeScriptRepo {
	return &fakeScriptRepo{scripts: map[string]*domain.Script{}}
}

func (f *fakeScriptRepo) Save(_ context.Context, s *domain.Script) error {
	f.scripts[s.ID] = s
	return nil
}

func (f *fakeScriptRepo) FindByID(_ context.Context, id string) (*domain.Script, error) {
	s, ok := f.scripts[id]
	if !ok {
		return nil, errors.New("script not found")
	}
	return s, nil
}

type fakeAudioFileRepo struct {
	files map[string]*domain.AudioFile
}

func newFakeAudioFileRepo() *fakeAudioFileRepo {
	return &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
}

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.files[a.ID] = a
	return nil
}

func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	a, ok := f.files[id]
	if !ok {
		return nil, errors.New("audio file not found")
	}
	return a, nil
}

type fakePublisher struct {
	published []string
}

func (f *fakePublisher) PublishTTSJob(_ context.Context, audioFileID, _, _, _, _ string) error {
	f.published = append(f.published, audioFileID)
	return nil
}

func TestReviewAndRequestAudio(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	publisher := &fakePublisher{}
	ctx := context.Background()

	script, err := domain.NewScript("place-1", domain.LanguageFR, "Le Cristo Redentor...", "source text")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("unexpected error saving fixture: %v", err)
	}

	if err := ReviewAndRequestAudio(ctx, scriptRepo, audioFileRepo, publisher, script.ID, "julie", "voice-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := scriptRepo.FindByID(ctx, script.ID)
	if saved.Status != domain.ScriptStatusReviewed {
		t.Fatalf("got status %v, want reviewed", saved.Status)
	}
	if len(audioFileRepo.files) != 1 {
		t.Fatalf("got %d audio files, want 1", len(audioFileRepo.files))
	}
	if len(publisher.published) != 1 {
		t.Fatalf("got %d published jobs, want 1", len(publisher.published))
	}
}

func TestReviewAndRequestAudio_UnknownScript(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	publisher := &fakePublisher{}

	err := ReviewAndRequestAudio(context.Background(), scriptRepo, audioFileRepo, publisher, "does-not-exist", "julie", "voice-1")
	if err == nil {
		t.Fatal("expected an error for an unknown script, got nil")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/application/... -run TestReviewAndRequestAudio -v`
Expected: build failure — `ReviewAndRequestAudio` undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/application/request_audio_generation.go
package application

import (
	"context"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

func ReviewAndRequestAudio(
	ctx context.Context,
	scriptRepo ports.ScriptRepository,
	audioFileRepo ports.AudioFileRepository,
	publisher ports.AudioJobPublisher,
	scriptID, reviewer, voiceID string,
) error {
	script, err := scriptRepo.FindByID(ctx, scriptID)
	if err != nil {
		return err
	}
	if err := script.MarkReviewed(reviewer); err != nil {
		return err
	}
	if err := scriptRepo.Save(ctx, script); err != nil {
		return err
	}

	audioFile, err := domain.NewAudioFile(script.ID, voiceID)
	if err != nil {
		return err
	}
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		return err
	}

	return publisher.PublishTTSJob(ctx, audioFile.ID, script.ID, script.Text, string(script.Language), voiceID)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/application/... -run TestReviewAndRequestAudio -v`
Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/application/request_audio_generation.go internal/application/request_audio_generation_test.go
git commit -m "Add ReviewAndRequestAudio use case"
```

---

### Task 6: Application — `StartAudioGeneration` and `CompleteAudioGeneration`

**Files:**
- Modify: `internal/application/publish_script.go`
- Modify: `internal/application/publish_script_test.go`

**Interfaces:**
- Consumes: `fakeScriptRepo`, `fakeAudioFileRepo` from Task 5 (same package, reused as-is).
- Produces: `StartAudioGeneration(ctx context.Context, audioFileRepo ports.AudioFileRepository,
  audioFileID string) error`, `CompleteAudioGeneration(ctx context.Context, scriptRepo
  ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, audioFileID, storageURL,
  timestampsURL string, duration time.Duration) error`.

Two separate functions, not one: the RabbitMQ worker (Task 10) calls `StartAudioGeneration` the moment
it picks up a job — so the stored state reflects reality while the slow TTS call is in flight — then
`CompleteAudioGeneration` once the TTS call returns. This is the function that makes a Script actually
go live, hence the file's name.

- [ ] **Step 1: Write the failing test**

```go
// internal/application/publish_script_test.go
package application

import (
	"context"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestStartAudioGeneration(t *testing.T) {
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	audioFile, _ := domain.NewAudioFile("script-1", "voice-1")
	_ = audioFileRepo.Save(ctx, audioFile)

	if err := StartAudioGeneration(ctx, audioFileRepo, audioFile.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := audioFileRepo.FindByID(ctx, audioFile.ID)
	if saved.Status != domain.AudioFileStatusGenerating {
		t.Fatalf("got status %v, want generating", saved.Status)
	}
}

func TestCompleteAudioGeneration(t *testing.T) {
	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()
	ctx := context.Background()

	script, _ := domain.NewScript("place-1", domain.LanguageFR, "Text", "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(ctx, script)

	audioFile, _ := domain.NewAudioFile(script.ID, "voice-1")
	_ = audioFile.MarkGenerating()
	_ = audioFileRepo.Save(ctx, audioFile)

	err := CompleteAudioGeneration(ctx, scriptRepo, audioFileRepo, audioFile.ID, "s3://bucket/a.mp3", "s3://bucket/a.json", 42*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	savedAudio, _ := audioFileRepo.FindByID(ctx, audioFile.ID)
	if savedAudio.Status != domain.AudioFileStatusReady {
		t.Fatalf("got audio status %v, want ready", savedAudio.Status)
	}

	savedScript, _ := scriptRepo.FindByID(ctx, script.ID)
	if savedScript.Status != domain.ScriptStatusPublished {
		t.Fatalf("got script status %v, want published", savedScript.Status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/application/... -run TestStartAudioGeneration -run TestCompleteAudioGeneration -v`
Expected: build failure — both functions undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/application/publish_script.go
package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/ports"
)

func StartAudioGeneration(ctx context.Context, audioFileRepo ports.AudioFileRepository, audioFileID string) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}
	if err := audioFile.MarkGenerating(); err != nil {
		return err
	}
	return audioFileRepo.Save(ctx, audioFile)
}

func CompleteAudioGeneration(
	ctx context.Context,
	scriptRepo ports.ScriptRepository,
	audioFileRepo ports.AudioFileRepository,
	audioFileID, storageURL, timestampsURL string,
	duration time.Duration,
) error {
	audioFile, err := audioFileRepo.FindByID(ctx, audioFileID)
	if err != nil {
		return err
	}
	if err := audioFile.MarkReady(storageURL, timestampsURL, duration); err != nil {
		return err
	}
	if err := audioFileRepo.Save(ctx, audioFile); err != nil {
		return err
	}

	script, err := scriptRepo.FindByID(ctx, audioFile.ScriptID)
	if err != nil {
		return err
	}
	if err := script.Publish(); err != nil {
		return err
	}
	return scriptRepo.Save(ctx, script)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/application/... -v`
Expected: PASS — every test in the `application` package (Task 5 and Task 6 together).

- [ ] **Step 5: Commit**

```bash
git add internal/application/publish_script.go internal/application/publish_script_test.go
git commit -m "Add StartAudioGeneration and CompleteAudioGeneration use cases"
```

---

### Task 7: Postgres schema + `PlaceRepository` adapter

**Files:**
- Create: `internal/adapters/postgres/schema.sql`
- Modify: `internal/adapters/postgres/place_repository.go`
- Modify: `internal/adapters/postgres/place_repository_test.go`

**Interfaces:**
- Consumes: `domain.Place`, `ports.PlaceRepository`.
- Produces: `NewPlaceRepository(pool *pgxpool.Pool) *PlaceRepository` satisfying `ports.PlaceRepository`.

This is the first task touching a real database — it needs Postgres with PostGIS running locally:

```bash
docker run --rm -d --name rio-postgres -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgis/postgis:16-3.4
```

- [ ] **Step 1: Add the pgx dependency**

```bash
go get github.com/jackc/pgx/v5/pgxpool
```

- [ ] **Step 2: Write the schema**

```sql
-- internal/adapters/postgres/schema.sql
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE places (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    category        TEXT NOT NULL,
    geom            geography(Point, 4326) NOT NULL,
    wikidata_qid    TEXT,
    source          TEXT NOT NULL,
    source_richness TEXT,
    status          TEXT NOT NULL DEFAULT 'active',
    removed_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX places_geom_idx ON places USING GIST (geom);

CREATE TABLE scripts (
    id           TEXT PRIMARY KEY,
    place_id     TEXT NOT NULL REFERENCES places(id),
    language     TEXT NOT NULL,
    text         TEXT NOT NULL,
    source_text  TEXT,
    status       TEXT NOT NULL DEFAULT 'draft',
    reviewer     TEXT,
    reviewed_at  TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (place_id, language)
);

CREATE TABLE audio_files (
    id             TEXT PRIMARY KEY,
    script_id      TEXT NOT NULL REFERENCES scripts(id),
    voice_id       TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'queued',
    storage_url    TEXT,
    timestamps_url TEXT,
    duration_ms    BIGINT,
    failure_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Apply it: `docker exec -i rio-postgres psql -U postgres -d postgres < internal/adapters/postgres/schema.sql`

`UNIQUE (place_id, language)` encodes a real invariant that Task 2 only enforced in memory: one script
per place per language.

- [ ] **Step 3: Write the failing integration test**

```go
// internal/adapters/postgres/place_repository_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:postgres@localhost:5432/postgres")
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPlaceRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	repo := NewPlaceRepository(pool)
	ctx := context.Background()

	place, err := domain.NewPlace("Cristo Redentor", "monument", -22.9519, -43.2105, "Q1963380", "wikidata", "rich")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}

	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindByID(ctx, place.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Name != place.Name || found.WikidataQID != place.WikidataQID {
		t.Fatalf("got %+v, want %+v", found, place)
	}

	if err := place.Remove("test cleanup"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := repo.Save(ctx, place); err != nil {
		t.Fatalf("save after remove: %v", err)
	}
	found, _ = repo.FindByID(ctx, place.ID)
	if found.Status != domain.PlaceStatusRemoved {
		t.Fatalf("got status %v, want removed", found.Status)
	}
}
```

- [ ] **Step 4: Run it to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: build failure — `PlaceRepository`/`NewPlaceRepository` undefined.

- [ ] **Step 5: Write the minimal implementation**

```go
// internal/adapters/postgres/place_repository.go
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type PlaceRepository struct {
	pool *pgxpool.Pool
}

func NewPlaceRepository(pool *pgxpool.Pool) *PlaceRepository {
	return &PlaceRepository{pool: pool}
}

func (r *PlaceRepository) Save(ctx context.Context, place *domain.Place) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO places (id, name, category, geom, wikidata_qid, source, source_richness, status, removed_reason)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			category = EXCLUDED.category,
			geom = EXCLUDED.geom,
			wikidata_qid = EXCLUDED.wikidata_qid,
			status = EXCLUDED.status,
			removed_reason = EXCLUDED.removed_reason,
			updated_at = now()
	`, place.ID, place.Name, place.Category, place.Lon, place.Lat, place.WikidataQID, place.Source,
		place.SourceRichness, string(place.Status), place.RemovedReason)
	return err
}

func (r *PlaceRepository) FindByID(ctx context.Context, id string) (*domain.Place, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''),
		       status, COALESCE(removed_reason, '')
		FROM places WHERE id = $1
	`, id)

	var p domain.Place
	var status string
	if err := row.Scan(&p.ID, &p.Name, &p.Category, &p.Lat, &p.Lon, &p.WikidataQID, &p.Source,
		&p.SourceRichness, &status, &p.RemovedReason); err != nil {
		return nil, err
	}
	p.Status = domain.PlaceStatus(status)
	return &p, nil
}

func (r *PlaceRepository) FindActiveInBoundingBox(ctx context.Context, minLat, minLon, maxLat, maxLon float64) ([]*domain.Place, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, category, ST_Y(geom::geometry), ST_X(geom::geometry),
		       COALESCE(wikidata_qid, ''), source, COALESCE(source_richness, ''), status
		FROM places
		WHERE status = 'active'
		  AND geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)::geography
	`, minLon, minLat, maxLon, maxLat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var places []*domain.Place
	for rows.Next() {
		var p domain.Place
		var status string
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Lat, &p.Lon, &p.WikidataQID, &p.Source,
			&p.SourceRichness, &status); err != nil {
			return nil, err
		}
		p.Status = domain.PlaceStatus(status)
		places = append(places, &p)
	}
	return places, rows.Err()
}
```

`FindByID` returns pgx's raw `pgx.ErrNoRows` when nothing matches, unwrapped — no domain-level
not-found error yet. Add that mapping when a caller actually needs to distinguish "not found" from
other failures; nothing in Tasks 1–6 does, so it's not built speculatively.

- [ ] **Step 6: Run it to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/postgres/schema.sql internal/adapters/postgres/place_repository.go internal/adapters/postgres/place_repository_test.go go.mod go.sum
git commit -m "Add Postgres schema and PlaceRepository adapter"
```

---

### Task 8: `ScriptRepository` Postgres adapter

**Files:**
- Modify: `internal/adapters/postgres/script_repository.go`
- Modify: `internal/adapters/postgres/script_repository_test.go`

**Interfaces:**
- Consumes: `domain.Script`, `ports.ScriptRepository`, the running Postgres from Task 7 (same schema).
- Produces: `NewScriptRepository(pool *pgxpool.Pool) *ScriptRepository` satisfying
  `ports.ScriptRepository`.

- [ ] **Step 1: Write the failing integration test**

```go
// internal/adapters/postgres/script_repository_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestScriptRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeRepo := NewPlaceRepository(pool)
	place, _ := domain.NewPlace("Escadaria Selarón", "monument", -22.9147, -43.1806, "", "overture", "correct")
	if err := placeRepo.Save(ctx, place); err != nil {
		t.Fatalf("save place fixture: %v", err)
	}

	scriptRepo := NewScriptRepository(pool)
	script, err := domain.NewScript(place.ID, domain.LanguageFR, "Voici l'escadaria...", "source text")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	_ = script.MarkReviewed("julie")

	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := scriptRepo.FindByID(ctx, script.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status != domain.ScriptStatusReviewed || found.Reviewer != "julie" {
		t.Fatalf("got %+v, want reviewed by julie", found)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestScriptRepository -v`
Expected: build failure — `ScriptRepository`/`NewScriptRepository` undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/adapters/postgres/script_repository.go
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type ScriptRepository struct {
	pool *pgxpool.Pool
}

func NewScriptRepository(pool *pgxpool.Pool) *ScriptRepository {
	return &ScriptRepository{pool: pool}
}

func (r *ScriptRepository) Save(ctx context.Context, script *domain.Script) error {
	var reviewedAt, publishedAt any
	if !script.ReviewedAt.IsZero() {
		reviewedAt = script.ReviewedAt
	}
	if !script.PublishedAt.IsZero() {
		publishedAt = script.PublishedAt
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO scripts (id, place_id, language, text, source_text, status, reviewer, reviewed_at, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			text = EXCLUDED.text,
			status = EXCLUDED.status,
			reviewer = EXCLUDED.reviewer,
			reviewed_at = EXCLUDED.reviewed_at,
			published_at = EXCLUDED.published_at,
			updated_at = now()
	`, script.ID, script.PlaceID, string(script.Language), script.Text, script.SourceText,
		string(script.Status), script.Reviewer, reviewedAt, publishedAt)
	return err
}

func (r *ScriptRepository) FindByID(ctx context.Context, id string) (*domain.Script, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, place_id, language, text, COALESCE(source_text, ''), status,
		       COALESCE(reviewer, ''), reviewed_at, published_at
		FROM scripts WHERE id = $1
	`, id)

	var s domain.Script
	var language, status string
	var reviewedAt, publishedAt *time.Time
	if err := row.Scan(&s.ID, &s.PlaceID, &language, &s.Text, &s.SourceText, &status,
		&s.Reviewer, &reviewedAt, &publishedAt); err != nil {
		return nil, err
	}
	s.Language = domain.Language(language)
	s.Status = domain.ScriptStatus(status)
	if reviewedAt != nil {
		s.ReviewedAt = *reviewedAt
	}
	if publishedAt != nil {
		s.PublishedAt = *publishedAt
	}
	return &s, nil
}
```

pgx maps a `NULL` `TIMESTAMPTZ` to a nil pointer when you scan into `*time.Time`; `domain.Script` uses
the zero `time.Time` to mean "not set" — the two `if reviewedAt != nil` checks are where that
translation happens, not boilerplate to skim past.

- [ ] **Step 4: Run it to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestScriptRepository -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/postgres/script_repository.go internal/adapters/postgres/script_repository_test.go
git commit -m "Add ScriptRepository adapter"
```

---

### Task 9: `AudioFileRepository` Postgres adapter

**Files:**
- Modify: `internal/adapters/postgres/audiofile_repository.go`
- Modify: `internal/adapters/postgres/audiofile_repository_test.go`

**Interfaces:**
- Consumes: `domain.AudioFile`, `ports.AudioFileRepository`, the Script fixture pattern from Task 8.
- Produces: `NewAudioFileRepository(pool *pgxpool.Pool) *AudioFileRepository` satisfying
  `ports.AudioFileRepository`.

- [ ] **Step 1: Write the failing integration test**

```go
// internal/adapters/postgres/audiofile_repository_test.go
//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestAudioFileRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	placeRepo := NewPlaceRepository(pool)
	place, _ := domain.NewPlace("Museu Nacional", "museum", -22.9058, -43.2256, "", "overture", "correct")
	_ = placeRepo.Save(ctx, place)

	scriptRepo := NewScriptRepository(pool)
	script, _ := domain.NewScript(place.ID, domain.LanguageEN, "The National Museum...", "source text")
	_ = scriptRepo.Save(ctx, script)

	audioRepo := NewAudioFileRepository(pool)
	audioFile, err := domain.NewAudioFile(script.ID, "voice-1")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	_ = audioFile.MarkGenerating()
	_ = audioFile.MarkReady("s3://bucket/a.mp3", "s3://bucket/a.json", 37*time.Second)

	if err := audioRepo.Save(ctx, audioFile); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := audioRepo.FindByID(ctx, audioFile.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status != domain.AudioFileStatusReady || found.StorageURL != audioFile.StorageURL {
		t.Fatalf("got %+v, want %+v", found, audioFile)
	}
	if found.Duration != 37*time.Second {
		t.Fatalf("got duration %v, want 37s", found.Duration)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestAudioFileRepository -v`
Expected: build failure — `AudioFileRepository`/`NewAudioFileRepository` undefined.

- [ ] **Step 3: Write the minimal implementation**

```go
// internal/adapters/postgres/audiofile_repository.go
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rioaudioguide/backend/internal/domain"
)

type AudioFileRepository struct {
	pool *pgxpool.Pool
}

func NewAudioFileRepository(pool *pgxpool.Pool) *AudioFileRepository {
	return &AudioFileRepository{pool: pool}
}

func (r *AudioFileRepository) Save(ctx context.Context, audioFile *domain.AudioFile) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audio_files (id, script_id, voice_id, status, storage_url, timestamps_url, duration_ms, failure_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			storage_url = EXCLUDED.storage_url,
			timestamps_url = EXCLUDED.timestamps_url,
			duration_ms = EXCLUDED.duration_ms,
			failure_reason = EXCLUDED.failure_reason,
			updated_at = now()
	`, audioFile.ID, audioFile.ScriptID, audioFile.VoiceID, string(audioFile.Status),
		audioFile.StorageURL, audioFile.TimestampsURL, audioFile.Duration.Milliseconds(), audioFile.FailureReason)
	return err
}

func (r *AudioFileRepository) FindByID(ctx context.Context, id string) (*domain.AudioFile, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, script_id, voice_id, status, COALESCE(storage_url, ''),
		       COALESCE(timestamps_url, ''), COALESCE(duration_ms, 0), COALESCE(failure_reason, '')
		FROM audio_files WHERE id = $1
	`, id)

	var a domain.AudioFile
	var status string
	var durationMs int64
	if err := row.Scan(&a.ID, &a.ScriptID, &a.VoiceID, &status, &a.StorageURL, &a.TimestampsURL,
		&durationMs, &a.FailureReason); err != nil {
		return nil, err
	}
	a.Status = domain.AudioFileStatus(status)
	a.Duration = time.Duration(durationMs) * time.Millisecond
	return &a, nil
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test -tags=integration ./internal/adapters/postgres/... -run TestAudioFileRepository -v`
Expected: PASS.

- [ ] **Step 5: Run the full integration suite together**

Run: `go test -tags=integration ./internal/adapters/postgres/... -v`
Expected: PASS — all three repositories.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/postgres/audiofile_repository.go internal/adapters/postgres/audiofile_repository_test.go
git commit -m "Add AudioFileRepository adapter"
```

---

### Task 10: `AudioJobPublisher` RabbitMQ adapter

**Files:**
- Modify: `internal/adapters/rabbitmq/audio_job_publisher.go`
- Modify: `internal/adapters/rabbitmq/audio_job_publisher_test.go`

**Interfaces:**
- Consumes: `ports.AudioJobPublisher`.
- Produces: `NewAudioJobPublisher(channel *amqp.Channel) (*AudioJobPublisher, error)` satisfying
  `ports.AudioJobPublisher`, and the exported `TTSJobQueue` queue-name constant that the future worker
  (not part of this plan — see "What this plan doesn't cover") will consume from.

Needs RabbitMQ running locally:

```bash
docker run --rm -d --name rio-rabbitmq -p 5672:5672 rabbitmq:3-management
```

- [ ] **Step 1: Add the amqp091-go dependency**

```bash
go get github.com/rabbitmq/amqp091-go
```

- [ ] **Step 2: Write the failing integration test**

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

func TestAudioJobPublisher_PublishTTSJob(t *testing.T) {
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

- [ ] **Step 3: Run it to verify it fails**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: build failure — `NewAudioJobPublisher`/`TTSJobQueue` undefined.

- [ ] **Step 4: Write the minimal implementation**

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

`QueueDeclare` here uses no dead-letter-exchange arguments — retries/DLQ topology is real, decided in
principle (`2026-08-04-backend-stack-decision.md`), but not designed at the level of exchange/queue
arguments yet. See "What this plan doesn't cover."

- [ ] **Step 5: Run it to verify it passes**

Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/rabbitmq/audio_job_publisher.go internal/adapters/rabbitmq/audio_job_publisher_test.go go.mod go.sum
git commit -m "Add AudioJobPublisher RabbitMQ adapter"
```

---

### Task 11: Wire `cmd/api/main.go`

**Files:**
- Modify: `cmd/api/main.go`

**Interfaces:**
- Consumes: every constructor from Tasks 7, 9, 10 (`postgres.NewPlaceRepository`,
  `postgres.NewScriptRepository`, `postgres.NewAudioFileRepository`, `rabbitmq.NewAudioJobPublisher`).
- Produces: a running composition root that connects to Postgres and RabbitMQ and confirms it's wired
  correctly. No HTTP server yet — deliberately out of scope, see below.

This task has no domain behavior to TDD — it's connecting real infrastructure, so "test" means running
the binary against the containers from Tasks 7 and 10 and reading the log line, not `go test`.

- [ ] **Step 1: Write `main.go`**

```go
// cmd/api/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"rioaudioguide/backend/internal/adapters/postgres"
	"rioaudioguide/backend/internal/adapters/rabbitmq"
)

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/postgres"
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitmqURL)
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

	// Nothing consumes these yet — no HTTP API surface has been designed (see the plan's
	// "What this plan doesn't cover"). Wired here so the next plan starts from a working
	// composition root instead of an empty main.
	_ = placeRepo
	_ = scriptRepo
	_ = audioFileRepo
	_ = publisher

	log.Println("backend wired: postgres and rabbitmq reachable")
}
```

- [ ] **Step 2: Run it against the containers from Tasks 7 and 10**

```bash
go run ./cmd/api
```

Expected: `backend wired: postgres and rabbitmq reachable`, process exits cleanly (nothing keeps it
running yet — that's expected, there's no server loop until the HTTP API is designed).

- [ ] **Step 3: Commit**

```bash
git add cmd/api/main.go
git commit -m "Wire composition root: postgres and rabbitmq connections"
```

---

## What this plan doesn't cover

Deliberately, not by oversight — each of these needs its own decision before it's built, same
discipline as everything else in this project:

- **HTTP API surface** — no routes, no handlers, no request/response shapes designed yet. `main.go`
  stops at wiring the repositories and publisher; the blank `_ =` assignments are a visible placeholder
  for "the next plan starts here," not a finished composition root.
- **RabbitMQ retry/DLQ topology** — decided in principle, not designed at the exchange/queue-argument
  level. Needs its own short design pass before Task 10's `QueueDeclare` grows dead-letter arguments.
- **The RabbitMQ consumer/worker** that calls `StartAudioGeneration`/`CompleteAudioGeneration` and
  actually talks to a TTS API — this plan builds the publisher side only.
- **Import script** from the Python pipeline's CSV output into `places`/`scripts` — needed before any
  of this is useful with real data (696 boundary-verified places, 187 narrated per `mission.md`), but
  it's a one-off script, not part of the hexagonal core, and deserves its own small plan.
- **Place edit/remove use cases** at the application layer — Task 1 built the domain invariants
  (`Edit`, `Remove`); nothing in `internal/application` calls them yet, since no caller (admin API)
  exists to call them from.
- **Partners and users** — explicitly out of scope for the design doc this plan implements.
- **Place ↔ pipeline reimport reconciliation** — explicitly deferred in the design doc.
