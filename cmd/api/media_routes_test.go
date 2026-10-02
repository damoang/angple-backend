package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/damoang/angple-backend/internal/handler"
	"github.com/gin-gonic/gin"
)

// TestMediaUploadRoutesAdminOnly: 미디어 업로드 세 경로는 관리자 전용이다.
// 관리자 아님(level 없음·일반 회원)이면 핸들러에 닿기 전에 403 으로 끝나야 한다.
// 핸들러에 서비스가 없어(nil) 닿으면 패닉이 나므로, 403 이 곧 차단의 증거다.
func TestMediaUploadRoutesAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, level := range []int{0, 2, 9} {
		router := gin.New()
		lv := level
		media := router.Group("/api/v2/media", func(c *gin.Context) {
			if lv > 0 {
				c.Set("level", lv)
			}
			c.Next()
		})
		registerMediaUploadRoutes(media, handler.NewMediaHandler(nil))

		for _, p := range []string{"/api/v2/media/images", "/api/v2/media/attachments", "/api/v2/media/videos"} {
			req := httptest.NewRequest(http.MethodPost, p, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("level=%d POST %s = %d, want 403", lv, p, w.Code)
			}
		}
	}
}

// TestMediaUploadRoutesAdminPasses: 관리자(level 10)는 관리자 검사를 통과해 핸들러에 닿는다.
// 파일이 없으므로 핸들러가 400 을 돌려준다(서비스 호출 전 단계).
func TestMediaUploadRoutesAdminPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	media := router.Group("/api/v2/media", func(c *gin.Context) {
		c.Set("level", 10)
		c.Next()
	})
	registerMediaUploadRoutes(media, handler.NewMediaHandler(nil))

	for _, p := range []string{"/api/v2/media/images", "/api/v2/media/attachments", "/api/v2/media/videos"} {
		req := httptest.NewRequest(http.MethodPost, p, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("admin POST %s = %d, want 400 (파일 없음)", p, w.Code)
		}
	}
}
