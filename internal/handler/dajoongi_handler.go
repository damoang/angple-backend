package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/gin-gonic/gin"
)

// DajoongiHandler handles dajoongi (duplicate account detection) requests
type DajoongiHandler struct {
	repo *repository.DajoongiRepository
}

// NewDajoongiHandler creates a new DajoongiHandler
func NewDajoongiHandler(repo *repository.DajoongiRepository) *DajoongiHandler {
	return &DajoongiHandler{repo: repo}
}

// GetDuplicateAccounts handles GET /api/v2/dajoongi
// @Summary 다중이 목록 조회
// @Description 동일 IP에서 여러 계정으로 활동한 기록을 조회합니다 (관리자 전용)
// @Tags admin
// @Produce json
// @Param days query int false "조회 기간 (일)" default(3)
// @Success 200 {object} common.APIResponse{data=domain.DajoongiResponse}
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /dajoongi [get]
func (h *DajoongiHandler) GetDuplicateAccounts(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission (level >= 10)
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Parse days parameter (default: 3)
	days := 3
	if daysParam := c.Query("days"); daysParam != "" {
		if parsed, err := strconv.Atoi(daysParam); err == nil && parsed > 0 && parsed <= 30 {
			days = parsed
		}
	}

	// Get duplicate accounts
	items, err := h.repo.GetDuplicateAccounts(days)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.DajoongiResponse{
			Date:  time.Now().Format("2006-01-02 15:04:05"),
			Items: items,
			Total: len(items),
		},
	})
}
