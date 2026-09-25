package ports

import "context"

type EmailSender interface {
	SendVerificationCode(ctx context.Context, toEmail, code, language string) error
	SendPasswordResetCode(ctx context.Context, toEmail, code, language string) error
}
