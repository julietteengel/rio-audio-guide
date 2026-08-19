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
	f.scripts[s.ID()] = s
	return nil
}

func (f *fakeScriptRepo) FindByID(_ context.Context, id string) (*domain.Script, error) {
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
	var found []*domain.Script
	for _, s := range f.scripts {
		if s.PlaceID() == placeID {
			found = append(found, s)
		}
	}
	return found, nil
}

type fakeAudioFileRepo struct {
	files map[string]*domain.AudioFile
}

func newFakeAudioFileRepo() *fakeAudioFileRepo {
	return &fakeAudioFileRepo{files: map[string]*domain.AudioFile{}}
}

func (f *fakeAudioFileRepo) Save(_ context.Context, a *domain.AudioFile) error {
	f.files[a.ID()] = a
	return nil
}

func (f *fakeAudioFileRepo) FindByID(_ context.Context, id string) (*domain.AudioFile, error) {
	a, ok := f.files[id]
	if !ok {
		return nil, errors.New("audio file not found")
	}
	return a, nil
}

func (f *fakeAudioFileRepo) FindByScriptID(_ context.Context, _ string) (*domain.AudioFile, error) {
	return nil, errors.New("not implemented in fake")
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

	text, err := domain.NewScriptText("Le Cristo Redentor...")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	script := domain.NewScript("place-1", domain.LanguageFR, text, "source text")
	if err := scriptRepo.Save(ctx, script); err != nil {
		t.Fatalf("unexpected error saving fixture: %v", err)
	}

	if err := ReviewAndRequestAudio(ctx, scriptRepo, audioFileRepo, publisher, script.ID(), "julie", "voice-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, _ := scriptRepo.FindByID(ctx, script.ID())
	if saved.Status() != domain.ScriptStatusReviewed {
		t.Fatalf("got status %v, want reviewed", saved.Status())
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
