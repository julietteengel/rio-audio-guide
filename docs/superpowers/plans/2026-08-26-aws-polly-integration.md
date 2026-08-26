# AWS Polly Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ElevenLabs with Amazon Polly as the active `TTSGenerator`, wiring real word-level
timestamps through to `AudioFile.timestampsURL` for the first time (the field has existed since the
domain was designed but was never populated). ElevenLabs code stays in the repo, disconnected from
`cmd/worker/main.go`.

**Architecture:** `ports.TTSGenerator.Generate` changes shape: instead of returning raw audio bytes
for the worker to upload, it now returns final `s3://` URLs directly, because Amazon Polly's
long-text API (`StartSpeechSynthesisTask`) writes straight to S3 itself. A new
`internal/adapters/awspolly` adapter launches two async Polly tasks per narration (audio + word
marks) in parallel, polls both to completion, and computes duration from the marks file's last
timestamp (falling back to the existing word-count estimate if marks are unavailable).
`worker.go` loses its `uploadWithRetry` step entirely — Polly already did the upload.

**Tech Stack:** Go 1.25.0, `github.com/aws/aws-sdk-go-v2/service/polly` (new dependency),
`github.com/aws/smithy-go` (already present, used for AWS error classification),
`golang.org/x/sync/errgroup` (already an indirect dependency, promoted to direct).

**Spec:** `docs/superpowers/specs/2026-08-26-aws-polly-integration-design.md`

**Branch:** Single short-lived feature branch off `master` (per `CLAUDE.md`'s monorepo convention —
no more per-subsystem worktrees), e.g. `feature/aws-polly-tts`, PR into `master` when done. All file
paths below are relative to `backend/` (the Go module root).

## Global Constraints

- Go 1.25.0, module `rioaudioguide/backend`.
- `internal/domain/` is untouched by this plan — confirmed not needed: `GeneratedAudio.timestampsURL`
  and `application.CompleteAudioGeneration`'s `timestampsURL` parameter already exist and already
  accept a real value; only `ports.TTSGenerator` and its call sites change.
- `internal/ports/` changes are explicitly authorized for this plan only (ports-authorship rule in
  `CLAUDE.md`) — do not treat this as a standing exception for future plans.
- Polly `LanguageCode` mapping is fixed: `fr`→`fr-FR`, `en`→`en-US`, `es`→`es-ES`, `pt`→`pt-BR`.
  `VoiceId` stays a caller-supplied parameter, never hardcoded in the adapter.
- S3 bucket: reuse `rio-audio-guide` (same `S3_BUCKET` env var already used for the existing
  `AudioStorage` adapter).
- ElevenLabs adapter (`internal/adapters/elevenlabs/`) is not modified by this plan. It stops
  compiling against `ports.TTSGenerator` once Task 1 lands (expected — nothing references it through
  that interface after Task 6 removes its wiring from `cmd/worker/main.go`).

---

### Task 1: Redesign `ports.TTSGenerator`

**Files:**
- Modify: `internal/ports/tts_generator.go`
- Test: `internal/ports/tts_generator_test.go` (existing tests only cover `PermanentError`, unaffected
  — no change needed there, just re-run them to confirm the package still compiles)

**Interfaces:**
- Produces: `ports.TTSGenerator.Generate(ctx, text, language, voiceID string) (storageURL,
  timestampsURL string, duration time.Duration, err error)` — the new contract every later task
  builds against.

- [ ] **Step 1: Edit the interface**

```go
// internal/ports/tts_generator.go
package ports

import (
	"context"
	"fmt"
	"time"
)

// TTSGenerator is the outbound port to a text-to-speech provider — implemented
// by internal/adapters/awspolly. Generate is expected to complete the ENTIRE
// synthesis (including any provider-side async polling) and return final,
// already-uploaded storage locations -- not raw bytes. Amazon Polly's
// long-text API writes directly to S3 itself; there is no upload step left
// for the worker to perform.
type TTSGenerator interface {
	Generate(ctx context.Context, text, language, voiceID string) (storageURL, timestampsURL string, duration time.Duration, err error)
}

// PermanentError indicates the TTS provider rejected the request in a way
// retrying the same message won't fix (bad API key, invalid text/voice_id,
// unsupported language, task failed for a non-transient reason). The
// RabbitMQ worker uses this to stop requeueing instead of looping forever on
// an unrecoverable message.
type PermanentError struct {
	StatusCode int
	Body       string
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("permanent error (status %d): %s", e.StatusCode, e.Body)
}
```

- [ ] **Step 2: Confirm the port package still compiles and its existing tests pass**

Run: `go test ./internal/ports/...`
Expected: PASS (both `TestPermanentError_*` tests, unaffected by the `Generate` signature change).

- [ ] **Step 3: Commit**

```bash
git add internal/ports/tts_generator.go
git commit -m "ports: TTSGenerator returns final storage URLs instead of raw bytes"
```

---

### Task 2: `awspolly` — language mapping

**Files:**
- Create: `internal/adapters/awspolly/language.go`
- Test: `internal/adapters/awspolly/language_test.go`

**Interfaces:**
- Produces: `languageCode(language string) (string, error)` — used by Task 3.

- [ ] **Step 1: Write the failing test**

```go
// internal/adapters/awspolly/language_test.go
package awspolly

import "testing"

func TestLanguageCode(t *testing.T) {
	tests := []struct {
		language string
		want     string
		wantErr  bool
	}{
		{language: "fr", want: "fr-FR"},
		{language: "en", want: "en-US"},
		{language: "es", want: "es-ES"},
		{language: "pt", want: "pt-BR"}, // Rio/Brazil, not pt-PT
		{language: "de", wantErr: true},
		{language: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			got, err := languageCode(tt.language)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("languageCode(%q): expected an error, got %q", tt.language, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("languageCode(%q): unexpected error: %v", tt.language, err)
			}
			if got != tt.want {
				t.Fatalf("languageCode(%q) = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/awspolly/... -run TestLanguageCode -v`
Expected: FAIL — package `awspolly` doesn't exist yet / `languageCode` undefined.

- [ ] **Step 3: Write the implementation**

```go
// internal/adapters/awspolly/language.go
package awspolly

import "fmt"

// languageCodes mappe le code de langue interne ("fr"/"en"/"es"/"pt", le même
// que celui déjà porté par ttsJobMessage côté rabbitmq) vers le LanguageCode
// Polly. pt-BR et pas pt-PT : ce guide couvre Rio de Janeiro, pas le Portugal.
var languageCodes = map[string]string{
	"fr": "fr-FR",
	"en": "en-US",
	"es": "es-ES",
	"pt": "pt-BR",
}

func languageCode(language string) (string, error) {
	code, ok := languageCodes[language]
	if !ok {
		return "", fmt.Errorf("awspolly: unsupported language %q", language)
	}
	return code, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/awspolly/... -run TestLanguageCode -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/awspolly/language.go internal/adapters/awspolly/language_test.go
git commit -m "awspolly: language-to-LanguageCode mapping"
```

---

### Task 3: `awspolly` — start + poll + classify errors

**Files:**
- Create: `internal/adapters/awspolly/generator.go`
- Test: `internal/adapters/awspolly/generator_test.go`

**Interfaces:**
- Consumes: `languageCode` (Task 2), `ports.PermanentError` (Task 1).
- Produces: `awspolly.Generator` struct, `NewGenerator(pollyClient pollyAPI, s3Client
  s3GetObjectAPI, bucket string) *Generator`, `(*Generator).runTask(...)` (used internally by
  `Generate`, built in Task 4). This task ends with `runTask` fully working and tested; `Generate`
  itself (which calls it twice in parallel) is assembled in Task 4 alongside duration calculation, so
  both halves land together with a working end-to-end method.

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapters/awspolly/generator_test.go
package awspolly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/smithy-go"

	"rioaudioguide/backend/internal/ports"
)

// fakePollyAPI simule StartSpeechSynthesisTask/GetSpeechSynthesisTask sans
// toucher le réseau. statuses est consommée dans l'ordre à chaque appel de
// GetSpeechSynthesisTask -- permet de simuler N tours de polling avant
// complétion.
type fakePollyAPI struct {
	startErr   error
	outputURI  string
	statuses   []types.TaskStatus
	statusIdx  int
	failReason string
}

func (f *fakePollyAPI) StartSpeechSynthesisTask(_ context.Context, _ *polly.StartSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.StartSpeechSynthesisTaskOutput, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	taskID := "task-1"
	return &polly.StartSpeechSynthesisTaskOutput{
		SynthesisTask: &types.SynthesisTask{TaskId: &taskID, TaskStatus: types.TaskStatusScheduled},
	}, nil
}

func (f *fakePollyAPI) GetSpeechSynthesisTask(_ context.Context, _ *polly.GetSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.GetSpeechSynthesisTaskOutput, error) {
	status := f.statuses[f.statusIdx]
	if f.statusIdx < len(f.statuses)-1 {
		f.statusIdx++
	}
	task := &types.SynthesisTask{TaskStatus: status}
	if status == types.TaskStatusCompleted {
		uri := f.outputURI
		task.OutputUri = &uri
	}
	if status == types.TaskStatusFailed {
		reason := f.failReason
		task.TaskStatusReason = &reason
	}
	return &polly.GetSpeechSynthesisTaskOutput{SynthesisTask: task}, nil
}

type fakeAPIError struct{ code, message string }

func (e fakeAPIError) Error() string                 { return e.code + ": " + e.message }
func (e fakeAPIError) ErrorCode() string              { return e.code }
func (e fakeAPIError) ErrorMessage() string           { return e.message }
func (e fakeAPIError) ErrorFault() smithy.ErrorFault  { return smithy.FaultClient }

func TestGenerator_RunTask_PollsUntilCompleted(t *testing.T) {
	fake := &fakePollyAPI{
		outputURI: "https://s3.us-east-1.amazonaws.com/rio-audio-guide/audio/voice-1/task-1.mp3",
		statuses:  []types.TaskStatus{types.TaskStatusScheduled, types.TaskStatusInProgress, types.TaskStatusCompleted},
	}
	gen := &Generator{polly: fake, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

	key, err := gen.runTask(context.Background(), "Bonjour", "fr-FR", "voice-1", types.OutputFormatMp3, nil, "audio/voice-1/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "audio/voice-1/task-1.mp3" {
		t.Fatalf("got key %q, want %q", key, "audio/voice-1/task-1.mp3")
	}
}

func TestGenerator_RunTask_TaskFailedIsPermanent(t *testing.T) {
	fake := &fakePollyAPI{
		statuses:   []types.TaskStatus{types.TaskStatusFailed},
		failReason: "invalid voice for language",
	}
	gen := &Generator{polly: fake, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

	_, err := gen.runTask(context.Background(), "Bonjour", "fr-FR", "voice-1", types.OutputFormatMp3, nil, "audio/voice-1/")
	var permErr *ports.PermanentError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected a *ports.PermanentError, got %v", err)
	}
}

func TestGenerator_RunTask_StartErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		startErr    error
		wantPermErr bool
	}{
		{name: "LanguageNotSupportedException is permanent", startErr: fakeAPIError{code: "LanguageNotSupportedException", message: "nope"}, wantPermErr: true},
		{name: "TextLengthExceededException is permanent", startErr: fakeAPIError{code: "TextLengthExceededException", message: "too long"}, wantPermErr: true},
		{name: "ThrottlingException is transient", startErr: fakeAPIError{code: "ThrottlingException", message: "slow down"}, wantPermErr: false},
		{name: "plain network error is transient", startErr: errors.New("connection reset"), wantPermErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakePollyAPI{startErr: tt.startErr}
			gen := &Generator{polly: fake, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

			_, err := gen.runTask(context.Background(), "Bonjour", "fr-FR", "voice-1", types.OutputFormatMp3, nil, "audio/voice-1/")
			var permErr *ports.PermanentError
			isPerm := errors.As(err, &permErr)
			if isPerm != tt.wantPermErr {
				t.Fatalf("got permanent=%v (err %v), want permanent=%v", isPerm, err, tt.wantPermErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/awspolly/... -run TestGenerator_RunTask -v`
Expected: FAIL — `Generator`, `pollyAPI`, `NewGenerator`, `runTask` undefined.

- [ ] **Step 3: Add the dependency and write the implementation**

Run: `go get github.com/aws/aws-sdk-go-v2/service/polly && go mod tidy`

```go
// internal/adapters/awspolly/generator.go
package awspolly

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"rioaudioguide/backend/internal/ports"
)

// pollyAPI n'expose que les deux méthodes utilisées ici -- *polly.Client les
// satisfait déjà structurellement. Permet de tester avec un faux client Go
// plutôt que de simuler la signature de requêtes du SDK v2 (impossible
// proprement via httptest, contrairement à l'appel REST direct d'ElevenLabs).
type pollyAPI interface {
	StartSpeechSynthesisTask(ctx context.Context, params *polly.StartSpeechSynthesisTaskInput, optFns ...func(*polly.Options)) (*polly.StartSpeechSynthesisTaskOutput, error)
	GetSpeechSynthesisTask(ctx context.Context, params *polly.GetSpeechSynthesisTaskInput, optFns ...func(*polly.Options)) (*polly.GetSpeechSynthesisTaskOutput, error)
}

// s3GetObjectAPI : seule méthode nécessaire pour relire le fichier de marks
// une fois sa tâche terminée (calcul de durée, Task 4). *s3.Client la
// satisfait déjà.
type s3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type Generator struct {
	polly        pollyAPI
	s3           s3GetObjectAPI
	bucket       string
	pollInterval time.Duration
}

func NewGenerator(pollyClient pollyAPI, s3Client s3GetObjectAPI, bucket string) *Generator {
	return &Generator{polly: pollyClient, s3: s3Client, bucket: bucket, pollInterval: 2 * time.Second}
}

// runTask lance une StartSpeechSynthesisTask et attend sa complétion en
// polling GetSpeechSynthesisTask. Renvoie la clé S3 (pas l'URL complète --
// Generate, Task 4, assemble le "s3://bucket/clé" final) du fichier produit.
func (g *Generator) runTask(ctx context.Context, text, languageCode, voiceID string, format types.OutputFormat, marks []types.SpeechMarkType, keyPrefix string) (string, error) {
	out, err := g.polly.StartSpeechSynthesisTask(ctx, &polly.StartSpeechSynthesisTaskInput{
		Engine:             types.EngineNeural,
		LanguageCode:       types.LanguageCode(languageCode),
		OutputFormat:       format,
		OutputS3BucketName: &g.bucket,
		OutputS3KeyPrefix:  &keyPrefix,
		SpeechMarkTypes:    marks,
		Text:               &text,
		TextType:           types.TextTypeText,
		VoiceId:            types.VoiceId(voiceID),
	})
	if err != nil {
		return "", classifyStartError(err)
	}

	taskID := out.SynthesisTask.TaskId
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(g.pollInterval):
		}

		got, err := g.polly.GetSpeechSynthesisTask(ctx, &polly.GetSpeechSynthesisTaskInput{TaskId: taskID})
		if err != nil {
			return "", fmt.Errorf("awspolly: poll task %s: %w", *taskID, err)
		}

		switch got.SynthesisTask.TaskStatus {
		case types.TaskStatusCompleted:
			return keyFromOutputURI(*got.SynthesisTask.OutputUri, g.bucket)
		case types.TaskStatusFailed:
			reason := "unknown reason"
			if got.SynthesisTask.TaskStatusReason != nil {
				reason = *got.SynthesisTask.TaskStatusReason
			}
			// Une tâche Failed ne se corrige pas en la relançant à l'identique
			// -- même texte/voix/langue -- d'où PermanentError plutôt qu'une
			// erreur simple qui déclencherait un retry côté worker.go.
			// StatusCode à 0 : Polly ne donne pas de code HTTP pour un échec
			// de tâche asynchrone (contrairement au 4xx synchrone
			// d'ElevenLabs), seulement cette raison textuelle.
			return "", &ports.PermanentError{StatusCode: 0, Body: "awspolly: task failed: " + reason}
		}
		// scheduled/inProgress : reboucle.
	}
}

// classifyStartError distingue les erreurs Polly non récupérables (mauvaise
// config, requête rejetée -- retenter à l'identique ne changerait rien) des
// erreurs transitoires (throttling, réseau) que le retry générique de
// worker.go doit gérer. Liste tirée de la doc API StartSpeechSynthesisTask
// (docs.aws.amazon.com/polly/latest/APIReference/API_StartSpeechSynthesisTask.html#API_StartSpeechSynthesisTask_Errors) --
// toutes ces exceptions signalent une requête mal formée, jamais un problème
// transitoire.
func classifyStartError(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "EngineNotSupportedException", "InvalidS3BucketException", "InvalidS3KeyException",
			"InvalidSampleRateException", "InvalidSnsTopicArnException", "InvalidSsmlException",
			"LanguageNotSupportedException", "LexiconNotFoundException",
			"MarksNotSupportedForFormatException", "SsmlMarksNotSupportedForTextTypeException",
			"TextLengthExceededException":
			return &ports.PermanentError{StatusCode: 0, Body: "awspolly: " + apiErr.ErrorMessage()}
		}
	}
	return fmt.Errorf("awspolly: start task: %w", err)
}

// keyFromOutputURI extrait la clé S3 de l'OutputUri renvoyé par Polly (URL
// HTTPS), en path-style (https://s3.<region>.amazonaws.com/<bucket>/<clé>)
// ou virtual-hosted-style (https://<bucket>.s3.<region>.amazonaws.com/<clé>)
// -- AWS ne garantit pas lequel des deux formats est utilisé, donc les deux
// sont gérés plutôt que de deviner le nom de fichier nous-mêmes.
func keyFromOutputURI(outputURI, bucket string) (string, error) {
	u, err := url.Parse(outputURI)
	if err != nil {
		return "", fmt.Errorf("awspolly: parse OutputUri %q: %w", outputURI, err)
	}
	if prefix := "/" + bucket + "/"; strings.HasPrefix(u.Path, prefix) {
		return strings.TrimPrefix(u.Path, prefix), nil
	}
	if strings.HasPrefix(u.Host, bucket+".") {
		return strings.TrimPrefix(u.Path, "/"), nil
	}
	return "", fmt.Errorf("awspolly: OutputUri %q does not reference bucket %q", outputURI, bucket)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/awspolly/... -run TestGenerator_RunTask -v`
Expected: PASS (all three tests)

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/adapters/awspolly/generator.go internal/adapters/awspolly/generator_test.go
git commit -m "awspolly: start/poll a synthesis task, classify permanent vs transient errors"
```

---

### Task 4: `awspolly` — `Generate`: parallel tasks + marks-based duration

**Files:**
- Modify: `internal/adapters/awspolly/generator.go`
- Test: `internal/adapters/awspolly/generator_test.go`

**Interfaces:**
- Consumes: `runTask`, `languageCode` (Tasks 2-3).
- Produces: `(*Generator).Generate(ctx, text, language, voiceID string) (storageURL,
  timestampsURL string, duration time.Duration, err error)` — satisfies `ports.TTSGenerator`,
  consumed by `worker.go` in Task 5.

- [ ] **Step 1: Write the failing tests**

```go
// append to internal/adapters/awspolly/generator_test.go

// fakePollyAPIByFormat route Start/GetSpeechSynthesisTask par OutputFormat --
// Generate lance deux tâches en parallèle (mp3 + json), il faut pouvoir leur
// faire renvoyer des OutputUri différents pour vérifier que storageURL et
// timestampsURL ne sont pas confondus.
type fakePollyAPIByFormat struct {
	mp3URI, jsonURI string
}

func (f *fakePollyAPIByFormat) StartSpeechSynthesisTask(_ context.Context, in *polly.StartSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.StartSpeechSynthesisTaskOutput, error) {
	taskID := string(in.OutputFormat)
	return &polly.StartSpeechSynthesisTaskOutput{
		SynthesisTask: &types.SynthesisTask{TaskId: &taskID, TaskStatus: types.TaskStatusScheduled},
	}, nil
}

func (f *fakePollyAPIByFormat) GetSpeechSynthesisTask(_ context.Context, in *polly.GetSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.GetSpeechSynthesisTaskOutput, error) {
	uri := f.mp3URI
	if *in.TaskId == string(types.OutputFormatJson) {
		uri = f.jsonURI
	}
	return &polly.GetSpeechSynthesisTaskOutput{
		SynthesisTask: &types.SynthesisTask{TaskStatus: types.TaskStatusCompleted, OutputUri: &uri},
	}, nil
}

// fakeS3GetObject renvoie un corps NDJSON fixe pour n'importe quelle clé.
type fakeS3GetObject struct{ body string }

func (f *fakeS3GetObject) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(f.body))}, nil
}

func TestGenerator_Generate_ReturnsDistinctAudioAndTimestampsURLs(t *testing.T) {
	polly := &fakePollyAPIByFormat{
		mp3URI:  "https://s3.us-east-1.amazonaws.com/rio-audio-guide/audio/voice-1/a.mp3",
		jsonURI: "https://s3.us-east-1.amazonaws.com/rio-audio-guide/timestamps/voice-1/a.json",
	}
	marks := `{"time":0,"type":"word","value":"Bonjour"}
{"time":820,"type":"word","value":"le"}
{"time":1150,"type":"word","value":"monde"}
`
	s3fake := &fakeS3GetObject{body: marks}
	gen := &Generator{polly: polly, s3: s3fake, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

	storageURL, timestampsURL, duration, err := gen.Generate(context.Background(), "Bonjour le monde", "fr", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storageURL != "s3://rio-audio-guide/audio/voice-1/a.mp3" {
		t.Fatalf("got storageURL %q", storageURL)
	}
	if timestampsURL != "s3://rio-audio-guide/timestamps/voice-1/a.json" {
		t.Fatalf("got timestampsURL %q", timestampsURL)
	}
	if duration != 1150*time.Millisecond {
		t.Fatalf("got duration %v, want 1150ms (last word mark)", duration)
	}
}

func TestGenerator_Generate_FallsBackToWordCountEstimateWhenMarksUnreadable(t *testing.T) {
	polly := &fakePollyAPIByFormat{
		mp3URI:  "https://s3.us-east-1.amazonaws.com/rio-audio-guide/audio/voice-1/a.mp3",
		jsonURI: "https://s3.us-east-1.amazonaws.com/rio-audio-guide/timestamps/voice-1/a.json",
	}
	gen := &Generator{
		polly: polly, bucket: "rio-audio-guide", pollInterval: time.Millisecond,
		s3: &erroringS3GetObject{},
	}

	_, _, duration, err := gen.Generate(context.Background(), "Oi!", "pt", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if duration <= 0 {
		t.Fatalf("got duration %v, want > 0 (fallback estimate)", duration)
	}
}

type erroringS3GetObject struct{}

func (erroringS3GetObject) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return nil, errors.New("s3: object not found")
}

func TestGenerator_Generate_UnsupportedLanguageFailsBeforeAnyPollyCall(t *testing.T) {
	polly := &fakePollyAPIByFormat{}
	gen := &Generator{polly: polly, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

	_, _, _, err := gen.Generate(context.Background(), "Hallo", "de", "voice-1")
	if err == nil {
		t.Fatal("expected an error for an unsupported language")
	}
}
```

Add `"io"` to the test file's imports alongside the existing ones.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/awspolly/... -run TestGenerator_Generate -v`
Expected: FAIL — `Generate`, `durationFromMarks` undefined.

- [ ] **Step 3: Write the implementation**

Run: `go get golang.org/x/sync/errgroup && go mod tidy` (already an indirect dependency, this
promotes it to direct).

```go
// append to internal/adapters/awspolly/generator.go, plus new imports:
// "bufio", "encoding/json", "golang.org/x/sync/errgroup"

func (g *Generator) Generate(ctx context.Context, text, language, voiceID string) (string, string, time.Duration, error) {
	code, err := languageCode(language)
	if err != nil {
		return "", "", 0, err
	}

	var audioKey, marksKey string
	group, gctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		key, err := g.runTask(gctx, text, code, voiceID, types.OutputFormatMp3, nil, "audio/"+voiceID+"/")
		audioKey = key
		return err
	})
	group.Go(func() error {
		key, err := g.runTask(gctx, text, code, voiceID, types.OutputFormatJson, []types.SpeechMarkType{types.SpeechMarkTypeWord}, "timestamps/"+voiceID+"/")
		marksKey = key
		return err
	})
	if err := group.Wait(); err != nil {
		return "", "", 0, err
	}

	duration := g.durationFromMarks(ctx, marksKey, text)
	return "s3://" + g.bucket + "/" + audioKey, "s3://" + g.bucket + "/" + marksKey, duration, nil
}

type wordMark struct {
	Time int64  `json:"time"`
	Type string `json:"type"`
}

// durationFromMarks lit le fichier NDJSON de marks juste écrit par Polly et
// prend le "time" (ms) du dernier mot -- plus précis que l'estimation par
// nombre de mots utilisée par le stub d'origine et par ElevenLabs. Ne
// remonte jamais d'erreur : un fichier de marks illisible ne doit pas faire
// échouer toute la génération, juste dégrader la précision de la durée.
func (g *Generator) durationFromMarks(ctx context.Context, marksKey, fallbackText string) time.Duration {
	out, err := g.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &g.bucket, Key: &marksKey})
	if err != nil {
		return estimateDuration(fallbackText)
	}
	defer func() { _ = out.Body.Close() }()

	var lastTime int64
	scanner := bufio.NewScanner(out.Body)
	for scanner.Scan() {
		var mark wordMark
		if err := json.Unmarshal(scanner.Bytes(), &mark); err != nil {
			continue
		}
		if mark.Type == "word" && mark.Time > lastTime {
			lastTime = mark.Time
		}
	}
	if lastTime <= 0 {
		return estimateDuration(fallbackText)
	}
	return time.Duration(lastTime) * time.Millisecond
}

// estimateDuration : même formule que l'estimation ElevenLabs/stub -- ~5
// caractères par mot, ~400ms par mot, plancher à 1 mot pour éviter une durée
// nulle que domain.NewGeneratedAudio rejette.
func estimateDuration(text string) time.Duration {
	wordCount := max(1, len(text)/5)
	return time.Duration(wordCount) * 400 * time.Millisecond
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/awspolly/... -v`
Expected: PASS (every test in the package, Tasks 2-4 combined)

- [ ] **Step 5: Confirm `*Generator` satisfies `ports.TTSGenerator`**

```go
// append to internal/adapters/awspolly/generator_test.go
var _ ports.TTSGenerator = (*Generator)(nil)
```

Run: `go build ./internal/adapters/awspolly/...`
Expected: builds cleanly — a compile error here means the signature drifted from Task 1.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/awspolly/generator.go internal/adapters/awspolly/generator_test.go
git commit -m "awspolly: Generate launches audio+marks tasks in parallel, derives duration from marks"
```

---

### Task 5: `worker.go` — drop the upload step, adapt to the new signature

**Files:**
- Modify: `internal/adapters/rabbitmq/worker.go`
- Modify: `internal/adapters/rabbitmq/worker_test.go`

**Interfaces:**
- Consumes: `ports.TTSGenerator` (new shape, Task 1).
- Produces: nothing new — `Worker.handle` behavior changes internally only.

- [ ] **Step 1: Update `worker.go`**

Replace the TTS-call-through-upload block (current lines 127–192: the `Generate` call, its error
handling, and the entire `uploadWithRetry` section) with:

```go
	storageURL, timestampsURL, duration, err := w.ttsGenerator.Generate(ctx, job.Text, job.Language, job.VoiceID)
	if err != nil {
		var permErr *ports.PermanentError
		if errors.As(err, &permErr) {
			log.Printf("tts worker: permanent TTS error for %s, marking failed: %v", job.AudioFileID, err)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			// Ack, pas Nack : réessayer le même message ne changera rien à une
			// clé invalide, un texte/voice_id rejeté, ou une tâche Polly qui a
			// définitivement échoué.
			ackOrLog(msg, job.AudioFileID)
			return
		}
		log.Printf("tts worker: transient TTS error for %s (attempt %d/%d): %v",
			job.AudioFileID, job.Attempt+1, maxTTSAttempts, err)

		if job.Attempt+1 >= maxTTSAttempts {
			log.Printf("tts worker: giving up on %s after %d attempts, marking failed", job.AudioFileID, maxTTSAttempts)
			if failErr := application.FailAudioGeneration(ctx, w.audioFileRepo, job.AudioFileID, err.Error()); failErr != nil {
				log.Printf("tts worker: mark failed also failed for %s: %v", job.AudioFileID, failErr)
			}
			ackOrLog(msg, job.AudioFileID)
			return
		}

		time.Sleep(requeueDelay)
		job.Attempt++
		if pubErr := w.requeueWithAttempt(ctx, job); pubErr != nil {
			log.Printf("tts worker: failed to republish retry for %s, falling back to plain requeue: %v", job.AudioFileID, pubErr)
			nackOrLog(msg, true, job.AudioFileID)
			return
		}
		ackOrLog(msg, job.AudioFileID)
		return
	}

	// Polly a déjà écrit l'audio ET les timestamps sur S3 lui-même -- storageURL
	// et timestampsURL sont déjà finaux, plus rien à uploader ici (contrairement
	// à l'ancienne intégration ElevenLabs, qui rendait des bytes que ce worker
	// uploadait via w.storage).
	if err := application.CompleteAudioGeneration(ctx, w.scriptRepo, w.audioFileRepo, job.AudioFileID, storageURL, timestampsURL, duration); err != nil {
		log.Printf("tts worker: complete generation failed for %s: %v", job.AudioFileID, err)
		time.Sleep(requeueDelay)
		nackOrLog(msg, true, job.AudioFileID)
		return
	}

	ackOrLog(msg, job.AudioFileID)
}
```

Then delete the now-unused `uploadWithRetry` function and `uploadWithRetryAttempts` constant
entirely (they had no other callers). `w.storage` (`ports.AudioStorage`) stays a `Worker` field and
`NewWorker` parameter unchanged — it's still a real dependency used elsewhere in the codebase (audio
serving/presigning), just no longer called from `handle`.

- [ ] **Step 2: Update `worker_test.go` fakes to the new 4-return-value signature**

Every existing fake's `Generate` method changes from `([]byte, time.Duration, error)` to `(string,
string, time.Duration, error)`. Apply this mechanical rewrite to each:

```go
// fakeTTSGenerator
func (fakeTTSGenerator) Generate(_ context.Context, text, _, _ string) (string, string, time.Duration, error) {
	return "s3://rio-audio-guide/" + text + ".mp3", "s3://rio-audio-guide/" + text + ".json", time.Second, nil
}

// onceFailingTTSGenerator
func (g *onceFailingTTSGenerator) Generate(_ context.Context, text, _, _ string) (string, string, time.Duration, error) {
	g.calls++
	if g.calls == 1 {
		return "", "", 0, errors.New("transient failure")
	}
	return "s3://rio-audio-guide/" + text + ".mp3", "s3://rio-audio-guide/" + text + ".json", time.Second, nil
}

// alwaysFailingTTSGenerator
func (g *alwaysFailingTTSGenerator) Generate(_ context.Context, _, _, _ string) (string, string, time.Duration, error) {
	g.calls++
	return "", "", 0, errors.New("persistent failure")
}

// failingTTSGenerator
func (f failingTTSGenerator) Generate(_ context.Context, _, _, _ string) (string, string, time.Duration, error) {
	return "", "", 0, f.err
}

// countingTTSGenerator
func (g *countingTTSGenerator) Generate(_ context.Context, text, _, _ string) (string, string, time.Duration, error) {
	g.calls++
	return "s3://rio-audio-guide/" + text + ".mp3", "s3://rio-audio-guide/" + text + ".json", time.Second, nil
}
```

Preserve each fake's existing field names (`calls` etc.) and every test that references them by name
— only the `Generate` method body and signature change. Read the current file first to match field
names exactly, since the plan above shows representative bodies, not a verbatim diff.

Then **delete** the upload-retry-without-refacturing test (the one built around `countingTTSGenerator`
+ `failingStorage`, roughly lines 432–560 in the current file) — the behavior it verified
(`worker.go` retrying an S3 upload locally, independent of the TTS call, to avoid re-billing an
expensive provider) no longer exists after Step 1's rewrite. Removing dead test coverage for a
removed code path is correct here, not a shortcut — keeping it would mean either deleting its
assertions piecemeal (leaving a confusing half-test) or reintroducing the upload step just to keep it
green.

- [ ] **Step 3: Run the full rabbitmq package test suite**

`worker_test.go` and `audio_job_publisher_test.go` both carry `//go:build integration` (pre-existing,
not introduced by this plan) — they need the `integration` build tag AND a live RabbitMQ broker on
`amqp://guest:guest@localhost:5672/` to actually run; without the tag, `go test
./internal/adapters/rabbitmq/...` silently reports "no test files" and verifies nothing in this
package.

Run first (always, catches compile errors even without a broker):
`go build -tags=integration ./internal/adapters/rabbitmq/...`
Expected: builds cleanly — this alone catches signature mismatches in the rewritten fakes even if no
broker is reachable.

Then, if a RabbitMQ broker is reachable (`docker compose up -d rabbitmq` from `backend/`, wait for
`docker compose ps` to show it healthy):
Run: `go test -tags=integration ./internal/adapters/rabbitmq/... -v`
Expected: PASS — every remaining test (message handling, permanent/transient classification, max
attempts, requeue-with-attempt) still holds with the new fakes; only the deleted test is gone.

If no broker is reachable in this environment, report DONE_WITH_CONCERNS rather than DONE, stating
plainly that `-tags=integration` tests were not executed for lack of a broker, and that
`go build -tags=integration` is the only verification these changes received. Do not report DONE on
build-only verification of this task.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/rabbitmq/worker.go internal/adapters/rabbitmq/worker_test.go
git commit -m "worker: consume TTSGenerator's final URLs directly, drop the now-dead upload-retry step"
```

---

### Task 6: Wire Polly into `cmd/worker/main.go`, disconnect ElevenLabs

**Files:**
- Modify: `cmd/worker/main.go`

**Interfaces:**
- Consumes: `awspolly.NewGenerator` (Task 4), `ports.TTSGenerator` (Task 1).

- [ ] **Step 1: Edit the composition root**

Replace:
```go
	"rioaudioguide/backend/internal/adapters/elevenlabs"
```
with:
```go
	"github.com/aws/aws-sdk-go-v2/service/polly"

	"rioaudioguide/backend/internal/adapters/awspolly"
```

(`awss3` import and `s3Client` construction stay as-is — still used by `s3.NewAudioStorage` for
everything unrelated to TTS: audio presigning, etc.)

Replace:
```go
	s3Client := awss3.NewFromConfig(awsCfg)
	storage := s3.NewAudioStorage(s3Client, envOr("S3_BUCKET", "rio-audio-guide"))

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	ttsGenerator := elevenlabs.NewGenerator(mustEnv("ELEVENLABS_API_KEY"))
```
with:
```go
	s3Client := awss3.NewFromConfig(awsCfg)
	bucket := envOr("S3_BUCKET", "rio-audio-guide")
	storage := s3.NewAudioStorage(s3Client, bucket)

	scriptRepo := postgres.NewScriptRepository(pool)
	audioFileRepo := postgres.NewAudioFileRepository(pool)
	pollyClient := polly.NewFromConfig(awsCfg)
	ttsGenerator := awspolly.NewGenerator(pollyClient, s3Client, bucket)
```

`mustEnv("ELEVENLABS_API_KEY")` is gone — the binary no longer requires a key it doesn't use. If
`mustEnv` has no other caller left in this file after this change, leave the function defined
(it's a small, generically useful helper, and `envOr` right below it stays in active use) rather than
deleting it speculatively — confirm with `grep -n "mustEnv" cmd/worker/main.go` before deciding either
way; only remove it if truly unused.

- [ ] **Step 2: Build the whole module**

Run: `go build ./...`
Expected: builds cleanly. `internal/adapters/elevenlabs` still compiles on its own (nothing in its
package changed) — it simply has no more callers anywhere in `cmd/`.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS across every package, including the untouched `internal/adapters/elevenlabs` tests
(still valid — that package's own behavior hasn't changed, only its callers). This run does NOT
include the `integration`-tagged tests (`rabbitmq`, `postgres`, `redis` packages all carry some —
pre-existing, matches `.github/workflows/backend-ci.yml`'s own `test` job, which also runs plain `go
test ./...` with no services and no tag). If a full local stack (`docker compose up -d`) is available,
also run `go test -tags=integration ./...` for full confidence; otherwise note in the report that
integration coverage wasn't exercised, same caveat as Task 5.

- [ ] **Step 4: Commit**

```bash
git add cmd/worker/main.go
git commit -m "worker: switch composition root from ElevenLabs to Amazon Polly"
```

---

## After this plan

Not covered here, left for the founder to decide when testing for real (per the spec's "Hors
scope" section):
- Picking actual `VoiceId` values per language (`Lea`/`Remi` for fr-FR, `Sergio`/`Lupe` for
  es-ES/es-US, `Vitoria`/`Camila`/`Thiago` for pt-BR, an en-US voice) — passed at call time via
  `Script`/generation-request data, nothing to hardcode.
- Granting the worker's runtime IAM role/user `polly:StartSpeechSynthesisTask` and
  `polly:GetSpeechSynthesisTask` (plus existing S3 write access on `rio-audio-guide`) — AWS-side,
  outside this codebase.
- Whether to strip `ELEVENLABS_API_KEY` out of the Helm secret entirely, or leave it inert.
