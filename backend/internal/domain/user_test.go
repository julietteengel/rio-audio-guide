package domain

import "testing"

func TestUser_NewUserStartsUnverified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)
	if u.EmailVerified() {
		t.Fatal("a newly registered user must start unverified")
	}
}

func TestUser_MarkEmailVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)

	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !u.EmailVerified() {
		t.Fatal("EmailVerified() should be true after MarkEmailVerified()")
	}
}

func TestUser_MarkEmailVerified_IdempotentOnAlreadyVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)

	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := u.MarkEmailVerified(); err != nil {
		t.Fatalf("second call should be a harmless no-op, got error: %v", err)
	}
	if !u.EmailVerified() {
		t.Fatal("still expected to be verified")
	}
}

func TestUser_MarkEmailVerified_RejectsDeletedUser(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")
	u := NewUser(email, hash, RoleUser)
	if err := u.Delete(); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}

	if err := u.MarkEmailVerified(); err != ErrUserDeleted {
		t.Fatalf("got error %v, want ErrUserDeleted", err)
	}
}

func TestReconstructUser_PreservesEmailVerified(t *testing.T) {
	email, _ := NewEmail("test@example.com")
	hash, _ := NewPasswordHash("$2a$10$fakehashfaketest")

	verified := ReconstructUser("id-1", email, hash, RoleUser, UserStatusActive, true)
	if !verified.EmailVerified() {
		t.Fatal("expected EmailVerified() true when reconstructed with true")
	}

	unverified := ReconstructUser("id-2", email, hash, RoleUser, UserStatusActive, false)
	if unverified.EmailVerified() {
		t.Fatal("expected EmailVerified() false when reconstructed with false")
	}
}
