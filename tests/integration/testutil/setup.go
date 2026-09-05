// Package testutil provides shared setup for integration tests: real
// Postgres/Redis connections via DATABASE_URL/REDIS_URL, with table
// truncation between tests for isolation.
package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KabileshRajaselvan/url-shortener/internal/store"
)

func RequireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set; skipping integration test", key)
	}
	return v
}

func NewStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.New(ctx, RequireEnv(t, "DATABASE_URL"))
	require.NoError(t, err)
	t.Cleanup(s.Close)
	TruncateAll(t, s)
	return s
}

func NewRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(RequireEnv(t, "REDIS_URL"))
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { client.Close() })
	require.NoError(t, client.FlushDB(context.Background()).Err())
	return client
}

func TruncateAll(t *testing.T, s *store.Store) {
	t.Helper()
	require.NoError(t, s.Exec(context.Background(),
		"TRUNCATE TABLE daily_click_stats, click_events, urls, api_keys, users RESTART IDENTITY CASCADE"))
}
