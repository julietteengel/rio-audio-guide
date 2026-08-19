package postgres

import (
	"context"

	"rioaudioguide/backend/internal/domain"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

const upsertUserSQL = `
	INSERT INTO users (id, email, password_hash, role, status)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (id) DO UPDATE SET
		email = EXCLUDED.email,
		password_hash = EXCLUDED.password_hash,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		updated_at = now()
`

func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	_, err := r.db.Exec(ctx, upsertUserSQL,
		user.ID(), user.Email().String(), user.PasswordHash().String(), string(user.Role()), string(user.Status()))
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status FROM users WHERE id = $1
	`, id)
	return scanUser(row)
}

// FindByEmail sert au login : on cherche un compte par email, pas par ID.
// Contrairement à places.name, users.email a une contrainte UNIQUE (voir
// schema.sql) -- pas d'ambiguïté possible entre deux comptes de même email.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, role, status FROM users WHERE email = $1
	`, email)
	return scanUser(row)
}

func scanUser(row rowScanner) (*domain.User, error) {
	var id, emailRaw, passwordHashRaw, roleRaw, statusRaw string
	if err := row.Scan(&id, &emailRaw, &passwordHashRaw, &roleRaw, &statusRaw); err != nil {
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

	return domain.ReconstructUser(id, email, passwordHash, role, domain.UserStatus(statusRaw)), nil
}
