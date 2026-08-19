package domain

import (
	"errors"
	"regexp"
)

type UserStatus string

const (
	UserStatusActive  UserStatus = "active"
	UserStatusDeleted UserStatus = "deleted"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

var (
	ErrUserEmailRequired    = errors.New("user: email is required")
	ErrUserInvalidEmail     = errors.New("user: email is not a valid address")
	ErrUserPasswordRequired = errors.New("user: password hash is required")
	ErrUserInvalidRole      = errors.New("user: unsupported role")
	ErrUserAlreadyDeleted   = errors.New("user: already deleted")
	ErrUserDeleted          = errors.New("user: cannot edit a deleted user")
)

// --- Value Objects ---

type Email string

// emailPattern est volontairement permissif (pas de RFC 5322 complet) --
// suffisant pour rejeter une saisie clairement invalide, pas pour se
// substituer à une vérification par email de confirmation.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func NewEmail(s string) (Email, error) {
	if s == "" {
		return "", ErrUserEmailRequired
	}
	if !emailPattern.MatchString(s) {
		return "", ErrUserInvalidEmail
	}
	return Email(s), nil
}

func (e Email) String() string { return string(e) }

// PasswordHash ne contient jamais un mot de passe en clair -- le hashing
// (bcrypt) est un problème technique, pas une règle métier, donc il se fait
// avant d'arriver ici (application/adapter layer). Le Value Object ne fait
// que garantir qu'on n'a pas oublié cette étape.
type PasswordHash string

func NewPasswordHash(hash string) (PasswordHash, error) {
	if hash == "" {
		return "", ErrUserPasswordRequired
	}
	return PasswordHash(hash), nil
}

func (h PasswordHash) String() string { return string(h) }

func NewRole(s string) (Role, error) {
	r := Role(s)
	switch r {
	case RoleUser, RoleAdmin:
		return r, nil
	}
	return "", ErrUserInvalidRole
}

func (r Role) String() string { return string(r) }

// --- Entity ---

type User struct {
	id           string
	email        Email
	passwordHash PasswordHash
	role         Role
	status       UserStatus
}

// NewUser ne retourne plus d'erreur : email/passwordHash/role sont déjà
// validés avant d'arriver ici (Value Objects) -- même convention que
// NewPlace/NewScript.
func NewUser(email Email, passwordHash PasswordHash, role Role) *User {
	return &User{
		id:           newID(),
		email:        email,
		passwordHash: passwordHash,
		role:         role,
		status:       UserStatusActive,
	}
}

// ReconstructUser rebâtit un User depuis des données déjà valides (une
// ligne Postgres) -- préserve l'ID et le statut donnés, ne revalide rien.
func ReconstructUser(id string, email Email, passwordHash PasswordHash, role Role, status UserStatus) *User {
	return &User{
		id:           id,
		email:        email,
		passwordHash: passwordHash,
		role:         role,
		status:       status,
	}
}

func (u *User) ChangeEmail(email Email) error {
	if u.status == UserStatusDeleted {
		return ErrUserDeleted
	}
	u.email = email
	return nil
}

func (u *User) ChangePassword(hash PasswordHash) error {
	if u.status == UserStatusDeleted {
		return ErrUserDeleted
	}
	u.passwordHash = hash
	return nil
}

func (u *User) ChangeRole(role Role) error {
	if u.status == UserStatusDeleted {
		return ErrUserDeleted
	}
	u.role = role
	return nil
}

// Delete est un soft delete (même logique que Place.Remove) -- le compte
// disparaît du produit sans perdre la trace de qui a réalisé quelles
// actions passées (ex. Script.reviewerID pointant vers cet ID).
func (u *User) Delete() error {
	if u.status == UserStatusDeleted {
		return ErrUserAlreadyDeleted
	}
	u.status = UserStatusDeleted
	return nil
}

// --- lecture ---

func (u *User) ID() string                 { return u.id }
func (u *User) Email() Email               { return u.email }
func (u *User) PasswordHash() PasswordHash { return u.passwordHash }
func (u *User) Role() Role                 { return u.role }
func (u *User) Status() UserStatus         { return u.status }
