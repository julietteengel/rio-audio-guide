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
		// Contre une adresse délibérément morte, les défauts du SDK coûtent
		// ~1.7s et 4 lignes de bruit sur stderr pour un résultat certain dès le
		// 1er essai. Deux réglages distincts, les deux nécessaires :
		//   - MaxRetries = -1 désactive les réessais de commande. Attention,
		//     0 ne les désactive PAS : go-redis mappe 0 sur son défaut (3).
		//   - DialerRetries = 1 coupe la boucle de reconnexion du pool
		//     (défaut : 5 essais espacés de 100ms), qui est ce qui domine
		//     réellement le temps ici.
		// Mesuré : 1.7s → ~1ms, 4 lignes de bruit → 1.
		MaxRetries:    -1,
		DialerRetries: 1,
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
