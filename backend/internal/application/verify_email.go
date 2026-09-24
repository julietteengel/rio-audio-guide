package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// ErrVerificationCodeInvalid covers a wrong code, an expired one, and "no
// code found at all" -- deliberately not distinguished, there's no
// operational reason to tell a client which of the three happened.
var ErrVerificationCodeInvalid = errors.New("application: invalid or expired verification code")

func VerifyEmail(ctx context.Context, userRepo ports.UserRepository, email, code string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrVerificationCodeInvalid
	}

	storedCode, expiresAt, err := userRepo.FindVerificationCode(ctx, user.ID())
	if err != nil {
		return ErrVerificationCodeInvalid
	}

	// Constant-time compare: a short numeric code checked over the network
	// is worth the trivial cost to avoid a timing side-channel, even though
	// the practical risk here is low.
	if subtle.ConstantTimeCompare([]byte(storedCode), []byte(code)) != 1 {
		return ErrVerificationCodeInvalid
	}
	if time.Now().After(expiresAt) {
		return ErrVerificationCodeInvalid
	}

	if err := user.MarkEmailVerified(); err != nil {
		return err
	}
	if err := userRepo.Save(ctx, user); err != nil {
		return err
	}
	// Clear the code so it can't be replayed -- an already-past expiry
	// makes any future FindVerificationCode/compare fail regardless of
	// what value is stored, so overwriting with an expired marker is
	// sufficient without needing a separate "clear" repository method.
	return userRepo.SaveVerificationCode(ctx, user.ID(), "", time.Now().Add(-1*time.Hour))
}
