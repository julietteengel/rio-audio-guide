package awsses

import (
	"strings"
	"testing"
)

func TestVerificationCopyFor_KnownLanguage(t *testing.T) {
	for _, lang := range []string{"fr", "en", "pt", "es"} {
		c := verificationCopyFor(lang)
		if c.subject == "" || c.heading == "" || c.body == "" {
			t.Fatalf("language %q: expected non-empty subject/heading/body, got %+v", lang, c)
		}
	}
}

func TestVerificationCopyFor_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	got := verificationCopyFor("de")
	want := verificationCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an unrecognized language, want the English copy %+v", got, want)
	}
}

func TestVerificationCopyFor_EmptyLanguageFallsBackToEnglish(t *testing.T) {
	got := verificationCopyFor("")
	want := verificationCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an empty language, want the English copy %+v", got, want)
	}
}

func TestResetCopyFor_KnownLanguage(t *testing.T) {
	for _, lang := range []string{"fr", "en", "pt", "es"} {
		c := resetCopyFor(lang)
		if c.subject == "" || c.heading == "" || c.body == "" {
			t.Fatalf("language %q: expected non-empty subject/heading/body, got %+v", lang, c)
		}
	}
}

func TestResetCopyFor_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	got := resetCopyFor("de")
	want := resetCopyFor("en")
	if got != want {
		t.Fatalf("got %+v for an unrecognized language, want the English copy %+v", got, want)
	}
}

func TestRenderHTML_ContainsHeadingBodyAndCode(t *testing.T) {
	html := renderHTML("My Heading", "My body text.", "123456")
	if !strings.Contains(html, "My Heading") {
		t.Fatal("expected the heading to appear in the rendered HTML")
	}
	if !strings.Contains(html, "My body text.") {
		t.Fatal("expected the body text to appear in the rendered HTML")
	}
	if !strings.Contains(html, "123456") {
		t.Fatal("expected the code to appear in the rendered HTML")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Fatal("expected a full HTML document")
	}
}

func TestRenderText_ContainsBodyAndCode(t *testing.T) {
	text := renderText("My body text.", "123456")
	if !strings.Contains(text, "My body text.") || !strings.Contains(text, "123456") {
		t.Fatalf("got %q, want it to contain both the body text and the code", text)
	}
}
