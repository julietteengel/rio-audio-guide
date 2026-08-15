package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewLanguage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{name: "valid FR", input: "fr", wantErr: nil},
		{name: "unsupported", input: "de", wantErr: ErrScriptInvalidLanguage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewLanguage(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if l.String() != tt.input {
				t.Fatalf("got %q, want %q", l.String(), tt.input)
			}
		})
	}
}

func TestNewScriptText(t *testing.T) {
	if _, err := NewScriptText(""); !errors.Is(err, ErrScriptTextRequired) {
		t.Fatalf("got error %v, want ErrScriptTextRequired", err)
	}
	txt, err := NewScriptText("Le Cristo Redentor...")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if txt.String() != "Le Cristo Redentor..." {
		t.Fatalf("got %q, want %q", txt.String(), "Le Cristo Redentor...")
	}
}

func validScriptFixture(t *testing.T) *Script {
	t.Helper()
	lang, err := NewLanguage("fr")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	text, err := NewScriptText("Le Cristo Redentor...")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	return NewScript("place-1", lang, text, "source text")
}

func TestNewScript(t *testing.T) {
	s := validScriptFixture(t)
	if s.Status() != ScriptStatusDraft {
		t.Fatalf("got status %v, want draft", s.Status())
	}
	if s.ID() == "" {
		t.Fatal("expected a generated ID, got empty string")
	}
}

func TestScript_MarkReviewedThenPublish(t *testing.T) {
	s := validScriptFixture(t)

	if err := s.Publish(); !errors.Is(err, ErrScriptNotReviewed) {
		t.Fatalf("got error %v, want ErrScriptNotReviewed (can't publish a draft)", err)
	}

	if err := s.MarkReviewed("julie"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status() != ScriptStatusReviewed || s.Reviewer() != "julie" {
		t.Fatalf("review did not apply: status=%v reviewer=%v", s.Status(), s.Reviewer())
	}

	if err := s.MarkReviewed("julie"); !errors.Is(err, ErrScriptNotDraft) {
		t.Fatalf("got error %v, want ErrScriptNotDraft (already reviewed)", err)
	}

	if err := s.Publish(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status() != ScriptStatusPublished {
		t.Fatalf("got status %v, want published", s.Status())
	}
	if s.PublishedAt().IsZero() {
		t.Fatal("expected PublishedAt to be set")
	}
}

func TestReconstructScript(t *testing.T) {
	lang, _ := NewLanguage("en")
	text, _ := NewScriptText("The Christ the Redeemer...")
	now := time.Now()

	s := ReconstructScript("existing-id-456", "place-1", lang, text, "source", ScriptStatusPublished, "julie", now, now)
	if s.ID() != "existing-id-456" {
		t.Fatalf("got ID %q, want %q", s.ID(), "existing-id-456")
	}
	if s.Status() != ScriptStatusPublished {
		t.Fatalf("got status %v, want published", s.Status())
	}
}
