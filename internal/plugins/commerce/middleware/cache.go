package middleware

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// CacheConfig 캐시 설정
type CacheConfig struct {
	DefaultTTL time.Duration // 기본 TTL
	KeyPrefix  string        // 캐시 키 접두사
}

// CacheMiddleware Redis 캐시 미들웨어
type CacheMiddleware struct {
	redis  *redis.Client
	config *CacheConfig
}

// NewCacheMiddleware 새 캐시 미들웨어 생성
func NewCacheMiddleware(redisClient *redis.Client, config *CacheConfig) *CacheMiddleware {
	if config == nil {
		config = &CacheConfig{
			DefaultTTL: 5 * time.Minute,
			KeyPrefix:  "commerce:cache:",
		}
	}
	return &CacheMiddleware{
		redis:  redisClient,
		config: config,
	}
}

// cachedResponseWriter 캐시 가능한 응답 writer
type cachedResponseWriter struct {
	gin.ResponseWriter
	body       []byte
	statusCode int
}

func (w *cachedResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

func (w *cachedResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// CachedResponse 캐시된 응답 구조체
type CachedResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// Cache GET 요청 캐시 미들웨어
func (m *CacheMiddleware) Cache(ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// GET 요청만 캐시
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		// 캐시 키 생성
		cacheKey := m.generateCacheKey(c)

		// 캐시 조회
		ctx := context.Background()
		cached, err := m.redis.Get(ctx, cacheKey).Bytes()
		if err == nil {
			// 캐시 히트
			var response CachedResponse
			if err := json.Unmarshal(cached, &response); err == nil {
				// 헤더 설정
				for key, value := range response.Headers {
					c.Header(key, value)
				}
				c.Header("X-Cache", "HIT")
				c.Data(response.StatusCode, response.Headers["Content-Type"], response.Body)
				c.Abort()
				return
			}
		}

		// 캐시 미스 - 응답 캡처
		c.Header("X-Cache", "MISS")
		writer := &cachedResponseWriter{
			ResponseWriter: c.Writer,
			statusCode:     http.StatusOK,
		}
		c.Writer = writer

		c.Next()

		// 성공 응답만 캐시
		if writer.statusCode == http.StatusOK {
			response := CachedResponse{
				StatusCode: writer.statusCode,
				Headers: map[string]string{
					"Content-Type": c.Writer.Header().Get("Content-Type"),
				},
				Body: writer.body,
			}

			data, err := json.Marshal(response)
			if err == nil {
				effectiveTTL := ttl
				if effectiveTTL == 0 {
					effectiveTTL = m.config.DefaultTTL
				}
				m.redis.Set(ctx, cacheKey, data, effectiveTTL)
			}
		}
	}
}

// generateCacheKey 캐시 키 생성
func (m *CacheMiddleware) generateCacheKey(c *gin.Context) string {
	// URL + 쿼리 파라미터 기반 키 생성
	path := c.Request.URL.Path
	query := c.Request.URL.RawQuery

	keyData := fmt.Sprintf("%s?%s", path, query)
	hash := md5.Sum([]byte(keyData))
	hashStr := hex.EncodeToString(hash[:])

	return fmt.Sprintf("%s%s", m.config.KeyPrefix, hashStr)
}

// InvalidatePrefix 특정 접두사의 캐시 무효화
func (m *CacheMiddleware) InvalidatePrefix(ctx context.Context, prefix string) error {
	pattern := fmt.Sprintf("%s%s*", m.config.KeyPrefix, prefix)
	return m.invalidatePattern(ctx, pattern)
}

// InvalidateKey 특정 키의 캐시 무효화
func (m *CacheMiddleware) InvalidateKey(ctx context.Context, key string) error {
	return m.redis.Del(ctx, fmt.Sprintf("%s%s", m.config.KeyPrefix, key)).Err()
}

// InvalidateAll 모든 캐시 무효화
func (m *CacheMiddleware) InvalidateAll(ctx context.Context) error {
	pattern := fmt.Sprintf("%s*", m.config.KeyPrefix)
	return m.invalidatePattern(ctx, pattern)
}

// invalidatePattern 패턴 기반 캐시 무효화
func (m *CacheMiddleware) invalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = m.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := m.redis.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}
