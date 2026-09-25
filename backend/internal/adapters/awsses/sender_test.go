package awsses

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

type fakeSESAPI struct {
	lastInput *sesv2.SendEmailInput
	err       error
}

func (f *fakeSESAPI) SendEmail(_ context.Context, params *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	f.lastInput = params
	if f.err != nil {
		return nil, f.err
	}
	return &sesv2.SendEmailOutput{}, nil
}

func TestSender_SendVerificationCode_SendsFromAndToCorrectAddresses(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "123456", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput == nil {
		t.Fatal("expected SendEmail to have been called")
	}
	if fake.lastInput.FromEmailAddress == nil || *fake.lastInput.FromEmailAddress != "noreply@example.com" {
		t.Fatalf("got FromEmailAddress %v, want noreply@example.com", fake.lastInput.FromEmailAddress)
	}
	if fake.lastInput.Destination == nil || len(fake.lastInput.Destination.ToAddresses) != 1 || fake.lastInput.Destination.ToAddresses[0] != "user@example.com" {
		t.Fatalf("got Destination %+v, want ToAddresses=[user@example.com]", fake.lastInput.Destination)
	}
}

func TestSender_SendVerificationCode_IncludesCodeInHTMLAndTextBody(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "654321", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := fake.lastInput.Content
	if content == nil || content.Simple == nil || content.Simple.Body == nil {
		t.Fatalf("expected a body, got %+v", content)
	}
	if content.Simple.Body.Html == nil || content.Simple.Body.Html.Data == nil || !strings.Contains(*content.Simple.Body.Html.Data, "654321") {
		t.Fatal("expected the HTML body to contain the code")
	}
	if content.Simple.Body.Text == nil || content.Simple.Body.Text.Data == nil || !strings.Contains(*content.Simple.Body.Text.Data, "654321") {
		t.Fatal("expected the plain-text fallback body to contain the code")
	}
}

func TestSender_SendVerificationCode_UsesRequestedLanguage(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111", "fr"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput.Content.Simple.Subject == nil || *fake.lastInput.Content.Simple.Subject.Data != verificationCopy["fr"].subject {
		t.Fatalf("got subject %v, want the French subject %q", fake.lastInput.Content.Simple.Subject, verificationCopy["fr"].subject)
	}
}

func TestSender_SendVerificationCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111", "en"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}

func TestSender_SendPasswordResetCode_SendsWithResetCopy(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendPasswordResetCode(context.Background(), "user@example.com", "987654", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastInput.Content.Simple.Subject == nil || *fake.lastInput.Content.Simple.Subject.Data != resetCopy["en"].subject {
		t.Fatalf("got subject %v, want the reset-email subject %q", fake.lastInput.Content.Simple.Subject, resetCopy["en"].subject)
	}
	if !strings.Contains(*fake.lastInput.Content.Simple.Body.Text.Data, "987654") {
		t.Fatal("expected the code in the reset email's text body")
	}
}

func TestSender_SendPasswordResetCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendPasswordResetCode(context.Background(), "user@example.com", "987654", "en"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}
