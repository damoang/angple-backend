package middleware

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// RequireLocalhost blocks requests not originating from 127.0.0.1 or ::1
func RequireLocalhost() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := getRemoteIP(c)
		if ip != localhostIPv4 && ip != localhostIPv6 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

// RequireAdmin checks that the authenticated user has admin level (>= 10)
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		level := GetUserLevel(c)
		if level < 10 {
			common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
