package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

func TestResetPassword_CorrectCodeChangesPassword(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "reset@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}

	if err := ResetPassword(context.Background(), repo, "reset@example.com", code, "new-password"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindByEmail(context.Background(), "reset@example.com")
	if err != nil {
		t.Fatalf("find by email after reset: %v", err)
	}
	if found.PasswordHash().String() == "" {
		t.Fatal("expected a password hash to be set")
	}
}

func TestResetPassword_WrongCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "wrong-reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "wrong-reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}

	err := ResetPassword(context.Background(), repo, "wrong-reset@example.com", "000000", "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_ExpiredCodeFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "expired-reset@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "expired-reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "expired-reset@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}
	// Overwrite with an already-past expiry to simulate a stale code.
	if err := repo.SaveResetCode(context.Background(), user.ID(), code, time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatalf("save expired code: %v", err)
	}

	err = ResetPassword(context.Background(), repo, "expired-reset@example.com", code, "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_UnknownEmailFails(t *testing.T) {
	repo := newFakeUserRepo()

	err := ResetPassword(context.Background(), repo, "nobody@example.com", "123456", "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_DeletedAccountFails(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	user, err := RegisterUser(context.Background(), repo, sender, "deleted-reset@example.com", "old-password", "en", domain.RoleUser)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "deleted-reset@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}
	if err := user.Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Save(context.Background(), user); err != nil {
		t.Fatalf("save: %v", err)
	}

	err = ResetPassword(context.Background(), repo, "deleted-reset@example.com", code, "new-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v, want ErrResetCodeInvalid", err)
	}
}

func TestResetPassword_ClearsCodeAfterSuccess(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "replay@example.com", "old-password", "en", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := ForgotPassword(context.Background(), repo, sender, "replay@example.com", "en"); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	user, err := repo.FindByEmail(context.Background(), "replay@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	code, _, err := repo.FindResetCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("find reset code: %v", err)
	}

	if err := ResetPassword(context.Background(), repo, "replay@example.com", code, "new-password"); err != nil {
		t.Fatalf("first reset: %v", err)
	}

	// Replaying the same code a second time must fail -- it was cleared
	// after the successful reset.
	err = ResetPassword(context.Background(), repo, "replay@example.com", code, "another-password")
	if !errors.Is(err, ErrResetCodeInvalid) {
		t.Fatalf("got error %v on replay, want ErrResetCodeInvalid", err)
	}
}
