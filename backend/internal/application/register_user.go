package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// verificationCodeTTL is how long a freshly generated code stays valid --
// 15 minutes, per the design spec.
const verificationCodeTTL = 15 * time.Minute

// ErrVerificationEmailNotSent wraps a failure from the email provider itself
// -- distinct from every other RegisterUser error, because the account was
// still created successfully. The caller (registerUser handler) uses
// errors.Is against this to still return 201: the user exists and can
// verify later via /resend-verification-code, so treating this the same as
// a genuine registration failure (bad email, duplicate account) would be
// wrong per the design spec's HTTP section.
var ErrVerificationEmailNotSent = errors.New("application: failed to send verification email")

// RegisterUser hashes the plaintext password with bcrypt -- the domain
// PasswordHash Value Object only validates "non-empty", it has no idea how
// a hash is produced. The plaintext password never leaves this function:
// it's hashed immediately and discarded.
//
// Email uniqueness isn't pre-checked with a FindByEmail round trip: the
// users.email UNIQUE constraint (schema.sql) is the actual source of truth,
// so a duplicate surfaces as a Save error instead of a racy check-then-act.
//
// After the account is persisted, a 6-digit verification code is generated,
// stored, and emailed. If the email send fails, ErrVerificationEmailNotSent
// is returned (wrapping the underlying error) alongside the created user --
// the account and its stored code are NOT rolled back: a transient
// email-provider failure shouldn't destroy a just-created account, and the
// client can always fall back to the resend endpoint. Callers must check
// errors.Is(err, ErrVerificationEmailNotSent) to distinguish this from a
// genuine registration failure (the returned *domain.User is nil in every
// other error case, non-nil only here).
func RegisterUser(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, plaintextPassword, language string, role domain.Role) (*domain.User, error) {
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

	code, err := generateVerificationCode()
	if err != nil {
		return nil, err
	}
	if err := userRepo.SaveVerificationCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return nil, err
	}
	if err := emailSender.SendVerificationCode(ctx, user.Email().String(), code, language); err != nil {
		return user, fmt.Errorf("%w: %v", ErrVerificationEmailNotSent, err)
	}

	return user, nil
}
