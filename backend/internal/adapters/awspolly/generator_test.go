// internal/adapters/awspolly/generator_test.go
package awspolly

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
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

// midStreamErrorReader returns data first, then a read error -- simulates a
// connection reset partway through streaming the marks object from S3, as
// opposed to the object simply not existing (erroringS3GetObject above).
type midStreamErrorReader struct {
	data []byte
	err  error
	pos  int
}

func (r *midStreamErrorReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

type midStreamErrorS3GetObject struct{ body string }

func (f *midStreamErrorS3GetObject) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	reader := &midStreamErrorReader{data: []byte(f.body), err: errors.New("read tcp: connection reset by peer")}
	return &s3.GetObjectOutput{Body: io.NopCloser(reader)}, nil
}

func TestGenerator_Generate_FallsBackToWordCountEstimateWhenMarksStreamErrorsMidRead(t *testing.T) {
	polly := &fakePollyAPIByFormat{
		mp3URI:  "https://s3.us-east-1.amazonaws.com/rio-audio-guide/audio/voice-1/a.mp3",
		jsonURI: "https://s3.us-east-1.amazonaws.com/rio-audio-guide/timestamps/voice-1/a.json",
	}
	// A real mark (820ms) precedes the read error -- without checking
	// scanner.Err(), durationFromMarks would silently return 820ms as if it
	// were the true (last) mark, instead of falling back.
	marks := `{"time":0,"type":"word","value":"Bonjour"}
{"time":820,"type":"word","value":"le"}
`
	text := "Bonjour le monde"
	gen := &Generator{
		polly: polly, bucket: "rio-audio-guide", pollInterval: time.Millisecond,
		s3: &midStreamErrorS3GetObject{body: marks},
	}

	_, _, duration, err := gen.Generate(context.Background(), text, "fr", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := estimateDuration(text); duration != want {
		t.Fatalf("got duration %v, want %v (word-count estimate, not the truncated 820ms mark)", duration, want)
	}
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

func TestWrapSSML_EscapesXMLSpecialCharsAndAppliesRate(t *testing.T) {
	got := wrapSSML(`Tom & Jerry <said> "hi"`, "fr")
	want := `<speak><prosody rate="90%">Tom &amp; Jerry &lt;said&gt; "hi"</prosody></speak>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapSSML_PortugueseUsesItsOwnRate(t *testing.T) {
	got := wrapSSML("Olá", "pt")
	want := `<speak><prosody rate="100%">Olá</prosody></speak>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapSSML_UnlistedLanguageFallsBackToDefaultRate(t *testing.T) {
	got := wrapSSML("Hello", "en")
	want := `<speak><prosody rate="90%">Hello</prosody></speak>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGenerator_Generate_SendsSSMLNotPlainText(t *testing.T) {
	polly := &capturingPollyAPI{
		mp3URI:  "https://s3.us-east-1.amazonaws.com/rio-audio-guide/audio/voice-1/a.mp3",
		jsonURI: "https://s3.us-east-1.amazonaws.com/rio-audio-guide/timestamps/voice-1/a.json",
	}
	gen := &Generator{polly: polly, s3: &fakeS3GetObject{body: ""}, bucket: "rio-audio-guide", pollInterval: time.Millisecond}

	if _, _, _, err := gen.Generate(context.Background(), "Bonjour", "fr", "voice-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Les deux tâches (mp3 + marks) reçoivent le même texte SSML -- on
	// vérifie les deux, pas juste la première arrivée.
	polly.mu.Lock()
	defer polly.mu.Unlock()
	if len(polly.startedWith) != 2 {
		t.Fatalf("got %d StartSpeechSynthesisTask calls, want 2", len(polly.startedWith))
	}
	for _, in := range polly.startedWith {
		if in.TextType != types.TextTypeSsml {
			t.Fatalf("got TextType %q, want %q", in.TextType, types.TextTypeSsml)
		}
		if !strings.Contains(*in.Text, `<prosody rate="90%">Bonjour</prosody>`) {
			t.Fatalf("got Text %q, want it wrapped in the configured prosody rate", *in.Text)
		}
	}
}

// capturingPollyAPI se comporte comme fakePollyAPIByFormat (complétion
// immédiate, distinguée par OutputFormat) mais capture aussi chaque requête
// StartSpeechSynthesisTask reçue -- pour vérifier ce qui est réellement
// envoyé à Polly (TextType, Text), pas juste ce que Generate renvoie. Les
// deux tâches (mp3 + marks) sont lancées en parallèle par errgroup, donc
// l'écriture doit être protégée par un mutex plutôt que d'assigner
// directement à des variables partagées (data race sous -race sinon).
type capturingPollyAPI struct {
	mu              sync.Mutex
	startedWith     []*polly.StartSpeechSynthesisTaskInput
	mp3URI, jsonURI string
}

func (f *capturingPollyAPI) StartSpeechSynthesisTask(_ context.Context, in *polly.StartSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.StartSpeechSynthesisTaskOutput, error) {
	f.mu.Lock()
	f.startedWith = append(f.startedWith, in)
	f.mu.Unlock()
	taskID := string(in.OutputFormat)
	return &polly.StartSpeechSynthesisTaskOutput{
		SynthesisTask: &types.SynthesisTask{TaskId: &taskID, TaskStatus: types.TaskStatusScheduled},
	}, nil
}

func (f *capturingPollyAPI) GetSpeechSynthesisTask(_ context.Context, in *polly.GetSpeechSynthesisTaskInput, _ ...func(*polly.Options)) (*polly.GetSpeechSynthesisTaskOutput, error) {
	uri := f.mp3URI
	if *in.TaskId == string(types.OutputFormatJson) {
		uri = f.jsonURI
	}
	return &polly.GetSpeechSynthesisTaskOutput{
		SynthesisTask: &types.SynthesisTask{TaskStatus: types.TaskStatusCompleted, OutputUri: &uri},
	}, nil
}
