package ports

import "context"

// AudioStorage est le port sortant vers le stockage objet (S3) — implémenté
// par l'adaptateur s3 (internal/adapters/s3).
type AudioStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (url string, err error)
}
