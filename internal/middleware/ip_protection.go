package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// IPProtectionConfig holds parsed IP protection settings from environment variables
type IPProtectionConfig struct {
	AdminIP    string            // replacement IP for super admins
	MemberList map[string]string // userID → replacement IP mapping
}

// LoadIPProtectionConfig loads IP protection settings from environment variables.
// KG_IPPROTECT_ADMIN_IP=127.0.0.1
// KG_IPPROTECT_LIST=police:112.112.112.112,ad:0.0.0.0
func LoadIPProtectionConfig() *IPProtectionConfig {
	cfg := &IPProtectionConfig{
		AdminIP:    os.Getenv("KG_IPPROTECT_ADMIN_IP"),
		MemberList: make(map[string]string),
	}

	list := os.Getenv("KG_IPPROTECT_LIST")
	if list == "" {
		return cfg
	}

	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) == 2 {
			userID := strings.TrimSpace(parts[0])
			ip := strings.TrimSpace(parts[1])
			if userID != "" && ip != "" {
				cfg.MemberList[userID] = ip
			}
		}
	}

	return cfg
}

// IPProtection middleware replaces real IPs for admins and designated members.
// Must be applied after JWTAuth so that userID and level are available in context.
func IPProtection(cfg *IPProtectionConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil {
			c.Next()
			return
		}

		userID := GetUserID(c)
		level := GetUserLevel(c)

		// Super admin (level >= 10) → replace with admin IP
		if level >= 10 && cfg.AdminIP != "" {
			c.Set("protected_ip", cfg.AdminIP)
			c.Next()
			return
		}

		// Designated member → replace with configured IP
		if userID != "" {
			if ip, ok := cfg.MemberList[userID]; ok {
				c.Set("protected_ip", ip)
			}
		}

		c.Next()
	}
}

// GetClientIP returns the protected IP if set, otherwise falls back to c.ClientIP()
func GetClientIP(c *gin.Context) string {
	if ip, exists := c.Get("protected_ip"); exists {
		if s, ok := ip.(string); ok && s != "" {
			return s
		}
	}
	return c.ClientIP()
}
