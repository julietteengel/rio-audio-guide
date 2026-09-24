//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
)

func TestUserRepository_SaveAndFindByID(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, err := domain.NewEmail("julie+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	passwordHash, err := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	if err != nil {
		t.Fatalf("unexpected error building fixture: %v", err)
	}
	user := domain.NewUser(email, passwordHash, domain.RoleUser)

	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Email() != email || found.Role() != domain.RoleUser || found.Status() != domain.UserStatusActive {
		t.Fatalf("got %+v, want a matching active RoleUser account", found)
	}
}

func TestUserRepository_FindByEmail(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("find-by-email+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleAdmin)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := repo.FindByEmail(ctx, email.String())
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}
	if found.ID() != user.ID() || found.Role() != domain.RoleAdmin {
		t.Fatalf("got %+v, want the same account with RoleAdmin", found)
	}

	if _, err := repo.FindByEmail(ctx, "nobody-"+fmt.Sprintf("%d", time.Now().UnixNano())+"@example.com"); err == nil {
		t.Fatal("expected an error for an email with no matching account")
	} else if err != pgx.ErrNoRows {
		t.Fatalf("got error %v, want pgx.ErrNoRows", err)
	}
}

// TestUserRepository_EmailMustBeUnique verifie la contrainte UNIQUE de
// schema.sql au niveau du repository, pas juste au niveau SQL brut -- deux
// comptes avec le même email ne doivent jamais coexister, c'est ce qui
// permet à FindByEmail (utilisé au login) de ne jamais être ambigu.
func TestUserRepository_EmailMustBeUnique(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("duplicate+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")

	first := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("save first user: %v", err)
	}

	second := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, second); err == nil {
		t.Fatal("expected a unique-constraint error saving a second user with the same email")
	}
}

func TestUserRepository_DeleteIsPersisted(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("delete-me+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := user.Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save after delete: %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Status() != domain.UserStatusDeleted {
		t.Fatalf("got status %v, want deleted", found.Status())
	}
}

func TestUserRepository_SaveAndFindVerificationCode(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verify-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	expiresAt := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	if err := repo.SaveVerificationCode(ctx, user.ID(), "123456", expiresAt); err != nil {
		t.Fatalf("save verification code: %v", err)
	}

	code, gotExpiresAt, err := repo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if code != "123456" {
		t.Fatalf("got code %q, want %q", code, "123456")
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("got expiry %v, want %v", gotExpiresAt, expiresAt)
	}
}

func TestUserRepository_SaveVerificationCode_OverwritesPrevious(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verify-overwrite+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if err := repo.SaveVerificationCode(ctx, user.ID(), "111111", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save first code: %v", err)
	}
	if err := repo.SaveVerificationCode(ctx, user.ID(), "222222", time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("save second code: %v", err)
	}

	code, _, err := repo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		t.Fatalf("find verification code: %v", err)
	}
	if code != "222222" {
		t.Fatalf("got code %q, want the overwritten %q, not the first one", code, "222222")
	}
}

func TestUserRepository_FindVerificationCode_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("no-code+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	if _, _, err := repo.FindVerificationCode(ctx, user.ID()); err != pgx.ErrNoRows {
		t.Fatalf("got error %v, want pgx.ErrNoRows for a user with no stored code", err)
	}
}

func TestUserRepository_EmailVerifiedRoundTrips(t *testing.T) {
	pool := testPool(t)
	repo := NewUserRepository(pool)
	ctx := context.Background()

	email, _ := domain.NewEmail("verified-roundtrip+" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com")
	passwordHash, _ := domain.NewPasswordHash("$2a$10$fakehashfaketest")
	user := domain.NewUser(email, passwordHash, domain.RoleUser)
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	if err := user.MarkEmailVerified(); err != nil {
		t.Fatalf("mark verified: %v", err)
	}
	if err := repo.Save(ctx, user); err != nil {
		t.Fatalf("save verified user: %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if !found.EmailVerified() {
		t.Fatal("expected EmailVerified() true after reload from Postgres")
	}
}
