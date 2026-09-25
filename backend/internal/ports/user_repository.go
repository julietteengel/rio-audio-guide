package ports

import (
	"context"
	"time"

	"rioaudioguide/backend/internal/domain"
)

type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	// SaveVerificationCode overwrites any previously stored code for this
	// user -- a resend replaces, it never accumulates multiple live codes.
	SaveVerificationCode(ctx context.Context, userID, code string, expiresAt time.Time) error
	// FindVerificationCode returns the same not-found error shape as
	// FindByID/FindByEmail (pgx.ErrNoRows, surfaced as-is -- status-code
	// translation happens at the HTTP layer, not here) when no code is
	// currently stored for this user.
	FindVerificationCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
	// SaveResetCode/FindResetCode mirror SaveVerificationCode/FindVerificationCode
	// exactly, but store into reset_code/reset_code_expires_at instead --
	// deliberately separate columns so a password-reset request and an
	// email-verification request for the same account can never overwrite
	// each other's in-flight code.
	SaveResetCode(ctx context.Context, userID, code string, expiresAt time.Time) error
	FindResetCode(ctx context.Context, userID string) (code string, expiresAt time.Time, err error)
}
