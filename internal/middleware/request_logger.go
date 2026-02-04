package middleware

import (
	"time"

	"github.com/damoang/angple-backend/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestLogger returns a gin middleware that logs every request with structured fields
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Generate request ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()[:8]
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		// Process request
		c.Next()

		// Log after response
		latency := time.Since(start)
		status := c.Writer.Status()
		userID := GetDamoangUserID(c)

		event := logger.GetLogger().Info()
		if status >= 500 {
			event = logger.GetLogger().Error()
		} else if status >= 400 {
			event = logger.GetLogger().Warn()
		}

		event.
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("query", c.Request.URL.RawQuery).
			Int("status", status).
			Dur("latency", latency).
			Str("client_ip", c.ClientIP()).
			Str("user_id", userID).
			Int("body_size", c.Writer.Size()).
			Msg("request")
	}
}
