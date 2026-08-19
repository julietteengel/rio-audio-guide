package ports

import "rioaudioguide/backend/internal/domain"

// TokenIssuer is the outbound port for issuing and verifying session tokens
// (JWT) — implemented by internal/adapters/jwt. Kept separate from
// UserRepository: issuing a token is a technical/crypto concern (signing,
// expiry), not a persistence concern.
type TokenIssuer interface {
	Issue(userID string, role domain.Role) (string, error)
	Verify(token string) (userID string, role domain.Role, err error)
}
