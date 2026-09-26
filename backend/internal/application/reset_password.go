package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ErrResetCodeInvalid covers a wrong code, an expired one, "no code found
// at all", and "no such account" -- deliberately not distinguished, same
// reasoning as ErrVerificationCodeInvalid: there's no operational reason to
// tell a client which of the four happened, and collapsing "no such
// account" into the same error here (rather than a separate case) keeps
// this endpoint from being a second anti-enumeration surface to get wrong.
var ErrResetCodeInvalid = errors.New("application: invalid or expired reset code")

// maxResetCodeAttempts caps how many guesses a single reset code tolerates
// before it's locked out regardless of whether a later guess would have
// been correct -- a 6-digit code with a 15-minute TTL and no attempt limit
// is brute-forceable by an unthrottled caller well within that window. A
// fresh code (via ForgotPassword's resend) resets the counter.
const maxResetCodeAttempts = 5

// ResetPassword mirrors VerifyEmail's validation shape exactly, but applies
// a new password instead of marking the account verified. Unlike
// RegisterUser's email-send failure (where the account still exists and
// works), a failed userRepo.Save here after ChangePassword is a genuine
// error to the caller -- the password was NOT actually changed, there is no
// "the important part still succeeded" case.
func ResetPassword(ctx context.Context, userRepo ports.UserRepository, email, code, newPlaintextPassword string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrResetCodeInvalid
	}
	if user.Status() == domain.UserStatusDeleted {
		return ErrResetCodeInvalid
	}

	storedCode, expiresAt, err := userRepo.FindResetCode(ctx, user.ID())
	if err != nil {
		return ErrResetCodeInvalid
	}

	// Count this attempt before comparing, so a lockout applies even to a
	// guess that would otherwise have been correct -- letting a correct
	// guess through after the limit defeats the point of counting at all.
	attempts, err := userRepo.IncrementResetCodeAttempts(ctx, user.ID())
	if err != nil {
		return err
	}
	if attempts > maxResetCodeAttempts {
		return ErrResetCodeInvalid
	}

	if subtle.ConstantTimeCompare([]byte(storedCode), []byte(code)) != 1 {
		return ErrResetCodeInvalid
	}
	if time.Now().After(expiresAt) {
		return ErrResetCodeInvalid
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPlaintextPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	passwordHash, err := domain.NewPasswordHash(string(hashed))
	if err != nil {
		return err
	}
	if err := user.ChangePassword(passwordHash); err != nil {
		return err
	}
	// Entering the code delivered to this address is the same proof email
	// verification demands, so a reset completes it too -- otherwise an
	// unverified account that resets successfully still can't log in
	// (LoginUser gates on EmailVerified), which was a real dead end the
	// mobile client had to work around before this. MarkEmailVerified is
	// idempotent, so this is a no-op for an already-verified account.
	if err := user.MarkEmailVerified(); err != nil {
		return err
	}
	if err := userRepo.Save(ctx, user); err != nil {
		return err
	}
	// Clear the code so it can't be replayed -- same technique VerifyEmail
	// uses: an already-past expiry makes any future FindResetCode/compare
	// fail regardless of what value is stored.
	return userRepo.SaveResetCode(ctx, user.ID(), "", time.Now().Add(-1*time.Hour))
}
