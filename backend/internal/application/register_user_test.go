package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type fakeUserRepo struct {
	users        map[string]*domain.User
	byEmail      map[string]string // email -> userID
	codes        map[string]string
	codeExpiries map[string]time.Time
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		users:        map[string]*domain.User{},
		byEmail:      map[string]string{},
		codes:        map[string]string{},
		codeExpiries: map[string]time.Time{},
	}
}

func (f *fakeUserRepo) Save(_ context.Context, u *domain.User) error {
	f.users[u.ID()] = u
	f.byEmail[u.Email().String()] = u.ID()
	return nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (f *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.byEmail[email]
	if !ok {
		return nil, errors.New("user not found")
	}
	return f.users[id], nil
}

func (f *fakeUserRepo) SaveVerificationCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	f.codes[userID] = code
	f.codeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeUserRepo) FindVerificationCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.codes[userID]
	if !ok {
		return "", time.Time{}, errors.New("no code found")
	}
	return code, f.codeExpiries[userID], nil
}

type fakeEmailSender struct {
	sentTo   []string
	sentCode []string
}

func (f *fakeEmailSender) SendVerificationCode(_ context.Context, toEmail, code string) error {
	f.sentTo = append(f.sentTo, toEmail)
	f.sentCode = append(f.sentCode, code)
	return nil
}

type erroringEmailSender struct{}

func (erroringEmailSender) SendVerificationCode(context.Context, string, string) error {
	return errors.New("ses is down")
}

func TestRegisterUser_GeneratesAndSendsVerificationCode(t *testing.T) {
	repo := newFakeUserRepo()
	sender := &fakeEmailSender{}

	user, err := RegisterUser(context.Background(), repo, sender, "new@example.com", "password123", domain.RoleUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.EmailVerified() {
		t.Fatal("a freshly registered user must not be verified yet")
	}

	if len(sender.sentTo) != 1 || sender.sentTo[0] != "new@example.com" {
		t.Fatalf("expected exactly one email sent to new@example.com, got %v", sender.sentTo)
	}
	code := sender.sentCode[0]
	if len(code) != 6 {
		t.Fatalf("got code %q, want exactly 6 digits", code)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Fatalf("got code %q, want only digits", code)
		}
	}

	storedCode, _, err := repo.FindVerificationCode(context.Background(), user.ID())
	if err != nil {
		t.Fatalf("expected a stored code: %v", err)
	}
	if storedCode != code {
		t.Fatalf("stored code %q does not match sent code %q", storedCode, code)
	}
}

func TestRegisterUser_ReturnsErrorWhenEmailSendFails(t *testing.T) {
	repo := newFakeUserRepo()

	_, err := RegisterUser(context.Background(), repo, erroringEmailSender{}, "fail@example.com", "password123", domain.RoleUser)
	if err == nil {
		t.Fatal("expected an error when the email send fails")
	}
	// The account and its code must still exist -- registration isn't
	// rolled back over a transient email-provider failure, the client can
	// use resend.
	found, findErr := repo.FindByEmail(context.Background(), "fail@example.com")
	if findErr != nil {
		t.Fatalf("expected the user to still be persisted despite the send failure: %v", findErr)
	}
	if _, _, err := repo.FindVerificationCode(context.Background(), found.ID()); err != nil {
		t.Fatalf("expected a stored code despite the send failure: %v", err)
	}
}
