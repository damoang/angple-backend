package middleware

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

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
