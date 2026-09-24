package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"rioaudioguide/backend/internal/application"
	"rioaudioguide/backend/internal/domain"
)

type fakeHTTPUserRepo struct {
	users        map[string]*domain.User
	byEmail      map[string]string
	codes        map[string]string
	codeExpiries map[string]time.Time
}

func newFakeHTTPUserRepo() *fakeHTTPUserRepo {
	return &fakeHTTPUserRepo{
		users:        map[string]*domain.User{},
		byEmail:      map[string]string{},
		codes:        map[string]string{},
		codeExpiries: map[string]time.Time{},
	}
}

func (f *fakeHTTPUserRepo) Save(_ context.Context, u *domain.User) error {
	f.users[u.ID()] = u
	f.byEmail[u.Email().String()] = u.ID()
	return nil
}

func (f *fakeHTTPUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (f *fakeHTTPUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.byEmail[email]
	if !ok {
		return nil, errors.New("not found")
	}
	return f.users[id], nil
}

func (f *fakeHTTPUserRepo) SaveVerificationCode(_ context.Context, userID, code string, expiresAt time.Time) error {
	f.codes[userID] = code
	f.codeExpiries[userID] = expiresAt
	return nil
}

func (f *fakeHTTPUserRepo) FindVerificationCode(_ context.Context, userID string) (string, time.Time, error) {
	code, ok := f.codes[userID]
	if !ok {
		return "", time.Time{}, errors.New("not found")
	}
	return code, f.codeExpiries[userID], nil
}

type fakeHTTPEmailSender struct {
	lastCode string
	err      error
}

func (f *fakeHTTPEmailSender) SendVerificationCode(_ context.Context, _, code string) error {
	f.lastCode = code
	if f.err != nil {
		return f.err
	}
	return nil
}

func newTestServerForUserHandlers(userRepo *fakeHTTPUserRepo, emailSender *fakeHTTPEmailSender) *Server {
	return NewServer(nil, nil, nil, userRepo, nil, nil, nil, nil, fakeTokenIssuerForHTTP{}, nil, nil, emailSender)
}

// Must satisfy the full ports.TokenIssuer interface (Issue AND Verify) to
// type-check as NewServer's tokens parameter -- Verify is stubbed since
// none of this file's tests exercise a route that calls it.
type fakeTokenIssuerForHTTP struct{}

func (fakeTokenIssuerForHTTP) Issue(userID string, role domain.Role) (string, error) {
	return "fake-token", nil
}

func (fakeTokenIssuerForHTTP) Verify(token string) (string, domain.Role, error) {
	return "", "", errors.New("not implemented in fake")
}

func TestRegisterHandler_Returns201EvenWhenVerificationEmailFailsToSend(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{err: errors.New("ses rejected the request")}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "sendfails@example.com", "password": "password123"})
	req := httptest.NewRequest("POST", "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 201 {
		t.Fatalf("got status %d, want 201 even though the verification email failed to send, body: %s", rec.Code, rec.Body.String())
	}
	if _, err := userRepo.FindByEmail(context.Background(), "sendfails@example.com"); err != nil {
		t.Fatalf("expected the account to still be persisted: %v", err)
	}
}

func TestVerifyEmailHandler_CorrectCodeReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "handler@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "handler@example.com", "code": emailSender.lastCode})
	req := httptest.NewRequest("POST", "/verify-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestVerifyEmailHandler_WrongCodeReturns422(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "handler2@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "handler2@example.com", "code": "000000"})
	req := httptest.NewRequest("POST", "/verify-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 422 {
		t.Fatalf("got status %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestResendVerificationCodeHandler_AlwaysReturns200(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "nobody@example.com"})
	req := httptest.NewRequest("POST", "/resend-verification-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200 even for an unknown email, body: %s", rec.Code, rec.Body.String())
	}
}

// This is the anti-enumeration property that matters most: without it, an
// unauthenticated caller could tell "no such account" (200, nil error) apart
// from "account exists, send failed" (would-be 500) -- exactly the outcome
// SES sandbox mode produces for every address that isn't pre-verified.
func TestResendVerificationCodeHandler_Returns200EvenWhenSendFails(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	registerSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, registerSender, "resend-fails@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	emailSender := &fakeHTTPEmailSender{err: errors.New("ses rejected the request")}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "resend-fails@example.com"})
	req := httptest.NewRequest("POST", "/resend-verification-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("got status %d, want 200 even though the resend's email send failed, body: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginHandler_ReturnsForbiddenWhenNotVerified(t *testing.T) {
	userRepo := newFakeHTTPUserRepo()
	emailSender := &fakeHTTPEmailSender{}
	if _, err := application.RegisterUser(context.Background(), userRepo, emailSender, "unverified-http@example.com", "password123", domain.RoleUser); err != nil {
		t.Fatalf("register: %v", err)
	}
	server := newTestServerForUserHandlers(userRepo, emailSender)

	body, _ := json.Marshal(map[string]string{"email": "unverified-http@example.com", "password": "password123"})
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.echo.ServeHTTP(rec, req)

	if rec.Code != 403 {
		t.Fatalf("got status %d, want 403, body: %s", rec.Code, rec.Body.String())
	}
}
