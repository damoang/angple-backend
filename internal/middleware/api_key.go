package middleware

import (
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/gin-gonic/gin"
)

// APIKeyValidator validates API keys (implemented by OAuthService)
type APIKeyValidator interface {
	ValidateAPIKey(ctx interface{}, key string) (*domain.APIKey, error)
}

// APIKeyAuth authenticates requests using an API key.
// Checks X-API-Key header or api_key query parameter.
func APIKeyAuth(validator APIKeyValidator, requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key == "" {
			common.ErrorResponse(c, http.StatusUnauthorized, "API key required", nil)
			c.Abort()
			return
		}

		apiKey, err := validator.ValidateAPIKey(c.Request.Context(), key)
		if err != nil {
			common.ErrorResponse(c, http.StatusUnauthorized, err.Error(), nil)
			c.Abort()
			return
		}

		// Check scope
		if requiredScope != "" {
			scopes := strings.Split(apiKey.Scopes, ",")
			hasScope := false
			for _, s := range scopes {
				if strings.TrimSpace(s) == requiredScope || strings.TrimSpace(s) == "admin" {
					hasScope = true
					break
				}
			}
			if !hasScope {
				common.ErrorResponse(c, http.StatusForbidden, "Insufficient API key scope", nil)
				c.Abort()
				return
			}
		}

		c.Set("api_key_id", apiKey.ID)
		c.Set("api_key_user_id", apiKey.UserID)
		c.Set("userID", apiKey.UserID)
		c.Next()
	}
}
