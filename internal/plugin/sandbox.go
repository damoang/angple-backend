package plugin

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

// SandboxConfig 플러그인 샌드박스 설정
type SandboxConfig struct {
	RequestTimeout time.Duration // API 요청 타임아웃 (기본 30초)
	RecoverPanics  bool          // 패닉 복구 여부 (기본 true)
}

// DefaultSandboxConfig 기본 샌드박스 설정
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		RequestTimeout: 30 * time.Second,
		RecoverPanics:  true,
	}
}

// SandboxMiddleware 플러그인 라우트용 샌드박스 미들웨어
// 패닉 복구 + 타임아웃 제한으로 플러그인이 시스템을 불안정하게 만드는 것을 방지
func SandboxMiddleware(pluginName string, cfg SandboxConfig, logger Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 패닉 복구
		if cfg.RecoverPanics {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					logger.Error("Plugin %s panicked: %v\n%s", pluginName, r, stack)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error": gin.H{
							"code":    "PLUGIN_PANIC",
							"message": fmt.Sprintf("플러그인 %s에서 내부 오류가 발생했습니다", pluginName),
						},
					})
				}
			}()
		}

		// 타임아웃 컨텍스트
		if cfg.RequestTimeout > 0 {
			ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.RequestTimeout)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()

		// 타임아웃 체크
		if c.Request.Context().Err() == context.DeadlineExceeded {
			logger.Warn("Plugin %s request timeout: %s %s", pluginName, c.Request.Method, c.Request.URL.Path)
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"error": gin.H{
						"code":    "PLUGIN_TIMEOUT",
						"message": fmt.Sprintf("플러그인 %s 요청 시간이 초과되었습니다", pluginName),
					},
				})
			}
		}
	}
}

// SafeCall 플러그인 함수를 안전하게 호출 (패닉 복구)
func SafeCall(pluginName string, logger Logger, fn func() error) error {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				logger.Error("Plugin %s panicked during call: %v\n%s", pluginName, r, stack)
				err = fmt.Errorf("plugin %s panicked: %v", pluginName, r)
			}
		}()
		err = fn()
	}()
	return err
}
