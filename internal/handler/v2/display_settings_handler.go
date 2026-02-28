package v2

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// DisplaySettingsHandler handles board display settings endpoints
type DisplaySettingsHandler struct {
	boardRepo           v2repo.BoardRepository
	displaySettingsRepo v2repo.BoardDisplaySettingsRepository
}

// NewDisplaySettingsHandler creates a new DisplaySettingsHandler
func NewDisplaySettingsHandler(
	boardRepo v2repo.BoardRepository,
	displaySettingsRepo v2repo.BoardDisplaySettingsRepository,
) *DisplaySettingsHandler {
	return &DisplaySettingsHandler{
		boardRepo:           boardRepo,
		displaySettingsRepo: displaySettingsRepo,
	}
}

// GetDisplaySettings handles GET /api/v1/boards/:slug/display-settings
func (h *DisplaySettingsHandler) GetDisplaySettings(c *gin.Context) {
	slug := c.Param("slug")

	// Get display settings directly by slug (board_id is the slug in this table)
	settings, err := h.displaySettingsRepo.FindByBoardSlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회 실패", err)
		return
	}

	common.V2Success(c, settings.ToResponse())
}

// UpdateDisplaySettings handles PUT /api/v1/boards/:slug/display-settings
func (h *DisplaySettingsHandler) UpdateDisplaySettings(c *gin.Context) {
	slug := c.Param("slug")

	// Check admin permission
	userLevel := middleware.GetUserLevel(c)
	if userLevel < 10 {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Verify board exists
	_, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	// Parse request
	var req v2domain.UpdateDisplaySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// Get existing settings or create default
	settings, err := h.displaySettingsRepo.FindByBoardSlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회 실패", err)
		return
	}

	// Update fields if provided
	if req.ListLayout != nil {
		settings.ListLayout = *req.ListLayout
	}
	if req.ViewLayout != nil {
		settings.ViewLayout = *req.ViewLayout
	}
	if req.ShowPreview != nil {
		settings.ShowPreview = *req.ShowPreview
	}
	if req.PreviewLength != nil {
		settings.PreviewLength = *req.PreviewLength
	}
	if req.ShowThumbnail != nil {
		settings.ShowThumbnail = *req.ShowThumbnail
	}

	// Ensure board_id (slug) is set
	settings.BoardID = slug

	// Save settings
	if err := h.displaySettingsRepo.Upsert(settings); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 저장 실패", err)
		return
	}

	common.V2Success(c, settings.ToResponse())
}
