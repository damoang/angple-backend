package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig Rate Limiting 설정
type RateLimitConfig struct {
	// 기본 제한
	RequestsPerMinute int           // 분당 요청 수 (기본: 100)
	WindowSize        time.Duration // 윈도우 크기 (기본: 1분)

	// 키 설정
	KeyPrefix string // Redis 키 접두사 (기본: "commerce:ratelimit:")

	// 응답 설정
	Message string // 제한 초과 시 메시지
}

// DefaultRateLimitConfig 기본 Rate Limiting 설정
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerMinute: 100,
		WindowSize:        time.Minute,
		KeyPrefix:         "commerce:ratelimit:",
		Message:           "Too many requests. Please try again later.",
	}
}

// RateLimiter Redis 기반 Rate Limiter
type RateLimiter struct {
	redis  *redis.Client
	config *RateLimitConfig
}

// NewRateLimiter 새 Rate Limiter 생성
func NewRateLimiter(redisClient *redis.Client, config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	return &RateLimiter{
		redis:  redisClient,
		config: config,
	}
}

// RateLimitInfo Rate Limit 정보
type RateLimitInfo struct {
	Limit     int   // 최대 요청 수
	Remaining int   // 남은 요청 수
	ResetAt   int64 // 리셋 시간 (Unix timestamp)
}

// Middleware Gin 미들웨어 반환
func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Redis가 없으면 Rate Limiting 건너뛰기
		if r.redis == nil {
			c.Next()
			return
		}

		// 클라이언트 IP 추출
		clientIP := r.getClientIP(c)

		// Rate Limit 체크
		info, allowed, err := r.checkRateLimit(c.Request.Context(), clientIP)
		if err != nil {
			// Redis 에러 시 요청 허용 (fail-open)
			c.Next()
			return
		}

		// Rate Limit 헤더 설정
		c.Header("X-RateLimit-Limit", strconv.Itoa(info.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(info.Remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(info.ResetAt, 10))

		if !allowed {
			// 제한 초과
			c.Header("Retry-After", strconv.FormatInt(info.ResetAt-time.Now().Unix(), 10))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": r.config.Message,
				},
			})
			return
		}

		c.Next()
	}
}

// getClientIP 클라이언트 IP 추출
func (r *RateLimiter) getClientIP(c *gin.Context) string {
	// X-Forwarded-For 헤더 확인 (프록시/로드밸런서 뒤에 있을 때)
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		return xff
	}

	// X-Real-IP 헤더 확인
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return xri
	}

	// 직접 연결된 클라이언트 IP
	return c.ClientIP()
}

// checkRateLimit 슬라이딩 윈도우 알고리즘으로 Rate Limit 체크
func (r *RateLimiter) checkRateLimit(ctx context.Context, clientIP string) (*RateLimitInfo, bool, error) {
	now := time.Now()
	key := fmt.Sprintf("%s%s", r.config.KeyPrefix, clientIP)

	// Lua 스크립트로 원자적 연산 수행
	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window_start = now - window

		-- 만료된 요청 제거
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

		-- 현재 요청 수
		local count = redis.call('ZCARD', key)

		if count < limit then
			-- 새 요청 추가
			redis.call('ZADD', key, now, now .. '-' .. math.random())
			redis.call('EXPIRE', key, window / 1000 + 1)
			return {count + 1, limit - count - 1, 1}
		else
			return {count, 0, 0}
		end
	`)

	result, err := script.Run(ctx, r.redis, []string{key},
		now.UnixMilli(),
		r.config.WindowSize.Milliseconds(),
		r.config.RequestsPerMinute,
	).Slice()

	if err != nil {
		return nil, true, err // Redis 에러 시 허용
	}

	_ = result[0] // count (사용하지 않음)
	remaining := int(result[1].(int64))
	allowed := result[2].(int64) == 1

	info := &RateLimitInfo{
		Limit:     r.config.RequestsPerMinute,
		Remaining: remaining,
		ResetAt:   now.Add(r.config.WindowSize).Unix(),
	}

	return info, allowed, nil
}

// GetCurrentUsage 현재 사용량 조회 (모니터링용)
func (r *RateLimiter) GetCurrentUsage(ctx context.Context, clientIP string) (int, error) {
	if r.redis == nil {
		return 0, nil
	}

	key := fmt.Sprintf("%s%s", r.config.KeyPrefix, clientIP)
	now := time.Now()
	windowStart := now.Add(-r.config.WindowSize)

	// 만료된 요청 제거
	r.redis.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", windowStart.UnixMilli()))

	// 현재 요청 수
	count, err := r.redis.ZCard(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// ResetLimit 특정 IP의 Rate Limit 리셋 (관리자용)
func (r *RateLimiter) ResetLimit(ctx context.Context, clientIP string) error {
	if r.redis == nil {
		return nil
	}

	key := fmt.Sprintf("%s%s", r.config.KeyPrefix, clientIP)
	return r.redis.Del(ctx, key).Err()
}

// CustomRateLimit 특정 엔드포인트에 다른 Rate Limit 적용
func (r *RateLimiter) CustomRateLimit(requestsPerMinute int) gin.HandlerFunc {
	customConfig := &RateLimitConfig{
		RequestsPerMinute: requestsPerMinute,
		WindowSize:        r.config.WindowSize,
		KeyPrefix:         r.config.KeyPrefix,
		Message:           r.config.Message,
	}

	customLimiter := &RateLimiter{
		redis:  r.redis,
		config: customConfig,
	}

	return customLimiter.Middleware()
}
