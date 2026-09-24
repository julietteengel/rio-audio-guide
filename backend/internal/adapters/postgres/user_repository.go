package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"rioaudioguide/backend/internal/domain"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

const upsertUserSQL = `
	INSERT INTO users (id, email, password_hash, role, status, email_verified)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (id) DO UPDATE SET
		email = EXCLUDED.email,
		password_hash = EXCLUDED.password_hash,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		email_verified = EXCLUDED.email_verified,
		updated_at = now()
`

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	_, err := r.db.Exec(ctx, upsertUserSQL,
		user.ID(), user.Email().String(), user.PasswordHash().String(), string(user.Role()), string(user.Status()), user.EmailVerified())
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status, email_verified FROM users WHERE id = $1
	`, id)
	return scanUser(row)
}

// FindByEmail sert au login : on cherche un compte par email, pas par ID.
// Contrairement à places.name, users.email a une contrainte UNIQUE (voir
// schema.sql) -- pas d'ambiguïté possible entre deux comptes de même email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status, email_verified FROM users WHERE email = $1
	`, email)
	return scanUser(row)
}

// SaveVerificationCode overwrites any previously stored code for this user
// -- a resend replaces the row's code/expiry in place, it never keeps a
// history of past codes.
func (r *UserRepository) SaveVerificationCode(ctx context.Context, userID, code string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET verification_code = $1, verification_code_expires_at = $2, updated_at = now() WHERE id = $3
	`, code, expiresAt, userID)
	return err
}

// FindVerificationCode returns pgx.ErrNoRows (the same sentinel
// FindByID/FindByEmail already surface unwrapped) when the user has no
// currently-stored code -- never registered one, or it was already cleared
// after a successful verification.
func (r *UserRepository) FindVerificationCode(ctx context.Context, userID string) (string, time.Time, error) {
	var code *string
	var expiresAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT verification_code, verification_code_expires_at FROM users WHERE id = $1
	`, userID).Scan(&code, &expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	if code == nil || expiresAt == nil {
		return "", time.Time{}, pgx.ErrNoRows
	}
	return *code, *expiresAt, nil
}

func scanUser(row rowScanner) (*domain.User, error) {
	var id, emailRaw, passwordHashRaw, roleRaw, statusRaw string
	var emailVerified bool
	if err := row.Scan(&id, &emailRaw, &passwordHashRaw, &roleRaw, &statusRaw, &emailVerified); err != nil {
		return nil, err
	}

	email, err := domain.NewEmail(emailRaw)
	if err != nil {
		return nil, err
	}
	passwordHash, err := domain.NewPasswordHash(passwordHashRaw)
	if err != nil {
		return nil, err
	}
	role, err := domain.NewRole(roleRaw)
	if err != nil {
		return nil, err
	}

	return domain.ReconstructUser(id, email, passwordHash, role, domain.UserStatus(statusRaw), emailVerified), nil
}
