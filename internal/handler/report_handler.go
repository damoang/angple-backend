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

// ReportHandler handles report requests
type ReportHandler struct {
	service *service.ReportService
}

// NewReportHandler creates a new ReportHandler
func NewReportHandler(service *service.ReportService) *ReportHandler {
	return &ReportHandler{service: service}
}

// ListReports handles GET /api/v2/reports
// @Summary 신고 목록 조회
// @Description 신고 목록을 조회합니다 (관리자 전용)
// @Tags reports
// @Produce json
// @Param status query string false "상태 필터 (pending, monitoring, approved, dismissed)"
// @Param page query int false "페이지 번호"
// @Param limit query int false "페이지당 항목 수"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports [get]
func (h *ReportHandler) ListReports(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Parse query parameters
	status := c.Query("status")
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil {
		limit = 20
	}

	// Get reports
	reports, total, err := h.service.List(status, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "신고 목록 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: reports,
		Meta: &common.Meta{
			Page:  page,
			Limit: limit,
			Total: total,
		},
	})
}

// GetReportData handles GET /api/v2/reports/data
// @Summary 신고 데이터 조회
// @Description 신고 상세 데이터를 조회합니다 (관리자 전용)
// @Tags reports
// @Produce json
// @Param sg_table query string true "신고 테이블"
// @Param sg_id query int true "신고 ID"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/data [get]
func (h *ReportHandler) GetReportData(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Parse query parameters
	table := c.Query("sg_table")
	id, err := strconv.Atoi(c.Query("sg_id"))
	if err != nil || table == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", nil)
		return
	}

	// Get report data
	report, err := h.service.GetData(table, id)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"success": true,
			"data":    report,
		},
	})
}

// GetRecentReports handles GET /api/v2/reports/recent
// @Summary 최근 신고 목록 조회
// @Description 최근 신고 목록을 조회합니다 (관리자 전용)
// @Tags reports
// @Produce json
// @Param limit query int false "조회할 개수"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/recent [get]
func (h *ReportHandler) GetRecentReports(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Parse query parameter
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil {
		limit = 10
	}

	// Get recent reports
	reports, err := h.service.GetRecent(limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "신고 목록 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: reports,
	})
}

// ProcessReport handles POST /api/v2/reports/process
// @Summary 신고 처리
// @Description 신고를 처리합니다 (관리자 전용)
// @Tags reports
// @Accept json
// @Produce json
// @Param request body domain.ReportActionRequest true "신고 처리 요청"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/process [post]
func (h *ReportHandler) ProcessReport(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	adminID := middleware.GetDamoangUserID(c)
	clientIP := c.ClientIP()

	var req domain.ReportActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// Process report
	if err := h.service.Process(adminID, clientIP, &req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"success": true,
			"message": "처리되었습니다",
		},
	})
}

// SubmitReport handles POST /api/v2/reports
// @Summary 신고 접수
// @Tags reports
// @Accept json
// @Produce json
// @Param request body domain.SubmitReportRequest true "신고 내용"
// @Success 200 {object} common.APIResponse
// @Router /reports [post]
func (h *ReportHandler) SubmitReport(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	var req domain.SubmitReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	report, err := h.service.Create(userID, req.TargetID, req.Table, req.PostID, req.Reason)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: domain.ReportListResponse{
			ID:         report.ID,
			Table:      report.Table,
			Parent:     report.Parent,
			ReporterID: report.ReporterID,
			TargetID:   report.TargetID,
			Reason:     report.Reason,
			Status:     report.Status(),
			CreatedAt:  report.CreatedAt.Format("2006-01-02 15:04:05"),
		},
	})
}

// MyReports handles GET /api/v2/reports/mine
// @Summary 내 신고 내역
// @Tags reports
// @Produce json
// @Param limit query int false "조회 개수 (기본 20)"
// @Success 200 {object} common.APIResponse{data=[]domain.ReportListResponse}
// @Router /reports/mine [get]
func (h *ReportHandler) MyReports(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}

	reports, err := h.service.GetMyReports(userID, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "신고 내역 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: reports})
}

// GetStats handles GET /api/v2/reports/stats
// @Summary 신고 통계 조회
// @Description 신고 통계를 조회합니다 (관리자 전용)
// @Tags reports
// @Produce json
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/stats [get]
func (h *ReportHandler) GetStats(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	// Check admin permission
	level := middleware.GetDamoangUserLevel(c)
	if level < 10 {
		common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
		return
	}

	// Get stats
	stats, err := h.service.GetStats()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "통계 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: stats,
	})
}
