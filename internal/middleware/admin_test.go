package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireAdmin_Allowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(func(c *gin.Context) {
		c.Set("level", 10)
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	c.Request, _ = http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_Denied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(func(c *gin.Context) {
		c.Set("level", 5)
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	c.Request, _ = http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_NoLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(RequireAdmin())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	c.Request, _ = http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, c.Request)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
