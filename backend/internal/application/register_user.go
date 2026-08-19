package application

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// RegisterUser hashes the plaintext password with bcrypt -- the domain
// PasswordHash Value Object only validates "non-empty", it has no idea how
// a hash is produced. The plaintext password never leaves this function:
// it's hashed immediately and discarded.
//
// Email uniqueness isn't pre-checked with a FindByEmail round trip: the
// users.email UNIQUE constraint (schema.sql) is the actual source of truth,
// so a duplicate surfaces as a Save error instead of a racy check-then-act.
func RegisterUser(ctx context.Context, userRepo ports.UserRepository, email, plaintextPassword string, role domain.Role) (*domain.User, error) {
	emailVO, err := domain.NewEmail(email)
	if err != nil {
		return nil, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	passwordHash, err := domain.NewPasswordHash(string(hashed))
	if err != nil {
		return nil, err
	}

	user := domain.NewUser(emailVO, passwordHash, role)
	if err := userRepo.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
