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

func (fakeTTSGenerator) Generate(_ context.Context, text, _, _ string) ([]byte, time.Duration, error) {
	return []byte("FAKE-AUDIO:" + text), 5 * time.Second, nil
}

// onceFailingTTSGenerator échoue transitoirement au PREMIER appel puis réussit
// — exactement le scénario qui déclenchait la boucle infinie : le message est
// requeué, redelivré, et StartAudioGeneration retrouve l'AudioFile déjà
// "generating".
type onceFailingTTSGenerator struct {
	mu     sync.Mutex
	failed bool
}

func (g *onceFailingTTSGenerator) Generate(_ context.Context, text, _, _ string) ([]byte, time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.failed {
		g.failed = true
		return nil, 0, errors.New("simulated transient failure")
	}
	return []byte("FAKE-AUDIO:" + text), 5 * time.Second, nil
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

func (g *alwaysFailingTTSGenerator) Generate(_ context.Context, _, _, _ string) ([]byte, time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return nil, 0, errors.New("simulated persistent transient failure")
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

type failingStorage struct{ err error }

func (f failingStorage) Upload(_ context.Context, _ string, _ []byte, _ string) (string, error) {
	return "", f.err
}
func (f failingStorage) PresignURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("not implemented in fake")
}

// countingTTSGenerator compte les appels à Generate -- c'est ce compteur qui
// prouve le fix : si un échec S3 transitoire redéclenchait ElevenLabs, il
// vaudrait 2, pas 1.
type countingTTSGenerator struct {
	mu    sync.Mutex
	calls int
}

func (g *countingTTSGenerator) Generate(_ context.Context, text, _, _ string) ([]byte, time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return []byte("FAKE-AUDIO:" + text), 5 * time.Second, nil
}

func (g *countingTTSGenerator) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

// onceFailingStorage échoue transitoirement au premier appel d'Upload puis
// réussit -- même forme que onceFailingTTSGenerator, côté S3 cette fois.
type onceFailingStorage struct {
	mu     sync.Mutex
	failed bool
}

func (s *onceFailingStorage) Upload(_ context.Context, key string, _ []byte, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return "", errors.New("simulated transient S3 failure")
	}
	return "fake://bucket/" + key, nil
}

func (s *onceFailingStorage) PresignURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("not implemented in fake")
}

// TestWorker_TransientS3Error_RetriesUploadWithoutRecallingTTS reproduit
// exactement le bug corrigé : avant le fix, un échec S3 transitoire faisait
// Nack(requeue=true) sur tout le message, donc à la redelivery, ElevenLabs
// était rappelé une deuxième fois -- payant, et inutile puisque lui n'avait
// pas échoué. Le compteur d'appels ci-dessous doit rester à 1.
func TestWorker_TransientS3Error_RetriesUploadWithoutRecallingTTS(t *testing.T) {
	channel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte dont l'upload S3 va d'abord échouer")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	if err := script.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	ttsGen := &countingTTSGenerator{}
	worker, err := NewWorker(channel, scriptRepo, audioFileRepo, &onceFailingStorage{}, ttsGen)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = worker.Run(ctx) }()

	body, _ := json.Marshal(ttsJobMessage{
		AudioFileID: audioFile.ID(),
		ScriptID:    script.ID(),
		Text:        "Texte dont l'upload S3 va d'abord échouer",
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
				if calls := ttsGen.callCount(); calls != 1 {
					t.Fatalf("ElevenLabs Generate called %d times, want 1 -- a transient S3 failure "+
						"should retry the upload locally, not re-trigger a paid TTS call", calls)
				}
				return
			}
		}
	}
}

func TestWorker_PermanentS3Error_MarksAudioFileFailedAndAcks(t *testing.T) {
	// Deux connexions distinctes : le worker consomme sur la sienne, le test
	// publie et vérifie sur l'autre. C'est ce qui rend l'assertion "queue vidée"
	// ci-dessous possible — il faut pouvoir couper le consumer du worker sans
	// perdre le canal qui sert à interroger la queue.
	channel := testChannel(t)
	workerChannel := testChannel(t)

	scriptRepo := newFakeScriptRepo()
	audioFileRepo := newFakeAudioFileRepo()

	text, _ := domain.NewScriptText("Texte")
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source")
	_ = script.MarkReviewed("julie")
	_ = scriptRepo.Save(context.Background(), script)

	audioFile, _ := domain.NewAudioFile(script.ID(), "voice-1")
	_ = audioFileRepo.Save(context.Background(), audioFile)

	permErr := &ports.PermanentError{StatusCode: 0, Body: "InvalidAccessKeyId"}
	worker, err := NewWorker(workerChannel, scriptRepo, audioFileRepo, failingStorage{err: permErr}, fakeTTSGenerator{})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Après NewWorker (qui déclare la queue) : repart d'une queue vide, sinon un
	// résidu d'un run précédent ferait échouer l'assertion finale pour la
	// mauvaise raison.
	if _, err := channel.QueuePurge(TTSJobQueue, false); err != nil {
		t.Fatalf("purge queue: %v", err)
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
				assertQueueDrained(t, channel, workerChannel)
				return
			}
		}
	}
}

// assertQueueDrained prouve que la delivery a bien été Ack'ée, et pas Nack'ée
// avec requeue — c'est-à-dire exactement le bug (boucle de redelivery infinie)
// que la classification des erreurs S3 permanentes existe pour corriger. Sans
// cette vérification, le test passerait aussi avec un Nack(requeue=true).
//
// Compter les messages pendant que le worker consomme encore ne prouverait
// rien : une delivery non-Ack'ée est "unacked", pas "ready", et n'est donc PAS
// comptée par QueueDeclarePassive — un message coincé en boucle de redelivery
// serait invisible presque tout le temps. Il faut d'abord couper le consumer
// (fermer son canal), ce qui force le broker à rendre à la queue tout ce qui
// n'a pas été Ack'é ; ce qui reste à 0 après ça n'y est vraiment plus.
func assertQueueDrained(t *testing.T, channel, workerChannel *amqp.Channel) {
	t.Helper()

	if err := workerChannel.Close(); err != nil {
		t.Fatalf("close worker channel: %v", err)
	}

	// Le retour en queue par le broker est asynchrone, d'où le polling plutôt
	// qu'une lecture unique : sur un Ack le compteur reste à 0, sur un requeue
	// il monte à 1 et y reste (plus aucun consumer pour le reprendre).
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	last := -1
	for {
		select {
		case <-deadline:
			t.Fatalf("got %d message(s) still in queue, want 0 (the delivery should have been Ack'd, not left for redelivery)", last)
		case <-tick.C:
			q, err := channel.QueueDeclarePassive(TTSJobQueue, true, false, false, false, nil)
			if err != nil {
				t.Fatalf("queue declare passive: %v", err)
			}
			if q.Messages == 0 {
				return
			}
			last = q.Messages
		}
	}
}
