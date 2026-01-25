package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/damoang/angple-backend/pkg/ginutil"
	"github.com/gin-gonic/gin"
)

type CommentHandler struct {
	service service.CommentService
}

func NewCommentHandler(service service.CommentService) *CommentHandler {
	return &CommentHandler{service: service}
}

// ListComments handles GET /api/v2/boards/:board_id/posts/:id/comments
func (h *CommentHandler) ListComments(c *gin.Context) {
	boardID := c.Param("board_id")
	postID, err := ginutil.ParamInt(c, "id") // 파라미터 이름 변경: post_id -> id
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	data, err := h.service.ListComments(boardID, postID)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch comments", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// GetComment handles GET /api/v2/boards/:board_id/posts/:id/comments/:comment_id
//
//nolint:dupl // Comment와 Post의 Get 로직은 유사하지만 다른 타입을 다룸
func (h *CommentHandler) GetComment(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "comment_id") // 파라미터 이름 변경: id -> comment_id
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid comment ID", err)
		return
	}

	data, err := h.service.GetComment(boardID, id)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Comment not found", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch comment", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreateComment handles POST /api/v2/boards/:board_id/posts/:id/comments
func (h *CommentHandler) CreateComment(c *gin.Context) {
	boardID := c.Param("board_id")
	postID, err := ginutil.ParamInt(c, "id") // 파라미터 이름 변경: post_id -> id
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	var req domain.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	data, err := h.service.CreateComment(boardID, postID, &req, authorID)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to create comment", err)
		return
	}

	c.JSON(201, common.APIResponse{Data: data})
}

// UpdateComment handles PUT /api/v2/boards/:board_id/posts/:id/comments/:comment_id
//
//nolint:dupl // Comment와 Post의 Update/Delete 로직은 유사하지만 다른 타입을 다룸
func (h *CommentHandler) UpdateComment(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "comment_id") // 파라미터 이름 변경: id -> comment_id
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid comment ID", err)
		return
	}

	var req domain.UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.UpdateComment(boardID, id, &req, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Comment not found", err)
		return
	}
	if errors.Is(err, common.ErrUnauthorized) {
		common.ErrorResponse(c, 403, "Unauthorized", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to update comment", err)
		return
	}

	c.Status(204)
}

// DeleteComment handles DELETE /api/v2/boards/:board_id/posts/:id/comments/:comment_id
func (h *CommentHandler) DeleteComment(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "comment_id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid comment ID", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.DeleteComment(boardID, id, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Comment not found", err)
		return
	}
	if errors.Is(err, common.ErrUnauthorized) {
		common.ErrorResponse(c, 403, "Unauthorized", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to delete comment", err)
		return
	}

	c.Status(204)
}

// LikeComment handles POST /api/v2/boards/:board_id/posts/:id/comments/:comment_id/like
//
//nolint:dupl // Like와 Dislike는 구조가 유사하나 의미적으로 다른 핸들러
func (h *CommentHandler) LikeComment(c *gin.Context) {
	boardID := c.Param("board_id")
	commentID, err := ginutil.ParamInt(c, "comment_id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid comment ID", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	userID := middleware.GetUserID(c)

	result, err := h.service.LikeComment(boardID, commentID, userID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Comment not found", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to like comment", err)
		return
	}

	common.SuccessResponse(c, result, nil)
}

// DislikeComment handles POST /api/v2/boards/:board_id/posts/:id/comments/:comment_id/dislike
func (h *CommentHandler) DislikeComment(c *gin.Context) {
	boardID := c.Param("board_id")
	commentID, err := ginutil.ParamInt(c, "comment_id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid comment ID", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	userID := middleware.GetUserID(c)

	result, err := h.service.DislikeComment(boardID, commentID, userID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Comment not found", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to dislike comment", err)
		return
	}

	common.SuccessResponse(c, result, nil)
}
