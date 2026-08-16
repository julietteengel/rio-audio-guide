package redis

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// TestCache_GetConnectionError vérifie que Get distingue bien une vraie panne
// Redis (client qui ne peut pas joindre le serveur) d'un miss normal — sans
// cette distinction, la logique fail-open des handlers HTTP (Task 6) ne
// pourrait pas savoir si "found=false" veut dire "clé absente" ou "cache en
// panne", ce qui les mélangerait dans les métriques/logs et masquerait une
// vraie indisponibilité Redis derrière un simple miss silencieux.
func TestCache_GetConnectionError(t *testing.T) {
	client := goredis.NewClient(&goredis.Options{
		Addr:        "localhost:1", // rien n'écoute là — la connexion échoue toujours
		DialTimeout: 100 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewCache(client)

	_, found, err := cache.Get(context.Background(), "any-key")
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
	if found {
		t.Fatal("expected found=false when Get errors")
	}
}
