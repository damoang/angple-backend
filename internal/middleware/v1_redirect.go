package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// V1RedirectConfig configures the v1→v2 redirect middleware
type V1RedirectConfig struct {
	// Enabled controls whether redirects are active (false = headers only, true = 301 redirect)
	Enabled bool
}

// v1ToV2Route defines a mapping from v1 path pattern to v2 path pattern.
// Placeholders like :board_id are preserved.
type v1ToV2Route struct {
	method string
	v1Path string
	v2Path string
}

// v1Redirects maps v1 endpoints to their v2 equivalents.
// Only endpoints that have v2 implementations are listed.
//
//nolint:gochecknoglobals
var v1Redirects = []v1ToV2Route{
	// Users
	{"GET", "/api/v1/auth/me", "/api/v2/me"},

	// Boards
	{"GET", "/api/v1/boards", "/api/v2/boards"},

	// Posts — v1 uses :board_id, v2 uses :slug (same value for migrated boards)
	{"GET", "/api/v1/boards/:board_id/posts", "/api/v2/boards/:board_id/posts"},
	{"POST", "/api/v1/boards/:board_id/posts", "/api/v2/boards/:board_id/posts"},

	// Comments
	{"GET", "/api/v1/boards/:board_id/posts/:id/comments", "/api/v2/boards/:board_id/posts/:id/comments"},
	{"POST", "/api/v1/boards/:board_id/posts/:id/comments", "/api/v2/boards/:board_id/posts/:id/comments"},

	// Scraps
	{"POST", "/api/v1/boards/:board_id/posts/:id/scrap", "/api/v2/posts/:id/scrap"},
	{"DELETE", "/api/v1/boards/:board_id/posts/:id/scrap", "/api/v2/posts/:id/scrap"},
	{"GET", "/api/v1/members/me/scraps", "/api/v2/me/scraps"},

	// Memos
	{"GET", "/api/v1/members/:id/memo", "/api/v2/members/:id/memo"},
	{"POST", "/api/v1/members/:id/memo", "/api/v2/members/:id/memo"},
	{"PUT", "/api/v1/members/:id/memo", "/api/v2/members/:id/memo"},
	{"DELETE", "/api/v1/members/:id/memo", "/api/v2/members/:id/memo"},

	// Messages
	{"POST", "/api/v1/messages", "/api/v2/messages"},
	{"GET", "/api/v1/messages/inbox", "/api/v2/messages/inbox"},
	{"GET", "/api/v1/messages/sent", "/api/v2/messages/sent"},
	{"GET", "/api/v1/messages/:id", "/api/v2/messages/:id"},
	{"DELETE", "/api/v1/messages/:id", "/api/v2/messages/:id"},
}

// V1ToV2Redirect returns a middleware that adds v2 migration hints.
// When cfg.Enabled is true, matching requests get a 301 redirect.
// When false, only the Link header with the v2 URL is added (informational).
func V1ToV2Redirect(cfg V1RedirectConfig) gin.HandlerFunc {
	// Build lookup: "METHOD /api/v1/..." -> v2 pattern
	type routeEntry struct {
		v1Pattern string
		v2Pattern string
	}
	lookup := make(map[string]routeEntry, len(v1Redirects))
	for _, r := range v1Redirects {
		key := r.method + " " + r.v1Path
		lookup[key] = routeEntry{v1Pattern: r.v1Path, v2Pattern: r.v2Path}
	}

	return func(c *gin.Context) {
		key := c.Request.Method + " " + c.FullPath()
		entry, ok := lookup[key]
		if !ok {
			c.Next()
			return
		}

		// Resolve actual URL by replacing params
		v2URL := resolveV2URL(entry.v2Pattern, entry.v1Pattern, c)

		if cfg.Enabled {
			c.Redirect(http.StatusMovedPermanently, v2URL)
			c.Abort()
			return
		}

		// Informational: add Link header pointing to v2
		c.Header("Link", `<`+v2URL+`>; rel="successor-version"`)
		c.Next()
	}
}

// resolveV2URL replaces gin route params in the v2 pattern using actual request values
func resolveV2URL(v2Pattern, v1Pattern string, c *gin.Context) string {
	result := v2Pattern

	// Extract param names from v1 pattern and substitute in v2 pattern
	v1Parts := strings.Split(v1Pattern, "/")
	for _, part := range v1Parts {
		if strings.HasPrefix(part, ":") {
			paramName := part[1:]
			paramValue := c.Param(paramName)
			if paramValue != "" {
				result = strings.Replace(result, part, paramValue, 1)
			}
		}
	}

	// Preserve query string
	if c.Request.URL.RawQuery != "" {
		result += "?" + c.Request.URL.RawQuery
	}

	return result
}
