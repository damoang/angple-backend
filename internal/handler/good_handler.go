package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// GoodHandler handles recommend/downvote HTTP requests
type GoodHandler struct {
	service service.GoodService
}

// NewGoodHandler creates a new GoodHandler
func NewGoodHandler(service service.GoodService) *GoodHandler {
	return &GoodHandler{service: service}
}

// RecommendPost handles POST /boards/:board_id/posts/:id/recommend
func (h *GoodHandler) RecommendPost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	ip := c.ClientIP()
	result, err := h.service.RecommendPost(boardID, wrID, userID, ip)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// CancelRecommendPost handles DELETE /boards/:board_id/posts/:id/recommend
func (h *GoodHandler) CancelRecommendPost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	result, err := h.service.CancelRecommendPost(boardID, wrID, userID)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// DownvotePost handles POST /boards/:board_id/posts/:id/downvote
func (h *GoodHandler) DownvotePost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	ip := c.ClientIP()
	result, err := h.service.DownvotePost(boardID, wrID, userID, ip)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// CancelDownvotePost handles DELETE /boards/:board_id/posts/:id/downvote
func (h *GoodHandler) CancelDownvotePost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	result, err := h.service.CancelDownvotePost(boardID, wrID, userID)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// RecommendComment handles POST /boards/:board_id/posts/:id/comments/:comment_id/recommend
func (h *GoodHandler) RecommendComment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	commentID, err := strconv.Atoi(c.Param("comment_id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID입니다", err)
		return
	}

	ip := c.ClientIP()
	result, err := h.service.RecommendComment(boardID, commentID, userID, ip)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// CancelRecommendComment handles DELETE /boards/:board_id/posts/:id/comments/:comment_id/recommend
func (h *GoodHandler) CancelRecommendComment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	commentID, err := strconv.Atoi(c.Param("comment_id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID입니다", err)
		return
	}

	result, err := h.service.CancelRecommendComment(boardID, commentID, userID)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// LikePost handles POST /boards/:board_id/posts/:id/like (frontend-compatible toggle)
func (h *GoodHandler) LikePost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	ip := c.ClientIP()
	result, err := h.service.ToggleLike(boardID, wrID, userID, ip)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// DislikePost handles POST /boards/:board_id/posts/:id/dislike (frontend-compatible toggle)
func (h *GoodHandler) DislikePost(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	ip := c.ClientIP()
	result, err := h.service.ToggleDislike(boardID, wrID, userID, ip)
	if err != nil {
		handleGoodError(c, err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// GetLikeStatus handles GET /boards/:board_id/posts/:id/like-status
func (h *GoodHandler) GetLikeStatus(c *gin.Context) {
	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.service.GetLikeStatus(boardID, wrID, userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "상태 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// GetLikers handles GET /boards/:board_id/posts/:id/likers
func (h *GoodHandler) GetLikers(c *gin.Context) {
	boardID := c.Param("board_id")
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID입니다", err)
		return
	}

	page := 1
	if val, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		page = val
	}
	limit := 20
	if val, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil {
		limit = val
	}

	result, err := h.service.GetLikers(boardID, wrID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "추천자 목록 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// handleGoodError maps service errors to HTTP responses
func handleGoodError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, common.ErrPostNotFound), errors.Is(err, common.ErrCommentNotFound):
		common.ErrorResponse(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, common.ErrSelfRecommend):
		common.ErrorResponse(c, http.StatusForbidden, "자신의 글은 추천할 수 없습니다", err)
	case errors.Is(err, common.ErrAlreadyRecommended):
		common.ErrorResponse(c, http.StatusConflict, "이미 추천/비추천한 글입니다", err)
	case errors.Is(err, common.ErrNotRecommended):
		common.ErrorResponse(c, http.StatusBadRequest, "추천/비추천하지 않은 글입니다", err)
	default:
		common.ErrorResponse(c, http.StatusInternalServerError, "서버 오류가 발생했습니다", err)
	}
}
