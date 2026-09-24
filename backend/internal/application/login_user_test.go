package application

import (
	"context"
	"errors"
	"testing"

	"rioaudioguide/backend/internal/domain"
)

// fakeTokenIssuer must satisfy the full ports.TokenIssuer interface
// (Issue AND Verify) to type-check as the tokens parameter LoginUser
// expects -- Verify is stubbed since these tests never call it.
type fakeTokenIssuer struct{}

func (fakeTokenIssuer) Issue(userID string, role domain.Role) (string, error) {
	return "fake-token-" + userID, nil
}

func (fakeTokenIssuer) Verify(token string) (string, domain.Role, error) {
	return "", "", errors.New("not implemented in fake")
}

func TestLoginUser_RejectsUnverifiedAccount(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "unverified@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, err := LoginUser(context.Background(), repo, fakeTokenIssuer{}, "unverified@example.com", "password123")
	if !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("got error %v, want ErrEmailNotVerified", err)
	}
}

func TestLoginUser_SucceedsOnceVerified(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}
	if _, err := RegisterUser(context.Background(), repo, sender, "will-verify@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	code := sender.sentCode[0]
	if err := VerifyEmail(context.Background(), repo, "will-verify@example.com", code); err != nil {
		t.Fatalf("verify: %v", err)
	}

	token, err := LoginUser(context.Background(), repo, fakeTokenIssuer{}, "will-verify@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}
}
