package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// k8sInternalNets defines K8s internal CIDR ranges to bypass rate limiting
var k8sInternalNets []*net.IPNet

func init() {
	for _, cidr := range []string{"10.42.0.0/16", "10.43.0.0/16"} {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid internal CIDR %q: %v", cidr, err))
		}
		k8sInternalNets = append(k8sInternalNets, ipNet)
	}
}

// isInternalIP checks if the IP is localhost or K8s internal
func isInternalIP(ip string) bool {
	if ip == localhostIPv4 || ip == localhostIPv6 {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, ipNet := range k8sInternalNets {
		if ipNet.Contains(parsed) {
			return true
		}
	}
	return false
}

// RateLimitConfig configures the rate limiter
type RateLimitConfig struct {
	RequestsPerMinute int
	KeyPrefix         string
	Message           string
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 600,
		KeyPrefix:         "api:ratelimit:",
		Message:           "요청이 너무 많습니다. 잠시 후 다시 시도해주세요.",
	}
}

// rateLimitScript is an atomic Lua script for sliding window rate limiting
var rateLimitScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local window_start = now - window

redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now, now .. ':' .. math.random(1000000))
    redis.call('EXPIRE', key, math.ceil(window / 1000) + 1)
    return {1, limit - count - 1, 0}
else
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local reset_at = 0
    if #oldest >= 2 then
        reset_at = tonumber(oldest[2]) + window
    end
    return {0, 0, reset_at}
end
`)

// RateLimit returns a gin middleware that rate limits by client IP
func RateLimit(redisClient *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if redisClient == nil {
			c.Next()
			return
		}

		// SSR 요청은 rate limit 우회 (K8s 내부 Pod에서 오는 SvelteKit 요청 포함)
		remoteIP := getRemoteIP(c)
		if isInternalIP(remoteIP) {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		key := cfg.KeyPrefix + clientIP

		now := time.Now().UnixMilli()
		windowMs := int64(60 * 1000) // 1 minute

		ctx := context.Background()
		result, err := rateLimitScript.Run(ctx, redisClient, []string{key},
			cfg.RequestsPerMinute, windowMs, now,
		).Int64Slice()

		if err != nil {
			// Fail open — allow request if Redis error
			c.Next()
			return
		}

		allowed := result[0] == 1
		remaining := result[1]
		resetAt := result[2]

		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			retryAfter := (resetAt - now) / 1000
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt/1000))
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   gin.H{"code": "RATE_LIMITED", "message": cfg.Message},
			})
			return
		}

		c.Next()
	}
}

// RateLimitPerUser returns a rate limiter keyed by user ID instead of IP
func RateLimitPerUser(redisClient *redis.Client, requestsPerMinute int) gin.HandlerFunc {
	cfg := RateLimitConfig{
		RequestsPerMinute: requestsPerMinute,
		KeyPrefix:         "api:ratelimit:user:",
		Message:           "요청이 너무 많습니다. 잠시 후 다시 시도해주세요.",
	}

	return func(c *gin.Context) {
		if redisClient == nil {
			c.Next()
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			// Fall back to IP if not authenticated
			userID = "ip:" + c.ClientIP()
		}

		key := cfg.KeyPrefix + userID

		now := time.Now().UnixMilli()
		windowMs := int64(60 * 1000)

		ctx := context.Background()
		result, err := rateLimitScript.Run(ctx, redisClient, []string{key},
			cfg.RequestsPerMinute, windowMs, now,
		).Int64Slice()

		if err != nil {
			c.Next()
			return
		}

		allowed := result[0] == 1
		remaining := result[1]

		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   gin.H{"code": "RATE_LIMITED", "message": cfg.Message},
			})
			return
		}

		c.Next()
	}
}
