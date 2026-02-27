package middleware

import (
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/gin-gonic/gin"
)

const planFree = "free"

// TenantContext holds tenant-specific information
type TenantContext struct {
	SiteID     string `json:"site_id"`
	Subdomain  string `json:"subdomain"`
	DBStrategy string `json:"db_strategy"`
	Plan       string `json:"plan"`
}

// TenantMiddleware extracts tenant information from the request
// and stores it in the Gin context for downstream handlers
func TenantMiddleware(siteRepo *repository.SiteRepository, baseDomain string) gin.HandlerFunc {
	// Compile regex once for performance
	subdomainRegex := regexp.MustCompile(`^([a-z0-9-]+)\.` + regexp.QuoteMeta(baseDomain) + `$`)

	return func(c *gin.Context) {
		// Skip tenant resolution for non-tenant routes
		if shouldSkipTenant(c.Request.URL.Path) {
			c.Next()
			return
		}

		// 1. Extract subdomain from Host header
		host := c.Request.Host
		host = strings.Split(host, ":")[0] // Remove port if present
		host = strings.ToLower(host)

		var subdomain string
		var isCustomDomain bool

		// Check for custom domain header (set by Caddy)
		if customDomain := c.GetHeader("X-Custom-Domain"); customDomain != "" {
			isCustomDomain = true
			subdomain = customDomain
		} else if subdomainHeader := c.GetHeader("X-Tenant-Subdomain"); subdomainHeader != "" {
			// Caddy passes subdomain in header
			subdomain = subdomainHeader
		} else {
			// Extract from Host header
			matches := subdomainRegex.FindStringSubmatch(host)
			if len(matches) < 2 {
				// Not a subdomain, might be the main domain
				c.Next()
				return
			}
			subdomain = matches[1]
		}

		// Skip reserved subdomains
		if isReservedSubdomain(subdomain) {
			c.Next()
			return
		}

		// 2. Look up site by subdomain or custom domain
		var site interface{}
		var err error

		if isCustomDomain {
			site, err = siteRepo.FindByCustomDomain(c.Request.Context(), subdomain)
		} else {
			site, err = siteRepo.FindBySubdomain(c.Request.Context(), subdomain)
		}

		if err != nil {
			common.ErrorResponse(c, 404, "Site not found", nil)
			c.Abort()
			return
		}

		// 3. Extract site information
		// Note: Actual implementation would use proper type assertion
		// This is a placeholder structure
		siteMap, ok := site.(map[string]interface{})
		if !ok {
			common.ErrorResponse(c, 500, "Invalid site data", nil)
			c.Abort()
			return
		}

		tenant := TenantContext{
			SiteID:     getString(siteMap, "id"),
			Subdomain:  getString(siteMap, "subdomain"),
			DBStrategy: getString(siteMap, "db_strategy"),
			Plan:       getString(siteMap, "plan"),
		}

		// 4. Store tenant info in context
		c.Set("tenant", tenant)
		c.Set("tenant_id", tenant.SiteID)
		c.Set("tenant_subdomain", tenant.Subdomain)
		c.Set("tenant_db_strategy", tenant.DBStrategy)
		c.Set("tenant_plan", tenant.Plan)

		c.Next()
	}
}

// GetTenant extracts tenant context from Gin context
func GetTenant(c *gin.Context) *TenantContext {
	tenant, exists := c.Get("tenant")
	if !exists {
		return nil
	}

	if t, ok := tenant.(TenantContext); ok {
		return &t
	}
	return nil
}

// GetTenantID extracts tenant ID from context
func GetTenantID(c *gin.Context) string {
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		return ""
	}
	if str, ok := tenantID.(string); ok {
		return str
	}
	return ""
}

// GetTenantDBStrategy returns the database strategy for the tenant
func GetTenantDBStrategy(c *gin.Context) string {
	strategy, exists := c.Get("tenant_db_strategy")
	if !exists {
		return "shared" // Default to shared database
	}
	if str, ok := strategy.(string); ok {
		return str
	}
	return "shared"
}

// GetTenantPlan returns the subscription plan for the tenant
func GetTenantPlan(c *gin.Context) string {
	plan, exists := c.Get("tenant_plan")
	if !exists {
		return planFree
	}
	if str, ok := plan.(string); ok {
		return str
	}
	return planFree
}

// shouldSkipTenant returns true for paths that don't need tenant resolution
func shouldSkipTenant(path string) bool {
	skipPaths := []string{
		"/health",
		"/metrics",
		"/api/v1/auth",
		"/api/v1/sites/check-subdomain",
		"/api/v1/sites/verify-domain",
		"/webhook",
	}

	for _, skip := range skipPaths {
		if strings.HasPrefix(path, skip) {
			return true
		}
	}
	return false
}

// isReservedSubdomain returns true for reserved subdomains
func isReservedSubdomain(subdomain string) bool {
	reserved := map[string]bool{
		"www":     true,
		"api":     true,
		"admin":   true,
		"app":     true,
		"mail":    true,
		"smtp":    true,
		"pop":     true,
		"imap":    true,
		"ftp":     true,
		"cdn":     true,
		"static":  true,
		"assets":  true,
		"status":  true,
		"help":    true,
		"support": true,
		"docs":    true,
		"blog":    true,
		"store":   true,
		"shop":    true,
	}
	return reserved[subdomain]
}

// getString safely extracts a string from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// RequireTenant middleware ensures a tenant context exists
func RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := GetTenant(c)
		if tenant == nil {
			common.ErrorResponse(c, 400, "Tenant context required", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequirePlan middleware ensures the tenant has a specific plan or higher
func RequirePlan(minPlan string) gin.HandlerFunc {
	planOrder := map[string]int{
		planFree:     0,
		"pro":        1,
		"business":   2,
		"enterprise": 3,
	}

	return func(c *gin.Context) {
		tenantPlan := GetTenantPlan(c)
		minLevel, minOk := planOrder[minPlan]
		currentLevel, currentOk := planOrder[tenantPlan]

		if !minOk || !currentOk {
			common.ErrorResponse(c, 500, "Invalid plan configuration", nil)
			c.Abort()
			return
		}

		if currentLevel < minLevel {
			common.ErrorResponse(c, 403, "This feature requires a "+minPlan+" plan or higher", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}
