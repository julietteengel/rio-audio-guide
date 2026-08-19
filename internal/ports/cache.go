package ports

import (
	"context"
	"time"
)

// Cache est le port sortant vers un cache de lectures chaudes (Redis) —
// implémenté par l'adaptateur redis (internal/adapters/redis). Toute erreur
// doit être traitée par l'appelant comme un cache miss (fail-open) : Cache
// n'est un point de défaillance unique que pour la performance, jamais pour
// la correction fonctionnelle.
type Cache interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}
