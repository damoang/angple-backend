package redis

import (
	"context"
	"errors"
	"testing"
	"time"
)

// nil-Cache 동작 검증 — 모든 함수가 안전하게 fallthrough 해야 함.
func TestNilCacheNoOps(t *testing.T) {
	ctx := context.Background()
	var c *Cache // nil

	// Get → ErrCacheMiss
	if _, err := Get[string](ctx, c, "k"); !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get nil: want ErrCacheMiss, got %v", err)
	}

	// Set → no-op, no error
	if err := Set(ctx, c, "k", "v", time.Minute); err != nil {
		t.Errorf("Set nil: want nil, got %v", err)
	}

	// GetOrSet → loader called, returns its value
	called := false
	v, err := GetOrSet(ctx, c, "k", time.Minute, func() (string, error) {
		called = true
		return "loaded", nil
	})
	if err != nil || v != "loaded" || !called {
		t.Errorf("GetOrSet nil: want loader called and value=loaded, got v=%q err=%v called=%v", v, err, called)
	}

	// Invalidate → no-op
	if err := c.Invalidate(ctx, "k1", "k2"); err != nil {
		t.Errorf("Invalidate nil: want nil, got %v", err)
	}

	// InvalidatePrefix → no-op
	if err := c.InvalidatePrefix(ctx, "prefix:"); err != nil {
		t.Errorf("InvalidatePrefix nil: want nil, got %v", err)
	}
}

// GetOrSet loader error propagation — 캐시 miss 시 loader err 그대로 반환.
func TestGetOrSetLoaderError(t *testing.T) {
	ctx := context.Background()
	var c *Cache
	expected := errors.New("loader failed")
	_, err := GetOrSet(ctx, c, "k", time.Minute, func() (string, error) {
		return "", expected
	})
	if !errors.Is(err, expected) {
		t.Errorf("want %v, got %v", expected, err)
	}
}
