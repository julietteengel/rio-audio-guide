package ports

import (
	"context"
	"time"
)

// AudioStorage est le port sortant vers le stockage objet (S3) — implémenté
// par l'adaptateur s3 (internal/adapters/s3).
type AudioStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (url string, err error)

	// PresignURL convertit une clé de stockage en URL HTTP temporaire que le
	// client peut charger directement — le storage_url stocké en base
	// (s3://bucket/clé) n'est pas une URL HTTP utilisable telle quelle.
	PresignURL(ctx context.Context, key string, expiry time.Duration) (url string, err error)
}
