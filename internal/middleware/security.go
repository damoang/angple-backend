package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds common security headers to all responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' https:; connect-src 'self' https:; frame-ancestors 'none'")

		// HSTS (only in production)
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

// InputSanitizer blocks requests with common XSS/injection patterns in query parameters
func InputSanitizer() gin.HandlerFunc {
	dangerousPatterns := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"onclick=",
		"onfocus=",
		"onmouseover=",
		"eval(",
		"document.cookie",
		"window.location",
		"String.fromCharCode",
	}

	return func(c *gin.Context) {
		// Check query parameters
		for _, values := range c.Request.URL.Query() {
			for _, v := range values {
				lower := strings.ToLower(v)
				for _, pattern := range dangerousPatterns {
					if strings.Contains(lower, pattern) {
						common.ErrorResponse(c, http.StatusBadRequest, "Potentially dangerous input detected", nil)
						c.Abort()
						return
					}
				}
			}
		}
		c.Next()
	}
}

// CSRFProtection provides CSRF protection for cookie-based authentication (v1 routes).
// It checks the X-CSRF-Token header against the csrf_token cookie on state-changing methods.
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only apply to state-changing methods
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		// Skip if using Bearer token (API clients, not browser)
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			c.Next()
			return
		}

		// Skip if using API key
		if c.GetHeader("X-API-Key") != "" || c.Query("api_key") != "" {
			c.Next()
			return
		}

		// For cookie-based auth, require CSRF token
		csrfCookie, err := c.Cookie("csrf_token")
		if err != nil || csrfCookie == "" {
			common.ErrorResponse(c, http.StatusForbidden, "CSRF token missing", nil)
			c.Abort()
			return
		}

		csrfHeader := c.GetHeader("X-CSRF-Token")
		if csrfHeader == "" || csrfHeader != csrfCookie {
			common.ErrorResponse(c, http.StatusForbidden, "CSRF token mismatch", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}

// GenerateCSRFToken generates a new CSRF token and sets it as a cookie
// GET /api/v1/tokens/csrf
func GenerateCSRFToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			common.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate CSRF token", nil)
			return
		}
		token := hex.EncodeToString(tokenBytes)

		c.SetCookie("csrf_token", token, 3600, "/", "", true, false) // HttpOnly=false so JS can read
		c.JSON(http.StatusOK, gin.H{"csrf_token": token})
	}
}
