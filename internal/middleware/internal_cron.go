package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	localhostIPv4 = "127.0.0.1"
	localhostIPv6 = "::1"
)

// RequireInternalCron allows localhost calls or trusted internal callers that
// present the shared internal secret header (see hasValidInternalSecret).
// Cron handlers still validate their own CRON_SECRET query parameter.
func RequireInternalCron() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := getRemoteIP(c)
		if ip == localhostIPv4 || ip == localhostIPv6 {
			c.Next()
			return
		}

		if hasValidInternalSecret(c.GetHeader("X-Internal-Secret")) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}
