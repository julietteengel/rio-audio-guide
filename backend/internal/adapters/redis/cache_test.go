//go:build integration

package redis

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) *goredis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCache_SetAndGet(t *testing.T) {
	client := testClient(t)
	cache := NewCache(client)
	ctx := context.Background()

	if err := cache.Set(ctx, "test:key", "test-value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}

	value, found, err := cache.Get(ctx, "test:key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !found || value != "test-value" {
		t.Fatalf("got (%q, %v), want (\"test-value\", true)", value, found)
	}
}

func TestCache_GetMissingKey(t *testing.T) {
	client := testClient(t)
	cache := NewCache(client)

	_, found, err := cache.Get(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for a missing key")
	}
}
