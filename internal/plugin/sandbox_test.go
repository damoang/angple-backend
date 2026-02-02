package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSandbox_RecoverPanic(t *testing.T) {
	logger := NewDefaultLogger("test")
	cfg := DefaultSandboxConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/panic", SandboxMiddleware("test-plugin", cfg, logger), func(_ *gin.Context) {
		panic("something went wrong")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Fatal("expected error response body")
	}
}

func TestSandbox_NoPanic(t *testing.T) {
	logger := NewDefaultLogger("test")
	cfg := DefaultSandboxConfig()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ok", SandboxMiddleware("test-plugin", cfg, logger), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/ok", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSandbox_Timeout(t *testing.T) {
	logger := NewDefaultLogger("test")
	cfg := SandboxConfig{
		RequestTimeout: 50 * time.Millisecond,
		RecoverPanics:  true,
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/slow", SandboxMiddleware("slow-plugin", cfg, logger), func(c *gin.Context) {
		select {
		case <-time.After(200 * time.Millisecond):
			c.String(http.StatusOK, "done")
		case <-c.Request.Context().Done():
			// Context canceled
			return
		}
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/slow", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", w.Code)
	}
}

func TestSandbox_RecoverDisabled(t *testing.T) {
	logger := NewDefaultLogger("test")
	cfg := SandboxConfig{
		RecoverPanics: false,
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/panic", SandboxMiddleware("test-plugin", cfg, logger), func(_ *gin.Context) {
		panic("boom")
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate when RecoverPanics is false")
		}
	}()

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/panic", nil)
	r.ServeHTTP(w, req)
}

func TestSafeCall_Normal(t *testing.T) {
	logger := NewDefaultLogger("test")

	err := SafeCall("test", logger, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestSafeCall_Error(t *testing.T) {
	logger := NewDefaultLogger("test")

	err := SafeCall("test", logger, func() error {
		return errors.New("expected error")
	})
	if err == nil || err.Error() != "expected error" {
		t.Fatalf("expected 'expected error', got: %v", err)
	}
}

func TestSafeCall_Panic(t *testing.T) {
	logger := NewDefaultLogger("test")

	err := SafeCall("test", logger, func() error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error from panic")
	}
}
