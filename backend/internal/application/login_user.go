package application

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrInvalidCredentials is deliberately the SAME error whether the email
// doesn't exist or the password is wrong -- distinguishing the two in the
// response would let an attacker enumerate which emails have accounts.
var ErrInvalidCredentials = errors.New("application: invalid email or password")

// LoginUser verifies the password against the stored bcrypt hash and, on
// success, issues a signed JWT carrying the user's ID and role -- nothing
// else goes in the token (see internal/adapters/jwt: the payload is
// readable by anyone, never a place for secrets).
func LoginUser(ctx context.Context, userRepo ports.UserRepository, tokens ports.TokenIssuer, email, plaintextPassword string) (string, error) {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	if user.Status() == domain.UserStatusDeleted {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash().String()), []byte(plaintextPassword)); err != nil {
		return "", ErrInvalidCredentials
	}

	return tokens.Issue(user.ID(), user.Role())
}
