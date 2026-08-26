// internal/adapters/awspolly/generator_test.go
package awspolly

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/polly"
	"github.com/aws/aws-sdk-go-v2/service/polly/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
func (e fakeAPIError) ErrorCode() string             { return e.code }
func (e fakeAPIError) ErrorMessage() string          { return e.message }
func (e fakeAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

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

var _ ports.TTSGenerator = (*Generator)(nil)
