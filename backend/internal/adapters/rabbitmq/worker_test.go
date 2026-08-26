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
	"rioaudioguide/backend/internal/ports"
)

type fakeScriptRepo struct {
	mu      sync.Mutex
	scripts map[string]*domain.Script
}

func newFakeScriptRepo() *fakeScriptRepo {
	return &fakeScriptRepo{scripts: map[string]*domain.Script{}}
}

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

func (f *fakeScriptRepo) FindByPlaceIDAndLanguage(_ context.Context, _, _ string) (*domain.Script, error) {
	return nil, errors.New("not implemented in fake")
}

func (f *fakeScriptRepo) FindByPlaceID(_ context.Context, placeID string) ([]*domain.Script, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []*domain.Script
	for _, s := range f.scripts {
		if s.PlaceID() == placeID {
			found = append(found, s)
		}
	}
	return found, nil
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

func (f *fakeAudioFileRepo) FindByScriptID(_ context.Context, _ string) (*domain.AudioFile, error) {
	return nil, errors.New("not implemented in fake")
}

type fakeStorage struct{}

func (fakeStorage) Upload(_ context.Context, key string, _ []byte, _ string) (string, error) {
	return "fake://bucket/" + key, nil
}

func (fakeStorage) PresignURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("not implemented in fake")
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

	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{}, fakeTTSGenerator{})
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

type fakeTTSGenerator struct{}

func (fakeTTSGenerator) Generate(_ context.Context, text, _, _ string) (string, string, time.Duration, error) {
	return "fake://bucket/" + text + ".mp3", "fake://bucket/" + text + ".json", 5 * time.Second, nil
}

// onceFailingTTSGenerator échoue transitoirement au PREMIER appel puis réussit
// — exactement le scénario qui déclenchait la boucle infinie : le message est
// requeué, redelivré, et StartAudioGeneration retrouve l'AudioFile déjà
// "generating".
type onceFailingTTSGenerator struct {
	mu     sync.Mutex
	failed bool
}

func (g *onceFailingTTSGenerator) Generate(_ context.Context, text, _, _ string) (string, string, time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.failed {
		g.failed = true
		return "", "", 0, errors.New("simulated transient failure")
	}
	return "fake://bucket/" + text + ".mp3", "fake://bucket/" + text + ".json", 5 * time.Second, nil
}

func TestWorker_TransientTTSError_RetriesOnRedeliveryAndSucceeds(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte à réessayer")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	if err := script.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{}, &onceFailingTTSGenerator{})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Le chemin transitoire dort 2s avant chaque Nack — la fenêtre doit couvrir
	// l'échec + le délai + la redelivery réussie.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() { _ = worker.Run(ctx) }()

	body, _ := json.Marshal(ttsJobMessage{
		AudioFileID: audioFile.ID(),
		ScriptID:    script.ID(),
		Text:        "Texte à réessayer",
		Language:    "fr",
		VoiceID:     "voice-1",
	})
	if err := channel.PublishWithContext(context.Background(), "", TTSJobQueue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	}); err != nil {
		t.Fatalf("publish test job: %v", err)
	}

	deadline := time.After(12 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			found, findErr := audioFileRepo.FindByID(context.Background(), audioFile.ID())
			if findErr == nil {
				t.Fatalf("timed out waiting for retry to succeed, audio file stuck in status %v", found.Status())
			}
			t.Fatal("timed out waiting for retry to succeed")
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

// alwaysFailingTTSGenerator never recovers -- proves maxTTSAttempts is a
// real ceiling, not just documentation: before this fix, a persistently
// slow/unreachable TTS call retried forever (Nack(requeue=true) with no
// counter), silently re-billing ElevenLabs on every redelivery.
type alwaysFailingTTSGenerator struct {
	mu    sync.Mutex
	calls int
}

func (g *alwaysFailingTTSGenerator) Generate(_ context.Context, _, _, _ string) (string, string, time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return "", "", 0, errors.New("simulated persistent transient failure")
}

func (g *alwaysFailingTTSGenerator) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func TestWorker_TransientTTSError_GivesUpAfterMaxAttempts(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	generator := &alwaysFailingTTSGenerator{}
	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, fakeStorage{}, generator)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// maxTTSAttempts-1 = 2 requeueDelay (2s) waits between attempts, plus
	// processing overhead -- 10s gives comfortable margin.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	deadline := time.After(9 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			found, _ := audioFileRepo.FindByID(context.Background(), audioFile.ID())
			t.Fatalf("timed out waiting for the job to give up, audio file stuck in status %v (calls=%d)",
				found.Status(), generator.callCount())
		case <-tick.C:
			found, err := audioFileRepo.FindByID(context.Background(), audioFile.ID())
			if err != nil || found.Status() != domain.AudioFileStatusFailed {
				continue
			}
			if found.FailureReason() == "" {
				t.Fatal("got empty failure reason on a failed audio file")
			}
			if generator.callCount() != maxTTSAttempts {
				t.Fatalf("got %d Generate calls, want exactly %d (maxTTSAttempts) -- no more, no fewer",
					generator.callCount(), maxTTSAttempts)
			}
			return
		}
	}
}

type failingTTSGenerator struct{ err error }

func (f failingTTSGenerator) Generate(_ context.Context, _, _, _ string) (string, string, time.Duration, error) {
	return "", "", 0, f.err
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
