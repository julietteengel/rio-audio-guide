package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewGeneratedAudio(t *testing.T) {
	tests := []struct {
		name          string
		storageURL    string
		timestampsURL string
		duration      time.Duration
		wantErr       error
	}{
		{name: "valid", storageURL: "s3://bucket/a.mp3", timestampsURL: "s3://bucket/a.json", duration: 42 * time.Second, wantErr: nil},
		{name: "empty storage URL", storageURL: "", timestampsURL: "s3://bucket/a.json", duration: 42 * time.Second, wantErr: ErrAudioFileStorageURLRequired},
		{name: "non-positive duration", storageURL: "s3://bucket/a.mp3", timestampsURL: "s3://bucket/a.json", duration: 0, wantErr: ErrAudioFileInvalidDuration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewGeneratedAudio(tt.storageURL, tt.timestampsURL, tt.duration)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g.StorageURL() != tt.storageURL || g.Duration() != tt.duration {
				t.Fatalf("got %+v, want storageURL=%q duration=%v", g, tt.storageURL, tt.duration)
			}
		})
	}
}

func TestNewAudioFile(t *testing.T) {
	a, err := NewAudioFile("script-1", "voice-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status() != AudioFileStatusQueued {
		t.Fatalf("got status %v, want queued", a.Status())
	}

	if _, err := NewAudioFile("", "voice-1"); !errors.Is(err, ErrAudioFileScriptIDRequired) {
		t.Fatalf("got error %v, want ErrAudioFileScriptIDRequired", err)
	}
}

func TestAudioFile_FullLifecycle(t *testing.T) {
	a, _ := NewAudioFile("script-1", "voice-1")
	audio, err := NewGeneratedAudio("s3://bucket/audio.mp3", "s3://bucket/audio.json", 42*time.Second)
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}

	if err := a.MarkReady(audio); !errors.Is(err, ErrAudioFileNotGenerating) {
		t.Fatalf("got error %v, want ErrAudioFileNotGenerating (still queued)", err)
	}

	if err := a.MarkGenerating(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := a.MarkGenerating(); !errors.Is(err, ErrAudioFileNotQueued) {
		t.Fatalf("got error %v, want ErrAudioFileNotQueued (already generating)", err)
	}

	if err := a.MarkReady(audio); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status() != AudioFileStatusReady || a.Audio().StorageURL() == "" {
		t.Fatalf("mark ready did not apply: status=%v audio=%+v", a.Status(), a.Audio())
	}
}

func TestAudioFile_FailAndRetry(t *testing.T) {
	a, _ := NewAudioFile("script-1", "voice-1")
	_ = a.MarkGenerating()

	if err := a.MarkFailed("TTS quota exceeded"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status() != AudioFileStatusFailed || a.FailureReason() != "TTS quota exceeded" {
		t.Fatalf("mark failed did not apply: status=%v reason=%v", a.Status(), a.FailureReason())
	}

	if err := a.Retry(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Status() != AudioFileStatusQueued || a.FailureReason() != "" {
		t.Fatalf("retry did not reset state: status=%v reason=%v", a.Status(), a.FailureReason())
	}

	if err := a.Retry(); !errors.Is(err, ErrAudioFileNotFailed) {
		t.Fatalf("got error %v, want ErrAudioFileNotFailed (not failed)", err)
	}
}

func TestReconstructAudioFile(t *testing.T) {
	audio, _ := NewGeneratedAudio("s3://bucket/a.mp3", "s3://bucket/a.json", 10*time.Second)
	a := ReconstructAudioFile("existing-id-789", "script-1", "voice-1", AudioFileStatusReady, audio, "")
	if a.ID() != "existing-id-789" {
		t.Fatalf("got ID %q, want %q", a.ID(), "existing-id-789")
	}
	if a.Status() != AudioFileStatusReady {
		t.Fatalf("got status %v, want ready", a.Status())
	}
}
