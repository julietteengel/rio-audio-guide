package ports

import "context"

type EmailSender interface {
	SendVerificationCode(ctx context.Context, toEmail, code string) error
}
