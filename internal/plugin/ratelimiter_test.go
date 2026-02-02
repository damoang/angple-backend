package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter_InMemory_Allow(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("test-plugin", 5, time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimiter_InMemory_Block(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("test-plugin", 3, time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 3 allowed
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i+1, w.Code)
		}
	}

	// 4th blocked
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: expected 429, got %d", w.Code)
	}
}

func TestRateLimiter_NoConfig_Passthrough(t *testing.T) {
	rl := NewRateLimiter(nil)
	// no Configure call

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware("unconfigured"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected passthrough 200, got %d", w.Code)
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("test-plugin", 1, time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// IP 1 - allowed
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("IP1: expected 200, got %d", w1.Code)
	}

	// IP 2 - also allowed (separate counter)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("IP2: expected 200, got %d", w2.Code)
	}
}

func TestRateLimiter_Remove(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("plugin-a", 10, time.Minute)

	if cfg := rl.GetConfig("plugin-a"); cfg == nil {
		t.Fatal("expected config to exist")
	}

	rl.Remove("plugin-a")

	if cfg := rl.GetConfig("plugin-a"); cfg != nil {
		t.Fatal("expected config to be removed")
	}
}

func TestRateLimiter_GetAllConfigs(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("a", 10, time.Minute)
	rl.Configure("b", 20, time.Hour)

	configs := rl.GetAllConfigs()
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	rl := NewRateLimiter(nil)
	rl.Configure("test-plugin", 1, 50*time.Millisecond)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 1st request - allowed
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("1st: expected 200, got %d", w.Code)
	}

	// 2nd request - blocked
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd: expected 429, got %d", w.Code)
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// 3rd request - allowed again
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("3rd after reset: expected 200, got %d", w.Code)
	}
}
