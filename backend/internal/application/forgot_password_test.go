package application

import (
	"context"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

func TestForgotPassword_GeneratesAndSendsResetCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "forgot@example.com", "password123", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := ForgotPassword(context.Background(), repo, sender, "forgot@example.com", "pt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.resetSentTo) != 1 || sender.resetSentTo[0] != "forgot@example.com" {
		t.Fatalf("expected exactly one reset email sent to forgot@example.com, got %v", sender.resetSentTo)
	}

	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("expected a stored reset code: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("got code %q, want exactly 6 digits", code)
	}
}

func TestForgotPassword_UnknownEmailReturnsNilSilently(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	if err := ForgotPassword(context.Background(), repo, sender, "nobody@example.com", "en"); err != nil {
		t.Fatalf("expected nil error for an unknown email (anti-enumeration), got %v", err)
	}
	if len(sender.resetSentTo) != 0 {
		t.Fatal("expected no reset email sent for an unknown address")
	}
}

func TestForgotPassword_DeletedAccountReturnsNilSilently(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "deleted@example.com", "password123", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := user.Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Save(context.Background(), user); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := ForgotPassword(context.Background(), repo, sender, "deleted@example.com", "en"); err != nil {
		t.Fatalf("expected nil error for a deleted account (anti-enumeration), got %v", err)
	}
	if len(sender.resetSentTo) != 0 {
		t.Fatal("expected no reset email sent for a deleted account")
	}
}

func TestForgotPassword_DoesNotTouchVerificationCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "separate-codes@example.com", "password123", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	verificationCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}

	if err := ForgotPassword(context.Background(), repo, sender, "separate-codes@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	stillThere, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find verification code after forgot-password: %v", err)
	}
	if stillThere != verificationCode {
		t.Fatalf("got verification code %q after a password-reset request, want it untouched (%q) -- the two flows must use separate storage", stillThere, verificationCode)
	}
}
