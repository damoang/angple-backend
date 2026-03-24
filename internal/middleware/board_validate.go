package middleware

import (
	"net/http"
	"regexp"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/gin-gonic/gin"
)

// BoardSlugRegex allows ASCII letters, digits, and underscores, max 20 chars.
// Some legacy Gnuboard boards in production use mixed-case bo_table values
// such as MSSurface, so uppercase slugs must also pass validation.
var BoardSlugRegex = regexp.MustCompile(`^[A-Za-z0-9_]{1,20}$`)

// ValidateBoardSlug rejects requests whose :slug parameter contains invalid characters.
func ValidateBoardSlug() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if slug == "" {
			c.Next()
			return
		}
		if !BoardSlugRegex.MatchString(slug) {
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid board ID", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
