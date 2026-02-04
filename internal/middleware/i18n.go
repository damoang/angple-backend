package middleware

import (
	"github.com/damoang/angple-backend/pkg/i18n"
	"github.com/gin-gonic/gin"
)

const localeKey = "locale"

// I18n middleware detects the client's preferred language from Accept-Language header
// and stores it in the gin context for later use.
func I18n() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Accept-Language")
		locale := i18n.ParseAcceptLanguage(header)
		c.Set(localeKey, locale)
		c.Header("Content-Language", string(locale))
		c.Next()
	}
}

// GetLocale returns the locale from the gin context (set by I18n middleware)
func GetLocale(c *gin.Context) i18n.Locale {
	if v, exists := c.Get(localeKey); exists {
		if locale, ok := v.(i18n.Locale); ok {
			return locale
		}
	}
	return i18n.LocaleKo
}
