package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ReportHandler handles report requests
type ReportHandler struct {
	service       *service.ReportService
	singoUserRepo *repository.SingoUserRepository
}

// NewReportHandler creates a new ReportHandler
func NewReportHandler(service *service.ReportService) *ReportHandler {
	return &ReportHandler{service: service}
}

// SetSingoUserRepo sets the singo user repository for role-based access control
func (h *ReportHandler) SetSingoUserRepo(repo *repository.SingoUserRepository) {
	h.singoUserRepo = repo
}

// findSingoUser — JWT user_id로 singo_users 조회 (mb_id → v2_users.id → g5_member.mb_no)
func (h *ReportHandler) findSingoUser(userID string) *domain.SingoUser {
	if userID == "" || h.singoUserRepo == nil {
		return nil
	}
	user, err := h.singoUserRepo.FindByMbID(userID)
	if err != nil {
		user, err = h.singoUserRepo.FindByV2UserID(userID)
		if err != nil {
			user, err = h.singoUserRepo.FindByMbNo(userID)
			if err != nil {
				return nil
			}
		}
	}
	return user
}

// getSingoRole returns the singo role for the authenticated user
func (h *ReportHandler) getSingoRole(c *gin.Context) (string, string) {
	userID := middleware.GetV2UserID(c)
	user := h.findSingoUser(userID)
	if user == nil {
		return userID, ""
	}
	return userID, user.Role
}

// requireSingoAccess checks if the user is registered in singo_users (any role)
// Falls back to level >= 10 check if singoUserRepo is not configured
func (h *ReportHandler) requireSingoAccess(c *gin.Context) bool {
	userID := middleware.GetV2UserID(c)
	if h.singoUserRepo != nil {
		if h.findSingoUser(userID) != nil {
			return true
		}
	}
	// 레거시 폴백: singo_users 미설정 시 level 체크
	if middleware.GetDamoangUserLevel(c) >= 10 {
		return true
	}
	common.ErrorResponse(c, http.StatusForbidden, "관리자 권한이 필요합니다", nil)
	return false
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
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	// Parse query parameters
	status := c.Query("status")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")
	sort := c.DefaultQuery("sort", "newest")
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil {
		limit = 20
	}
	minOpinions, _ := strconv.Atoi(c.DefaultQuery("min_opinions", "0"))
	groupBy := c.DefaultQuery("group_by", "content") // content (기본) or target
	excludeReviewer := c.Query("exclude_reviewer")

	// Get singo role for reviewer info visibility + requesting user ID
	requestingUserID, singoRole := h.getSingoRole(c)

	// group_by=target: 피신고자별 그룹핑
	if groupBy == "target" {
		reports, total, err := h.service.ListByTarget(status, page, limit, fromDate, toDate, sort, singoRole, excludeReviewer)
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
		return
	}

	// Default: group_by=content (기존 방식)
	reports, total, err := h.service.List(status, page, limit, fromDate, toDate, sort, singoRole, minOpinions, excludeReviewer, requestingUserID)
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
// @Description 신고 상세 데이터를 조회합니다 (관리자 전용). Phase 2: ?include=ai,history 파라미터 지원
// @Tags reports
// @Produce json
// @Param sg_table query string true "신고 테이블"
// @Param sg_id query int false "신고 ID (특정 신고 조회 시)"
// @Param sg_parent query int false "신고 대상 게시글 ID (sg_id 없을 때 필수, 레거시 호환)"
// @Param include query string false "포함할 데이터 (ai,history) - 쉼표로 구분"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/data [get]
func (h *ReportHandler) GetReportData(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	// Parse query parameters
	table := c.Query("sg_table")
	sgID, _ := strconv.Atoi(c.Query("sg_id"))
	parent, err := strconv.Atoi(c.Query("sg_parent"))

	// 하위 호환성: sg_id 없으면 sg_parent 필수 (레거시)
	if table == "" || (sgID == 0 && (err != nil || parent == 0)) {
		common.ErrorResponse(c, http.StatusBadRequest, "sg_table과 sg_id 또는 sg_parent가 필요합니다", nil)
		return
	}

	// sg_id 없으면 parent 사용 (레거시 호환)
	if sgID == 0 && parent > 0 {
		// parent만 있는 경우: 기존 동작 유지
	}

	// Singo 역할 기반 닉네임 마스킹
	userID, singoRole := h.getSingoRole(c)

	// Phase 2: include 파라미터 파싱
	includeParam := c.Query("include")
	var includes []string
	if includeParam != "" {
		includes = strings.Split(includeParam, ",")
		// Trim whitespace from each include
		for i := range includes {
			includes[i] = strings.TrimSpace(includes[i])
		}
	}

	// Get report data (enhanced if includes specified)
	if len(includes) > 0 {
		// Phase 2: 통합 API - AI 평가 + 징계 이력 포함
		var enhanced *domain.ReportDetailEnhancedResponse
		var err error
		if sgID > 0 {
			enhanced, err = h.service.GetDataEnhanced(table, parent, userID, singoRole, includes, sgID)
		} else {
			enhanced, err = h.service.GetDataEnhanced(table, parent, userID, singoRole, includes)
		}
		if err != nil {
			common.ErrorResponse(c, http.StatusNotFound, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, common.APIResponse{
			Data: gin.H{
				"success":            true,
				"data":               enhanced.Report,
				"all_reports":        enhanced.AllReports,
				"opinions":           enhanced.Opinions,
				"status":             enhanced.Status,
				"process_result":     enhanced.ProcessResult,
				"ai_evaluations":     enhanced.AIEvaluations,
				"discipline_history": enhanced.DisciplineHistory,
			},
		})
	} else {
		// 기존 API (하위 호환성 유지)
		var detail *domain.ReportDetailResponse
		var err error
		if sgID > 0 {
			detail, err = h.service.GetData(table, parent, userID, singoRole, sgID)
		} else {
			detail, err = h.service.GetData(table, parent, userID, singoRole)
		}
		if err != nil {
			common.ErrorResponse(c, http.StatusNotFound, err.Error(), nil)
			return
		}

		c.JSON(http.StatusOK, common.APIResponse{
			Data: gin.H{
				"success":        true,
				"data":           detail.Report,
				"all_reports":    detail.AllReports,
				"opinions":       detail.Opinions,
				"status":         detail.Status,
				"process_result": detail.ProcessResult,
			},
		})
	}
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
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
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
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	adminID := middleware.GetV2UserID(c)
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

// GetOpinions handles GET /api/v1/reports/opinions
// @Summary 특정 신고의 의견 목록
// @Tags reports
// @Produce json
// @Param sg_table query string true "신고 테이블"
// @Param sg_id query int false "신고 ID"
// @Param sg_parent query int true "신고 대상 게시글"
// @Success 200 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/opinions [get]
func (h *ReportHandler) GetOpinions(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	table := c.Query("sg_table")
	sgID, _ := strconv.Atoi(c.Query("sg_id"))
	parent, err := strconv.Atoi(c.Query("sg_parent"))
	if err != nil || table == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", nil)
		return
	}

	userID, singoRole := h.getSingoRole(c)
	opinions, err := h.service.GetOpinions(table, sgID, parent, userID, singoRole)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "의견 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: opinions,
	})
}

// BatchProcessReport handles POST /api/v2/reports/batch-process
// @Summary 신고 일괄 처리
// @Description 여러 신고를 일괄 처리합니다 (관리자 전용)
// @Tags reports
// @Accept json
// @Produce json
// @Param request body domain.BatchReportActionRequest true "일괄 처리 요청"
// @Success 200 {object} common.APIResponse
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Security BearerAuth
// @Router /reports/batch-process [post]
func (h *ReportHandler) BatchProcessReport(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	adminID := middleware.GetV2UserID(c)
	clientIP := c.ClientIP()

	var req domain.BatchReportActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if len(req.Tables) != len(req.Parents) {
		common.ErrorResponse(c, http.StatusBadRequest, "tables와 parents의 길이가 일치하지 않습니다", nil)
		return
	}

	// Immediate batch approve: 같은 피신고자끼리 묶어서 1개 징계로그 생성
	if req.Immediate && req.Action == "adminApprove" {
		result, err := h.service.ProcessBatchImmediate(adminID, clientIP, &req)
		if err != nil {
			common.ErrorResponse(c, http.StatusInternalServerError, err.Error(), err)
			return
		}
		c.JSON(http.StatusOK, common.APIResponse{
			Data: gin.H{
				"success":   true,
				"processed": result.Processed,
				"failed":    result.Failed,
				"errors":    result.Errors,
			},
		})
		return
	}

	// 기존 for-loop (dismiss, opinion, non-immediate 등)
	processed := 0
	failed := 0
	var errors []string

	for i := range req.Tables {
		actionReq := &domain.ReportActionRequest{
			Action:         req.Action,
			Table:          req.Tables[i],
			Parent:         req.Parents[i],
			AdminMemo:      req.AdminMemo,
			PenaltyDays:    req.PenaltyDays,
			PenaltyType:    req.PenaltyType,
			PenaltyReasons: req.PenaltyReasons,
			Immediate:      req.Immediate,
		}

		if err := h.service.Process(adminID, clientIP, actionReq); err != nil {
			failed++
			errors = append(errors, err.Error())
		} else {
			processed++
		}
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"success":   true,
			"processed": processed,
			"failed":    failed,
			"errors":    errors,
		},
	})
}

// ListSingoUsers handles GET /api/v1/singo-users
// Returns the list of singo reviewers (super_admin: full info, admin: count only)
func (h *ReportHandler) ListSingoUsers(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	if h.singoUserRepo == nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "singo_users 미설정", nil)
		return
	}

	users, err := h.singoUserRepo.FindAll()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "검토자 목록 조회 실패", err)
		return
	}

	_, singoRole := h.getSingoRole(c)

	if singoRole == "super_admin" {
		// Return full user list
		c.JSON(http.StatusOK, common.APIResponse{
			Data: users,
		})
	} else {
		// Return only count for non-super_admin
		c.JSON(http.StatusOK, common.APIResponse{
			Data: gin.H{
				"total": len(users),
			},
		})
	}
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
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
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

// GetAdjacentReport handles GET /api/v1/reports/adjacent
// 인접 신고 조회 (이전/다음 네비게이션)
func (h *ReportHandler) GetAdjacentReport(c *gin.Context) {
	if !middleware.IsDamoangAuthenticated(c) {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if !h.requireSingoAccess(c) {
		return
	}

	table := c.Query("sg_table")
	sgID, err := strconv.Atoi(c.Query("sg_id"))
	if err != nil || table == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "sg_table과 sg_id가 필요합니다", nil)
		return
	}

	direction := c.Query("direction")
	if direction != "prev" && direction != "next" {
		common.ErrorResponse(c, http.StatusBadRequest, "direction은 prev 또는 next여야 합니다", nil)
		return
	}

	status := c.Query("status")
	sort := c.DefaultQuery("sort", "newest")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")
	search := c.Query("search")

	report, err := h.service.GetAdjacentReport(table, sgID, direction, status, sort, fromDate, toDate, search)
	if err != nil {
		// 인접 신고 없음 → null 반환
		c.JSON(http.StatusOK, common.APIResponse{
			Data: nil,
		})
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: gin.H{
			"sg_id":  report.SGID,
			"table":  report.Table,
			"parent": report.Parent,
			"type":   report.Type,
		},
	})
}
