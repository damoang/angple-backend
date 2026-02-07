package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// AIEvaluationHandler handles AI evaluation requests
type AIEvaluationHandler struct {
	service *service.AIEvaluationService
}

// NewAIEvaluationHandler creates a new AIEvaluationHandler
func NewAIEvaluationHandler(service *service.AIEvaluationService) *AIEvaluationHandler {
	return &AIEvaluationHandler{service: service}
}

// SaveEvaluation handles POST /api/v1/reports/ai-evaluation
// @Summary AI 평가 결과 저장
// @Description AI 평가 결과를 저장합니다 (관리자 전용)
// @Tags ai-evaluation
// @Accept json
// @Produce json
// @Param request body domain.SaveAIEvaluationRequest true "AI 평가 결과"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/ai-evaluation [post]
func (h *AIEvaluationHandler) SaveEvaluation(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	adminID := middleware.GetDamoangUserID(c)

	var req domain.SaveAIEvaluationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	eval, err := h.service.Save(adminID, &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "AI 평가 저장 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: eval,
	})
}

// GetEvaluation handles GET /api/v1/reports/ai-evaluation
// @Summary AI 평가 결과 조회
// @Description 신고에 대한 최신 AI 평가 결과를 조회합니다 (관리자 전용)
// @Tags ai-evaluation
// @Produce json
// @Param sg_table query string true "게시판 테이블명"
// @Param sg_parent query int true "게시글 번호"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/ai-evaluation [get]
func (h *AIEvaluationHandler) GetEvaluation(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	table := c.Query("sg_table")
	parent, err := strconv.Atoi(c.Query("sg_parent"))
	if err != nil || table == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", nil)
		return
	}

	eval, err := h.service.GetByReport(table, parent)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: eval,
	})
}
