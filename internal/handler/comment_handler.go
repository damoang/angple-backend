package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

type CommentHandler struct {
	service service.CommentService
}

func NewCommentHandler(service service.CommentService) *CommentHandler {
	return &CommentHandler{service: service}
}

// ListComments handles GET /api/v2/boards/:board_id/posts/:post_id/comments
func (h *CommentHandler) ListComments(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	postID, err := c.ParamsInt("post_id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid post ID", err)
	}

	data, err := h.service.ListComments(boardID, postID)
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to fetch comments", err)
	}

	return common.SuccessResponse(c, data, nil)
}

// GetComment handles GET /api/v2/boards/:board_id/posts/:post_id/comments/:id
//nolint:dupl // Comment와 Post의 Get 로직은 유사하지만 다른 타입을 다룸
func (h *CommentHandler) GetComment(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid comment ID", err)
	}

	data, err := h.service.GetComment(boardID, id)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Comment not found", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to fetch comment", err)
	}

	return common.SuccessResponse(c, data, nil)
}

// CreateComment handles POST /api/v2/boards/:board_id/posts/:post_id/comments
func (h *CommentHandler) CreateComment(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	postID, err := c.ParamsInt("post_id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid post ID", err)
	}

	var req domain.CreateCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	data, err := h.service.CreateComment(boardID, postID, &req, authorID)
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to create comment", err)
	}

	return c.Status(201).JSON(common.APIResponse{Data: data})
}

// UpdateComment handles PUT /api/v2/boards/:board_id/posts/:post_id/comments/:id
//nolint:dupl // Comment와 Post의 Update/Delete 로직은 유사하지만 다른 타입을 다룸
func (h *CommentHandler) UpdateComment(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid comment ID", err)
	}

	var req domain.UpdateCommentRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.UpdateComment(boardID, id, &req, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Comment not found", err)
	}
	if errors.Is(err, common.ErrUnauthorized) {
		return common.ErrorResponse(c, 403, "Unauthorized", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to update comment", err)
	}

	return c.Status(204).Send(nil)
}

// DeleteComment handles DELETE /api/v2/boards/:board_id/posts/:post_id/comments/:id
func (h *CommentHandler) DeleteComment(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid comment ID", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.DeleteComment(boardID, id, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Comment not found", err)
	}
	if errors.Is(err, common.ErrUnauthorized) {
		return common.ErrorResponse(c, 403, "Unauthorized", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to delete comment", err)
	}

	return c.Status(204).Send(nil)
}
