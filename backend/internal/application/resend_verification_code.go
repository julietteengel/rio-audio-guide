package application

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/ports"
)

// ResendVerificationCode always returns nil, whether or not the email
// belongs to a real account -- mirrors this project's existing
// anti-enumeration stance on /login (ErrInvalidCredentials never
// distinguishes "no such email" from "wrong password"). A resend endpoint
// that reveals which emails have accounts would be a regression from that
// stance.
func ResendVerificationCode(ctx context.Context, userRepo ports.UserRepository, emailSender ports.EmailSender, email, language string) error {
	user, err := userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil
	}

	code, err := generateVerificationCode()
	if err != nil {
		return err
	}
	if err := userRepo.SaveVerificationCode(ctx, user.ID(), code, time.Now().Add(verificationCodeTTL)); err != nil {
		return err
	}
	return emailSender.SendVerificationCode(ctx, user.Email().String(), code, language)
}
