package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestVerifyEmail_CorrectCodeMarksVerified(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "verify@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]

	if err := VerifyEmail(context.Background(), repo, "verify@example.com", code); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindByID(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if !found.EmailVerified() {
		t.Fatal("expected the account to be verified after a correct code")
	}
}

func TestVerifyEmail_WrongCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "wrong-code@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}

	err := VerifyEmail(context.Background(), repo, "wrong-code@example.com", "000000")
	if !errors.Is(err, ErrVerificationCodeInvalid) {
		t.Fatalf("got error %v, want ErrVerificationCodeInvalid", err)
	}
}

func TestVerifyEmail_ExpiredCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "expired@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]
	// Overwrite with an already-past expiry to simulate a stale code.
	if err := repo.SaveVerificationCode(context.Background(), user.ID(), code, time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("save expired code: %v", err)
	}

	err = VerifyEmail(context.Background(), repo, "expired@example.com", code)
	if !errors.Is(err, ErrVerificationCodeInvalid) {
		t.Fatalf("got error %v, want ErrVerificationCodeInvalid", err)
	}
}

func TestResendVerificationCode_GeneratesADifferentCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "resend@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	firstCode := sender.sentCode[0]

	if err := ResendVerificationCode(context.Background(), repo, sender, "resend@example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sentCode) != 2 {
		t.Fatalf("expected 2 emails sent total, got %d", len(sender.sentCode))
	}
	secondCode := sender.sentCode[1]
	if secondCode == firstCode {
		t.Fatal("expected the resent code to differ from the original (astronomically unlikely to collide by chance for a 6-digit code across two independent draws)")
	}

	storedCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if storedCode != secondCode {
		t.Fatalf("stored code %q should match the resent code %q, not the original", storedCode, secondCode)
	}
}

func TestResendVerificationCode_UnknownEmailReturnsNilSilently(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	if err := ResendVerificationCode(context.Background(), repo, sender, "nobody@example.com"); err != nil {
		t.Fatalf("expected nil error for an unknown email (anti-enumeration), got %v", err)
	}
	if len(sender.sentTo) != 0 {
		t.Fatal("expected no email sent for an unknown address")
	}
}
