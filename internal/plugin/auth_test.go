package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestVerifier(valid bool) *DefaultJWTVerifier {
	return NewDefaultJWTVerifier(func(_ string) (string, string, int, error) {
		if valid {
			return "user1", "nick", 2, nil
		}
		return "", "", 0, errors.New("invalid token")
	})
}

func TestPluginAuth_Required_WithValidToken(t *testing.T) {
	verifier := newTestVerifier(true)
	r := gin.New()
	r.GET("/test", AuthMiddleware("required", verifier), func(c *gin.Context) {
		userID, _ := c.Get("userID")
		c.String(http.StatusOK, userID.(string))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "user1" {
		t.Fatalf("expected user1, got %s", w.Body.String())
	}
}

func TestPluginAuth_Required_NoToken(t *testing.T) {
	verifier := newTestVerifier(true)
	r := gin.New()
	r.GET("/test", AuthMiddleware("required", verifier), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPluginAuth_Required_InvalidToken(t *testing.T) {
	verifier := newTestVerifier(false)
	r := gin.New()
	r.GET("/test", AuthMiddleware("required", verifier), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestPluginAuth_Optional_WithToken(t *testing.T) {
	verifier := newTestVerifier(true)
	r := gin.New()
	r.GET("/test", AuthMiddleware("optional", verifier), func(c *gin.Context) {
		userID, _ := c.Get("userID")
		c.String(http.StatusOK, userID.(string))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPluginAuth_Optional_NoToken(t *testing.T) {
	verifier := newTestVerifier(true)
	r := gin.New()
	r.GET("/test", AuthMiddleware("optional", verifier), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPluginAuth_None(t *testing.T) {
	r := gin.New()
	r.GET("/test", AuthMiddleware("none", nil), func(c *gin.Context) {
		c.String(http.StatusOK, "public")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPluginAuth_NilVerifier(t *testing.T) {
	r := gin.New()
	r.GET("/test", AuthMiddleware("required", nil), func(c *gin.Context) {
		c.String(http.StatusOK, "pass")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when verifier is nil, got %d", w.Code)
	}
}
