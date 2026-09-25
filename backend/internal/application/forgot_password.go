package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/domain"
	"rioaudioguide/backend/internal/ports"
)

// ForgotPassword always returns nil, whether or not the email belongs to a
// real account -- same anti-enumeration stance as ResendVerificationCode.
// Reuses generateVerificationCode() as-is: it's purpose-agnostic, just "a
// random 6-digit numeric string" -- there's no reason to duplicate it for
// a second purpose.
func ForgotPassword(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, language string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil
	}
	if user.Status() == domain.UserStatusDeleted {
		return nil
	}

	code, err := generateVerificationCode()
	if err != nil {
		return err
	}
	if err := userRepo.SaveResetCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return err
	}
	return emailSender.SendPasswordResetCode(ctx, user.Email().String(), code, language)
}
