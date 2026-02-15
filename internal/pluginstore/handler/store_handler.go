package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/damoang/angple-backend/internal/pluginstore/service"
	"github.com/gin-gonic/gin"
)

// StoreHandler 플러그인 스토어 관리 핸들러
type StoreHandler struct {
	storeSvc   *service.StoreService
	catalogSvc *service.CatalogService
	manager    *plugin.Manager
}

// NewStoreHandler 생성자
func NewStoreHandler(
	storeSvc *service.StoreService,
	catalogSvc *service.CatalogService,
	manager *plugin.Manager,
) *StoreHandler {
	return &StoreHandler{
		storeSvc:   storeSvc,
		catalogSvc: catalogSvc,
		manager:    manager,
	}
}

// ListPlugins 카탈로그 목록 조회
// GET /api/v2/admin/plugins
func (h *StoreHandler) ListPlugins(c *gin.Context) {
	entries, err := h.catalogSvc.ListCatalog()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "CATALOG_ERROR", "message": "카탈로그 조회 실패", "details": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// GetPlugin 플러그인 상세 정보
// GET /api/v2/admin/plugins/:name
func (h *StoreHandler) GetPlugin(c *gin.Context) {
	name := c.Param("name")

	entry, err := h.catalogSvc.GetCatalogEntry(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "CATALOG_ERROR", "message": "조회 실패", "details": err.Error()},
		})
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{"code": "PLUGIN_NOT_FOUND", "message": "플러그인을 찾을 수 없습니다"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": entry})
}

// InstallPlugin 플러그인 설치
// POST /api/v2/admin/plugins/:name/install
func (h *StoreHandler) InstallPlugin(c *gin.Context) {
	name := c.Param("name")
	actorID := getActorID(c)

	if err := h.storeSvc.Install(name, actorID, h.manager); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "INSTALL_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "플러그인이 설치되었습니다", "plugin": name}})
}

// EnablePlugin 플러그인 활성화
// POST /api/v2/admin/plugins/:name/enable
func (h *StoreHandler) EnablePlugin(c *gin.Context) {
	name := c.Param("name")
	actorID := getActorID(c)

	if err := h.storeSvc.Enable(name, actorID, h.manager); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "ENABLE_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "플러그인이 활성화되었습니다", "plugin": name}})
}

// DisablePlugin 플러그인 비활성화
// POST /api/v2/admin/plugins/:name/disable
func (h *StoreHandler) DisablePlugin(c *gin.Context) {
	name := c.Param("name")
	actorID := getActorID(c)

	if err := h.storeSvc.Disable(name, actorID, h.manager); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "DISABLE_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "플러그인이 비활성화되었습니다", "plugin": name}})
}

// UninstallPlugin 플러그인 제거
// DELETE /api/v2/admin/plugins/:name
func (h *StoreHandler) UninstallPlugin(c *gin.Context) {
	name := c.Param("name")
	actorID := getActorID(c)

	if err := h.storeSvc.Uninstall(name, actorID, h.manager); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"code": "UNINSTALL_ERROR", "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "플러그인이 제거되었습니다", "plugin": name}})
}

// GetEvents 플러그인 이벤트 로그
// GET /api/v2/admin/plugins/:name/events
func (h *StoreHandler) GetEvents(c *gin.Context) {
	name := c.Param("name")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events, err := h.storeSvc.GetEvents(name, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "EVENT_ERROR", "message": "이벤트 조회 실패", "details": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": events})
}

// HealthCheck 플러그인 헬스 체크
// GET /api/v2/admin/plugins/health
func (h *StoreHandler) HealthCheck(c *gin.Context) {
	results := h.manager.CheckAllHealth()
	c.JSON(http.StatusOK, gin.H{"data": results})
}

// HealthCheckSingle 단일 플러그인 헬스 체크
// GET /api/v2/admin/plugins/:name/health
func (h *StoreHandler) HealthCheckSingle(c *gin.Context) {
	name := c.Param("name")
	result := h.manager.CheckHealth(name)
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Dashboard 플러그인 대시보드
// GET /api/v2/admin/plugins/dashboard
func (h *StoreHandler) Dashboard(c *gin.Context) {
	data, err := h.storeSvc.GetDashboard(h.manager)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"code": "DASHBOARD_ERROR", "message": err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ScheduledTasks 스케줄 작업 목록 조회
// GET /api/v2/admin/plugins/schedules
func (h *StoreHandler) ScheduledTasks(c *gin.Context) {
	tasks := h.manager.GetScheduledTasks()
	c.JSON(http.StatusOK, gin.H{"data": tasks})
}

// RateLimitConfigs 레이트 리밋 설정 목록 조회
// GET /api/v2/admin/plugins/rate-limits
func (h *StoreHandler) RateLimitConfigs(c *gin.Context) {
	configs := h.manager.GetRateLimitConfigs()
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

// PluginMetrics 전체 플러그인 메트릭 조회
// GET /api/v2/admin/plugins/metrics
func (h *StoreHandler) PluginMetrics(c *gin.Context) {
	metrics := h.manager.GetAllPluginMetrics()
	c.JSON(http.StatusOK, gin.H{"data": metrics})
}

// PluginMetricsSingle 단일 플러그인 메트릭 조회
// GET /api/v2/admin/plugins/:name/metrics
func (h *StoreHandler) PluginMetricsSingle(c *gin.Context) {
	name := c.Param("name")
	metrics := h.manager.GetPluginMetrics(name)
	c.JSON(http.StatusOK, gin.H{"data": metrics})
}

// EventSubscriptions 이벤트 구독 현황 조회
// GET /api/v2/admin/plugins/event-subscriptions
func (h *StoreHandler) EventSubscriptions(c *gin.Context) {
	subs := h.manager.GetEventSubscriptions()
	c.JSON(http.StatusOK, gin.H{"data": subs})
}

// PluginOverview 플러그인 전체 현황 조회
// GET /api/v2/admin/plugins/overview
func (h *StoreHandler) PluginOverview(c *gin.Context) {
	overview := h.manager.GetOverview()
	c.JSON(http.StatusOK, gin.H{"data": overview})
}

// PluginDetail 플러그인 상세 정보 (capabilities 포함)
// GET /api/v2/admin/plugins/:name/detail
func (h *StoreHandler) PluginDetail(c *gin.Context) {
	name := c.Param("name")
	detail := h.manager.GetDetail(name)
	if detail == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{"code": "PLUGIN_NOT_FOUND", "message": "플러그인을 찾을 수 없습니다"},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

// getActorID 요청에서 사용자 ID 추출
func getActorID(c *gin.Context) string {
	if userID, exists := c.Get("userID"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return "unknown"
}
