package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadIPProtectionConfig(t *testing.T) {
	// Save and restore env
	origAdmin := os.Getenv("KG_IPPROTECT_ADMIN_IP")
	origList := os.Getenv("KG_IPPROTECT_LIST")
	defer func() {
		os.Setenv("KG_IPPROTECT_ADMIN_IP", origAdmin)
		os.Setenv("KG_IPPROTECT_LIST", origList)
	}()

	t.Run("empty env", func(t *testing.T) {
		os.Setenv("KG_IPPROTECT_ADMIN_IP", "")
		os.Setenv("KG_IPPROTECT_LIST", "")

		cfg := LoadIPProtectionConfig()
		if cfg.AdminIP != "" {
			t.Errorf("expected empty AdminIP, got %q", cfg.AdminIP)
		}
		if len(cfg.MemberList) != 0 {
			t.Errorf("expected empty MemberList, got %v", cfg.MemberList)
		}
	})

	t.Run("full config", func(t *testing.T) {
		os.Setenv("KG_IPPROTECT_ADMIN_IP", "127.0.0.1")
		os.Setenv("KG_IPPROTECT_LIST", "police:112.112.112.112,ad:0.0.0.0")

		cfg := LoadIPProtectionConfig()
		if cfg.AdminIP != "127.0.0.1" {
			t.Errorf("expected AdminIP=127.0.0.1, got %q", cfg.AdminIP)
		}
		if len(cfg.MemberList) != 2 {
			t.Errorf("expected 2 members, got %d", len(cfg.MemberList))
		}
		if cfg.MemberList["police"] != "112.112.112.112" {
			t.Errorf("expected police=112.112.112.112, got %q", cfg.MemberList["police"])
		}
		if cfg.MemberList["ad"] != "0.0.0.0" {
			t.Errorf("expected ad=0.0.0.0, got %q", cfg.MemberList["ad"])
		}
	})

	t.Run("spaces in list", func(t *testing.T) {
		os.Setenv("KG_IPPROTECT_ADMIN_IP", "10.0.0.1")
		os.Setenv("KG_IPPROTECT_LIST", " user1 : 1.2.3.4 , user2 : 5.6.7.8 ")

		cfg := LoadIPProtectionConfig()
		if cfg.MemberList["user1"] != "1.2.3.4" {
			t.Errorf("expected user1=1.2.3.4, got %q", cfg.MemberList["user1"])
		}
		if cfg.MemberList["user2"] != "5.6.7.8" {
			t.Errorf("expected user2=5.6.7.8, got %q", cfg.MemberList["user2"])
		}
	})
}

func setupTestRouter(cfg *IPProtectionConfig, userID string, level int) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.POST("/test", func(c *gin.Context) {
		// Simulate JWTAuth setting context values
		if userID != "" {
			c.Set("userID", userID)
		}
		if level > 0 {
			c.Set("level", level)
		}
		c.Next()
	}, IPProtection(cfg), func(c *gin.Context) {
		ip := GetClientIP(c)
		c.JSON(200, gin.H{"ip": ip})
	})

	w := httptest.NewRecorder()
	return router, w
}

func TestIPProtectionMiddleware(t *testing.T) {
	cfg := &IPProtectionConfig{
		AdminIP: "127.0.0.1",
		MemberList: map[string]string{
			"police": "112.112.112.112",
			"ad":     "0.0.0.0",
		},
	}

	t.Run("super admin gets admin IP", func(t *testing.T) {
		router, w := setupTestRouter(cfg, "admin1", 10)
		req, _ := http.NewRequest("POST", "/test", nil)
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		body := w.Body.String()
		if body != `{"ip":"127.0.0.1"}` {
			t.Errorf("expected admin IP, got %s", body)
		}
	})

	t.Run("designated member gets mapped IP", func(t *testing.T) {
		router, w := setupTestRouter(cfg, "police", 2)
		req, _ := http.NewRequest("POST", "/test", nil)
		router.ServeHTTP(w, req)

		body := w.Body.String()
		if body != `{"ip":"112.112.112.112"}` {
			t.Errorf("expected police IP, got %s", body)
		}
	})

	t.Run("normal user gets real IP", func(t *testing.T) {
		router, w := setupTestRouter(cfg, "normaluser", 2)
		req, _ := http.NewRequest("POST", "/test", nil)
		router.ServeHTTP(w, req)

		body := w.Body.String()
		// gin test uses empty RemoteAddr so ClientIP returns empty string
		// The important thing is that it does NOT return a protected IP
		if body == `{"ip":"127.0.0.1"}` || body == `{"ip":"112.112.112.112"}` || body == `{"ip":"0.0.0.0"}` {
			t.Errorf("normal user should not get a protected IP, got %s", body)
		}
	})

	t.Run("nil config passes through", func(t *testing.T) {
		router, w := setupTestRouter(nil, "admin1", 10)
		req, _ := http.NewRequest("POST", "/test", nil)
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("admin takes priority over member list", func(t *testing.T) {
		// User "police" is also level 10 → should get admin IP, not member IP
		router, w := setupTestRouter(cfg, "police", 10)
		req, _ := http.NewRequest("POST", "/test", nil)
		router.ServeHTTP(w, req)

		body := w.Body.String()
		if body != `{"ip":"127.0.0.1"}` {
			t.Errorf("admin should take priority, expected 127.0.0.1, got %s", body)
		}
	})
}

func TestGetClientIPFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Request.RemoteAddr = "192.168.1.100:12345"

	// No protected_ip set → should return ClientIP
	ip := GetClientIP(c)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", ip)
	}

	// Set protected_ip → should return it
	c.Set("protected_ip", "10.0.0.1")
	ip = GetClientIP(c)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}
