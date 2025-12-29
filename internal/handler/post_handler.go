package handler

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gofiber/fiber/v2"
)

// PostHandler handles HTTP requests for posts
type PostHandler struct {
	service service.PostService
}

// NewPostHandler creates a new PostHandler
func NewPostHandler(service service.PostService) *PostHandler {
	return &PostHandler{service: service}
}

// ListPosts handles GET /api/v2/boards/:board_id/posts
func (h *PostHandler) ListPosts(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	data, meta, err := h.service.ListPosts(boardID, page, limit)
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to fetch posts", err)
	}

	return common.SuccessResponse(c, data, meta)
}

// GetPost handles GET /api/v2/boards/:board_id/posts/:id
//nolint:dupl // Post와 Comment의 Get 로직은 유사하지만 다른 타입을 다룸
func (h *PostHandler) GetPost(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid post ID", err)
	}

	data, err := h.service.GetPost(boardID, id)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Post not found", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to fetch post", err)
	}

	return common.SuccessResponse(c, data, nil)
}

// CreatePost handles POST /api/v2/boards/:board_id/posts
func (h *PostHandler) CreatePost(c *fiber.Ctx) error {
	boardID := c.Params("board_id")

	var req domain.CreatePostRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	data, err := h.service.CreatePost(boardID, &req, authorID)
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to create post", err)
	}

	return c.Status(201).JSON(common.APIResponse{Data: data})
}

// UpdatePost handles PUT /api/v2/boards/:board_id/posts/:id
//nolint:dupl // Post와 Comment의 Update/Delete 로직은 유사하지만 다른 타입을 다룸
func (h *PostHandler) UpdatePost(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid post ID", err)
	}

	var req domain.UpdatePostRequest
	if err := c.BodyParser(&req); err != nil {
		return common.ErrorResponse(c, 400, "Invalid request body", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.UpdatePost(boardID, id, &req, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Post not found", err)
	}
	if errors.Is(err, common.ErrUnauthorized) {
		return common.ErrorResponse(c, 403, "Unauthorized", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to update post", err)
	}

	return c.Status(204).Send(nil)
}

// DeletePost handles DELETE /api/v2/boards/:board_id/posts/:id
func (h *PostHandler) DeletePost(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	id, err := c.ParamsInt("id")
	if err != nil {
		return common.ErrorResponse(c, 400, "Invalid post ID", err)
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.DeletePost(boardID, id, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		return common.ErrorResponse(c, 404, "Post not found", err)
	}
	if errors.Is(err, common.ErrUnauthorized) {
		return common.ErrorResponse(c, 403, "Unauthorized", err)
	}
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to delete post", err)
	}

	return c.Status(204).Send(nil)
}

// SearchPosts handles GET /api/v2/boards/:board_id/posts/search
func (h *PostHandler) SearchPosts(c *fiber.Ctx) error {
	boardID := c.Params("board_id")
	keyword := c.Query("q", "")
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)

	if keyword == "" {
		return common.ErrorResponse(c, 400, "Search keyword required", nil)
	}

	data, meta, err := h.service.SearchPosts(boardID, keyword, page, limit)
	if err != nil {
		return common.ErrorResponse(c, 500, "Failed to search posts", err)
	}

	return common.SuccessResponse(c, data, meta)
}
