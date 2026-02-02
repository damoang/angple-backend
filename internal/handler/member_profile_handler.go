package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MemberProfileHandler handles member profile HTTP requests
type MemberProfileHandler struct {
	service service.MemberProfileService
}

// NewMemberProfileHandler creates a new MemberProfileHandler
func NewMemberProfileHandler(service service.MemberProfileService) *MemberProfileHandler {
	return &MemberProfileHandler{service: service}
}

// GetProfile handles GET /api/v2/members/:user_id
// @Summary 회원 프로필 조회
// @Description 회원 ID로 공개 프로필 정보를 조회합니다
// @Tags members
// @Produce json
// @Param user_id path string true "회원 ID"
// @Success 200 {object} common.APIResponse{data=domain.MemberProfileResponse}
// @Failure 404 {object} common.APIResponse
// @Router /members/{user_id} [get]
func (h *MemberProfileHandler) GetProfile(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "회원 ID를 입력해 주세요", nil)
		return
	}

	profile, err := h.service.GetProfile(userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: profile})
}

// GetPosts handles GET /api/v2/members/:user_id/posts
// @Summary 회원 작성글 조회
// @Description 회원의 최근 작성글 목록을 조회합니다 (최근 5개)
// @Tags members
// @Produce json
// @Param user_id path string true "회원 ID"
// @Param limit query int false "조회 개수 (기본 5, 최대 20)"
// @Success 200 {object} common.APIResponse{data=[]domain.MemberPostSummary}
// @Failure 404 {object} common.APIResponse
// @Router /members/{user_id}/posts [get]
func (h *MemberProfileHandler) GetPosts(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "회원 ID를 입력해 주세요", nil)
		return
	}

	limit := 5
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	posts, err := h.service.GetRecentPosts(userID, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "작성글 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: posts})
}

// GetComments handles GET /api/v2/members/:user_id/comments
// @Summary 회원 작성댓글 조회
// @Description 회원의 최근 작성댓글 목록을 조회합니다 (최근 5개)
// @Tags members
// @Produce json
// @Param user_id path string true "회원 ID"
// @Param limit query int false "조회 개수 (기본 5, 최대 20)"
// @Success 200 {object} common.APIResponse{data=[]domain.MemberCommentSummary}
// @Failure 404 {object} common.APIResponse
// @Router /members/{user_id}/comments [get]
func (h *MemberProfileHandler) GetComments(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "회원 ID를 입력해 주세요", nil)
		return
	}

	limit := 5
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	comments, err := h.service.GetRecentComments(userID, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "작성댓글 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: comments})
}

// GetPointHistory handles GET /api/v2/members/:user_id/points/history
// @Summary 포인트 내역 조회
// @Description 본인의 포인트 내역을 조회합니다 (본인만 조회 가능)
// @Tags members
// @Produce json
// @Param user_id path string true "회원 ID"
// @Param limit query int false "조회 개수 (기본 20, 최대 50)"
// @Success 200 {object} common.APIResponse{data=[]domain.PointHistory}
// @Failure 401 {object} common.APIResponse
// @Failure 403 {object} common.APIResponse
// @Router /members/{user_id}/points/history [get]
func (h *MemberProfileHandler) GetPointHistory(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "회원 ID를 입력해 주세요", nil)
		return
	}

	// 본인만 조회 가능
	currentUserID := middleware.GetDamoangUserID(c)
	if currentUserID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}
	if currentUserID != userID {
		common.ErrorResponse(c, http.StatusForbidden, "본인의 포인트 내역만 조회할 수 있습니다", nil)
		return
	}

	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	history, err := h.service.GetPointHistory(userID, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "포인트 내역 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: history})
}
