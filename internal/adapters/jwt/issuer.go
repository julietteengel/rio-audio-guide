// Package jwt implements ports.TokenIssuer using signed JWTs (HS256 --
// one shared secret signs and verifies, which is enough for a single
// backend service; RS256/asymmetric keys only start to matter once
// multiple independent services need to verify tokens without all holding
// the same secret).
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"rioaudioguide/backend/internal/domain"
)

// tokenTTL: no refresh-token flow yet, so this is a straight tradeoff
// between security (shorter is better if a token leaks) and UX (longer
// means fewer forced re-logins). 24h is a reasonable default for a mobile
// app without silent refresh; revisit once refresh tokens exist.
const tokenTTL = 24 * time.Hour

var (
	ErrTokenInvalid = errors.New("jwt: token is invalid or expired")
	ErrEmptySecret  = errors.New("jwt: secret must not be empty")
)

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret []byte
}

func NewIssuer(secret string) (*Issuer, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	return &Issuer{secret: []byte(secret)}, nil
}

func (i *Issuer) Issue(userID string, role domain.Role) (string, error) {
	now := time.Now()
	c := claims{
		Role: role.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(i.secret)
}

func (i *Issuer) Verify(tokenString string) (string, domain.Role, error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (any, error) {
		return i.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil || !token.Valid {
		return "", "", ErrTokenInvalid
	}

	role, err := domain.NewRole(c.Role)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	return c.Subject, role, nil
}
