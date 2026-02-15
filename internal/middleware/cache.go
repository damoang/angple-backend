package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// CacheConfig configures the cache middleware
type CacheConfig struct {
	TTL       time.Duration
	KeyPrefix string
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:       5 * time.Minute,
		KeyPrefix: "api:cache:",
	}
}

type cachedResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// Cache returns a gin middleware that caches GET responses in Redis
func Cache(redisClient *redis.Client, cfg CacheConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache GET requests
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		// Skip if no Redis
		if redisClient == nil {
			c.Next()
			return
		}

		// Build cache key
		key := cfg.KeyPrefix + cacheKey(c.Request.URL.Path, c.Request.URL.RawQuery)

		// Try cache hit
		ctx := c.Request.Context()
		val, err := redisClient.Get(ctx, key).Bytes()
		if err == nil {
			var cached cachedResponse
			if json.Unmarshal(val, &cached) == nil {
				for k, v := range cached.Headers {
					c.Header(k, v)
				}
				c.Header("X-Cache", "HIT")
				c.Data(cached.Status, "application/json", []byte(cached.Body))
				c.Abort()
				return
			}
		}

		// Cache miss — capture response
		w := &responseWriter{ResponseWriter: c.Writer, body: make([]byte, 0, 1024)}
		c.Writer = w

		c.Next()

		// Only cache successful responses
		if w.status >= 200 && w.status < 300 {
			headers := map[string]string{
				"Content-Type": w.Header().Get("Content-Type"),
			}
			cached := cachedResponse{
				Status:  w.status,
				Headers: headers,
				Body:    string(w.body),
			}
			data, err := json.Marshal(cached)
			if err != nil {
				return
			}
			redisClient.Set(ctx, key, data, cfg.TTL)
		}

		c.Header("X-Cache", "MISS")
	}
}

// CacheWithTTL is a shorthand for Cache with a custom TTL
func CacheWithTTL(redisClient *redis.Client, ttl time.Duration) gin.HandlerFunc {
	cfg := DefaultCacheConfig()
	cfg.TTL = ttl
	return Cache(redisClient, cfg)
}

// InvalidateCache deletes cache entries matching a prefix pattern
func InvalidateCache(redisClient *redis.Client, prefix string) {
	if redisClient == nil {
		return
	}
	ctx := context.Background()
	iter := redisClient.Scan(ctx, 0, prefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		redisClient.Del(ctx, iter.Val())
	}
}

func cacheKey(path, query string) string {
	raw := path
	if query != "" {
		raw += "?" + query
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}

// responseWriter captures the response body
type responseWriter struct {
	gin.ResponseWriter
	body   []byte
	status int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.body = append(w.body, []byte(s)...)
	return w.ResponseWriter.WriteString(s)
}
