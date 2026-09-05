package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (rc *RedisCache) Get(ctx context.Context, shortCode string) (string, error) {
	val, err := rc.client.Get(ctx, "url:"+shortCode).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("cache get failed: %w", err)
	}
	return val, nil
}

func (rc *RedisCache) Set(ctx context.Context, shortCode, longURL string, ttl time.Duration) error {
	return rc.client.Set(ctx, "url:"+shortCode, longURL, ttl).Err()
}

func (rc *RedisCache) Delete(ctx context.Context, shortCode string) error {
	return rc.client.Del(ctx, "url:"+shortCode).Err()
}
