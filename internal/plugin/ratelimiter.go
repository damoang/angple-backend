package plugin

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig 플러그인별 레이트 리밋 설정
type RateLimitConfig struct {
	PluginName string        // 플러그인 이름
	Requests   int           // 허용 요청 수
	Window     time.Duration // 시간 윈도우
}

// RateLimiter Redis 기반 슬라이딩 윈도우 레이트 리미터
type RateLimiter struct {
	redis    *redis.Client
	configs  map[string]*RateLimitConfig // pluginName -> config
	mu       sync.RWMutex
	fallback *inMemoryLimiter // Redis 없을 때 폴백
}

// NewRateLimiter 생성자
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:    redisClient,
		configs:  make(map[string]*RateLimitConfig),
		fallback: newInMemoryLimiter(),
	}
}

// Configure 플러그인 레이트 리밋 설정
func (rl *RateLimiter) Configure(pluginName string, requests int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.configs[pluginName] = &RateLimitConfig{
		PluginName: pluginName,
		Requests:   requests,
		Window:     window,
	}
}

// Remove 플러그인 레이트 리밋 제거
func (rl *RateLimiter) Remove(pluginName string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.configs, pluginName)
	rl.fallback.remove(pluginName)
}

// GetConfig 설정 조회
func (rl *RateLimiter) GetConfig(pluginName string) *RateLimitConfig {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.configs[pluginName]
}

// GetAllConfigs 전체 설정 조회
func (rl *RateLimiter) GetAllConfigs() []RateLimitConfig {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	result := make([]RateLimitConfig, 0, len(rl.configs))
	for _, cfg := range rl.configs {
		result = append(result, *cfg)
	}
	return result
}

// Middleware Gin 미들웨어 - 플러그인 라우트에 적용
func (rl *RateLimiter) Middleware(pluginName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rl.mu.RLock()
		cfg, exists := rl.configs[pluginName]
		rl.mu.RUnlock()

		if !exists {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		key := fmt.Sprintf("ratelimit:%s:%s", pluginName, clientIP)

		var allowed bool
		var remaining int
		if rl.redis != nil {
			var err error
			allowed, remaining, err = rl.check(key, cfg)
			if err != nil {
				allowed, remaining = rl.fallback.check(pluginName, clientIP, cfg)
			}
		} else {
			allowed, remaining = rl.fallback.check(pluginName, clientIP, cfg)
		}

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Requests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Window", cfg.Window.String())

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": fmt.Sprintf("요청 한도 초과: %d회/%s", cfg.Requests, cfg.Window),
				},
			})
			return
		}

		c.Next()
	}
}

// check Redis 슬라이딩 윈도우 카운터
func (rl *RateLimiter) check(key string, cfg *RateLimitConfig) (bool, int, error) {
	ctx := context.Background()
	now := time.Now()

	pipe := rl.redis.Pipeline()

	// 윈도우 밖 요소 제거
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now.Add(-cfg.Window).UnixNano()))
	// 현재 요청 추가
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixNano()), Member: now.UnixNano()})
	// 카운트 조회
	countCmd := pipe.ZCard(ctx, key)
	// TTL 설정
	pipe.Expire(ctx, key, cfg.Window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, err
	}

	count := int(countCmd.Val())
	remaining := cfg.Requests - count
	if remaining < 0 {
		remaining = 0
	}

	return count <= cfg.Requests, remaining, nil
}

// inMemoryLimiter Redis 없을 때 사용하는 간단한 인메모리 리미터
type inMemoryLimiter struct {
	buckets map[string]*bucket
	mu      sync.Mutex
}

type bucket struct {
	count    int
	resetAt  time.Time
	requests int
	window   time.Duration
}

func newInMemoryLimiter() *inMemoryLimiter {
	return &inMemoryLimiter{
		buckets: make(map[string]*bucket),
	}
}

func (m *inMemoryLimiter) check(pluginName, clientIP string, cfg *RateLimitConfig) (bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := pluginName + ":" + clientIP
	b, exists := m.buckets[key]
	now := time.Now()

	if !exists || now.After(b.resetAt) {
		m.buckets[key] = &bucket{
			count:    1,
			resetAt:  now.Add(cfg.Window),
			requests: cfg.Requests,
			window:   cfg.Window,
		}
		return true, cfg.Requests - 1
	}

	b.count++
	remaining := cfg.Requests - b.count
	if remaining < 0 {
		remaining = 0
	}
	return b.count <= cfg.Requests, remaining
}

func (m *inMemoryLimiter) remove(pluginName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.buckets {
		if len(key) > len(pluginName) && key[:len(pluginName)+1] == pluginName+":" {
			delete(m.buckets, key)
		}
	}
}
