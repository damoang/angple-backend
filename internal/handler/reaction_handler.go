package handler

import (
	"fmt"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ReactionHandler handles reaction requests
type ReactionHandler struct {
	service *service.ReactionService
}

// NewReactionHandler creates a new ReactionHandler
func NewReactionHandler(service *service.ReactionService) *ReactionHandler {
	return &ReactionHandler{service: service}
}

// React handles POST /api/v2/boards/:board_id/posts/:id/reactions
// @Summary 게시글 반응 추가/제거
// @Description 게시글에 반응(좋아요, 하트 등)을 추가하거나 제거합니다
// @Tags posts
// @Accept json
// @Produce json
// @Param board_id path string true "게시판 ID"
// @Param id path string true "게시글 ID"
// @Param request body domain.ReactionRequest true "반응 요청"
// @Success 200 {object} domain.ReactionResponse
// @Failure 401 {object} domain.ReactionResponse
// @Failure 400 {object} domain.ReactionResponse
// @Security BearerAuth
// @Router /boards/{board_id}/posts/{id}/reactions [post]
func (h *ReactionHandler) React(c *gin.Context) {
	// Check authentication
	if !middleware.IsDamoangAuthenticated(c) {
		c.JSON(http.StatusOK, domain.ReactionResponse{
			Status:  "error",
			Message: "로그인이 필요합니다",
		})
		return
	}

	memberID := middleware.GetDamoangUserID(c)
	boardID := c.Param("board_id")
	postID := c.Param("id")

	var req domain.ReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, domain.ReactionResponse{
			Status:  "error",
			Message: "요청 형식이 올바르지 않습니다",
		})
		return
	}

	// Validate request
	if req.ReactionMode != "add" && req.ReactionMode != "remove" {
		req.ReactionMode = "add"
	}

	if req.Reaction == "" {
		c.JSON(http.StatusOK, domain.ReactionResponse{
			Status:  "error",
			Message: "반응 유형을 선택해주세요",
		})
		return
	}

	// Generate target ID from path if not in body
	// Format: comment:{board_id}:{comment_id} or post:{board_id}:{post_id}
	if req.TargetID == "" {
		req.TargetID = fmt.Sprintf("comment:%s:%s", boardID, postID)
	}

	// Generate parent ID if not set
	if req.ParentID == "" {
		req.ParentID = fmt.Sprintf("document:%s:%s", boardID, postID)
	}

	// Get client IP
	ip := c.ClientIP()

	// React
	result, err := h.service.React(memberID, &req, ip)
	if err != nil {
		c.JSON(http.StatusOK, domain.ReactionResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, domain.ReactionResponse{
		Status: "success",
		Result: result,
	})
}

// GetReactions handles GET /api/v2/boards/:board_id/posts/:id/reactions
// @Summary 게시글 반응 조회
// @Description 게시글의 반응 목록을 조회합니다
// @Tags posts
// @Produce json
// @Param board_id path string true "게시판 ID"
// @Param id path string true "게시글 ID"
// @Success 200 {object} common.APIResponse
// @Router /boards/{board_id}/posts/{id}/reactions [get]
func (h *ReactionHandler) GetReactions(c *gin.Context) {
	boardID := c.Param("board_id")
	postID := c.Param("id")

	// Get member ID if authenticated
	memberID := ""
	if middleware.IsDamoangAuthenticated(c) {
		memberID = middleware.GetDamoangUserID(c)
	}

	// Generate parent ID
	parentID := fmt.Sprintf("document:%s:%s", boardID, postID)

	// Get reactions by parent (all comments in the post)
	result, err := h.service.GetReactionsByParent(parentID, memberID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "반응 조회 중 오류가 발생했습니다", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{
		Data: result,
	})
}
