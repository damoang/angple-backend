package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const (
	localhostIPv4 = "127.0.0.1"
	localhostIPv6 = "::1"
)

// RequireInternalCron allows localhost calls or trusted internal callers that
// present the shared INTERNAL_SECRET header. Cron handlers still validate their
// own CRON_SECRET query parameter.
func RequireInternalCron() gin.HandlerFunc {
	internalSecret := os.Getenv("INTERNAL_SECRET")

	return func(c *gin.Context) {
		ip := getRemoteIP(c)
		if ip == localhostIPv4 || ip == localhostIPv6 {
			c.Next()
			return
		}

		headerSecret := c.GetHeader("X-Internal-Secret")
		if internalSecret != "" && headerSecret == internalSecret {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}
