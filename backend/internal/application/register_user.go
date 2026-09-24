package application

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// verificationCodeTTL is how long a freshly generated code stays valid --
// 15 minutes, per the design spec.
const verificationCodeTTL = 15 * time.Minute

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
// stored, and emailed. If the email send fails, the error is returned to
// the caller -- but the account and its stored code are NOT rolled back:
// a transient email-provider failure shouldn't destroy a just-created
// account, and the client can always fall back to the resend endpoint.
func RegisterUser(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, plaintextPassword string, role domain.Role) (*domain.User, error) {
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
	if err := emailSender.SendVerificationCode(ctx, user.Email().String(), code); err != nil {
		return nil, err
	}

	return user, nil
}
