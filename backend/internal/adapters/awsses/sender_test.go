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

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "123456"); err != nil {
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

func TestSender_SendVerificationCode_IncludesCodeInBody(t *testing.T) {
	fake := &fakeSESAPI{}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := fake.lastInput.Content
	if content == nil || content.Simple == nil || content.Simple.Body == nil || content.Simple.Body.Text == nil || content.Simple.Body.Text.Data == nil {
		t.Fatalf("expected a plain-text body, got %+v", content)
	}
	body := *content.Simple.Body.Text.Data
	if !strings.Contains(body, "654321") {
		t.Fatalf("body %q does not contain the code %q", body, "654321")
	}
}

func TestSender_SendVerificationCode_PropagatesSESError(t *testing.T) {
	fake := &fakeSESAPI{err: errors.New("ses rejected the request")}
	sender := NewSender(fake, "noreply@example.com")

	if err := sender.SendVerificationCode(context.Background(), "user@example.com", "111111"); err == nil {
		t.Fatal("expected the SES error to propagate")
	}
}
