// Package redis — JSON-typed cache helpers built on top of go-redis client.
//
// 2026-04-25: backend Redis cache middleware (Phase 2, N+1 해소) 도입을 위한 헬퍼.
// 비인증 사용자 GET 응답 캐시용. 무효화는 write 핸들러에서 수동 호출.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Cache wraps a redis.Client with JSON-typed GetOrSet + invalidation helpers.
// Nil-safe: methods on a nil Cache are no-ops (loader fallthrough).
type Cache struct {
	client *goredis.Client
}

// NewCache creates a Cache wrapper. Pass the *redis.Client returned from NewClient.
func NewCache(c *goredis.Client) *Cache {
	return &Cache{client: c}
}

// ErrCacheMiss is returned by Get when the key does not exist.
var ErrCacheMiss = errors.New("redis: cache miss")

// Get returns the JSON-decoded value or ErrCacheMiss.
func Get[T any](ctx context.Context, c *Cache, key string) (T, error) {
	var zero T
	if c == nil || c.client == nil {
		return zero, ErrCacheMiss
	}
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return zero, ErrCacheMiss
	}
	if err != nil {
		return zero, fmt.Errorf("redis get %q: %w", key, err)
	}
	var val T
	if err := json.Unmarshal(raw, &val); err != nil {
		return zero, fmt.Errorf("redis json decode %q: %w", key, err)
	}
	return val, nil
}

// Set stores v as JSON with the given TTL. Best-effort (errors logged by caller if needed).
func Set[T any](ctx context.Context, c *Cache, key string, v T, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return nil
	}
	blob, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("redis json encode %q: %w", key, err)
	}
	return c.client.Set(ctx, key, blob, ttl).Err()
}

// GetOrSet returns the cached value if present, else invokes loader, stores the
// result with TTL, and returns it. Cache write failures are silently ignored
// (we always return the loader's result on miss).
//
// Use only for idempotent, anonymous-safe data. Do NOT cache user-scoped data
// without including a user identifier in the key.
func GetOrSet[T any](
	ctx context.Context,
	c *Cache,
	key string,
	ttl time.Duration,
	loader func() (T, error),
) (T, error) {
	if c != nil && c.client != nil {
		if v, err := Get[T](ctx, c, key); err == nil {
			return v, nil
		}
	}
	v, err := loader()
	if err != nil {
		return v, err
	}
	if c != nil && c.client != nil {
		// 캐시 쓰기 실패는 path-blocking 이 아님 — best-effort.
		//nolint:errcheck // intentionally ignored: cache miss case still returns loader value
		Set(ctx, c, key, v, ttl)
	}
	return v, nil
}

// Invalidate best-effort deletes the given keys. Nil-safe.
func (c *Cache) Invalidate(ctx context.Context, keys ...string) error {
	if c == nil || c.client == nil || len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// InvalidatePrefix SCANs and deletes all keys matching prefix*. Sparingly use —
// SCAN is O(N) over the keyspace and may be slow on large databases. For
// frequent invalidation, prefer explicit per-key Invalidate.
func (c *Cache) InvalidatePrefix(ctx context.Context, prefix string) error {
	if c == nil || c.client == nil || prefix == "" {
		return nil
	}
	iter := c.client.Scan(ctx, 0, prefix+"*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan %q: %w", prefix, err)
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Client returns the underlying go-redis client (escape hatch for advanced ops).
func (c *Cache) Client() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.client
}
