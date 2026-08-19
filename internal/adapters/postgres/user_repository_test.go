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
