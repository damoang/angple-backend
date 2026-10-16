package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// setInternalSecrets swaps the package-level secrets for one test.
func setInternalSecrets(t *testing.T, current, previous string) {
	t.Helper()
	prevCurrent, prevPrevious := internalSharedSecret, internalSharedSecretPrevious
	internalSharedSecret, internalSharedSecretPrevious = current, previous
	t.Cleanup(func() {
		internalSharedSecret, internalSharedSecretPrevious = prevCurrent, prevPrevious
	})
}

func TestHasValidInternalSecret(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		previous string
		header   string
		want     bool
	}{
		{"current matches", "cur-secret", "", "cur-secret", true},
		{"previous matches", "cur-secret", "old-secret", "old-secret", true},
		{"current matches with previous set", "cur-secret", "old-secret", "cur-secret", true},
		{"empty previous is ignored", "cur-secret", "", "old-secret", false},
		{"neither matches", "cur-secret", "old-secret", "wrong", false},
		{"empty header rejected", "cur-secret", "old-secret", "", false},
		{"no secrets configured rejects everything", "", "", "anything", false},
		{"no secrets configured rejects empty header", "", "", "", false},
		{"previous only still works", "", "old-secret", "old-secret", true},
		{"prefix of secret rejected", "cur-secret", "", "cur", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setInternalSecrets(t, tt.current, tt.previous)
			if got := hasValidInternalSecret(tt.header); got != tt.want {
				t.Errorf("hasValidInternalSecret(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestRequireInternalCron_Secrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setInternalSecrets(t, "cur-secret", "old-secret")

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"current secret", "cur-secret", http.StatusOK},
		{"previous secret", "old-secret", http.StatusOK},
		{"wrong secret", "wrong", http.StatusForbidden},
		{"missing header", "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RequireInternalCron())
			r.GET("/cron", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/cron", nil)
			req.RemoteAddr = "10.0.0.5:12345"
			if tt.header != "" {
				req.Header.Set("X-Internal-Secret", tt.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestRequireInternalCron_LocalhostAllowedWithoutSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setInternalSecrets(t, "", "")

	r := gin.New()
	r.Use(RequireInternalCron())
	r.GET("/cron", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/cron", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
