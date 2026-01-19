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

// PostHandler handles HTTP requests for posts
type PostHandler struct {
	service service.PostService
}

// NewPostHandler creates a new PostHandler
func NewPostHandler(service service.PostService) *PostHandler {
	return &PostHandler{service: service}
}

// ListPosts godoc
// @Summary      게시글 목록 조회
// @Description  특정 게시판의 게시글 목록을 페이지네이션하여 조회합니다
// @Tags         posts
// @Accept       json
// @Produce      json
// @Param        board_id  path      string  true   "게시판 ID (예: free, notice)"
// @Param        page      query     int     false  "페이지 번호 (기본값: 1)"  default(1)
// @Param        limit     query     int     false  "페이지당 항목 수 (기본값: 20)"  default(20)
// @Success      200  {object}  common.APIResponse{data=[]domain.Post}
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts [get]
func (h *PostHandler) ListPosts(c *gin.Context) {
	boardID := c.Param("board_id")
	page := ginutil.QueryInt(c, "page", 1)
	limit := ginutil.QueryInt(c, "limit", 20)

	data, meta, err := h.service.ListPosts(boardID, page, limit)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch posts", err)
		return
	}

	common.SuccessResponse(c, data, meta)
}

// GetPost godoc
// @Summary      게시글 상세 조회
// @Description  특정 게시판의 특정 게시글 상세 정보를 조회합니다
// @Tags         posts
// @Accept       json
// @Produce      json
// @Param        board_id  path      string  true   "게시판 ID"
// @Param        id        path      int     true   "게시글 ID"
// @Success      200  {object}  common.APIResponse{data=domain.Post}
// @Failure      400  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts/{id} [get]
//
//nolint:dupl // Post와 Comment의 Get 로직은 유사하지만 다른 타입을 다룸
func (h *PostHandler) GetPost(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	data, err := h.service.GetPost(boardID, id)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Post not found", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch post", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreatePost godoc
// @Summary      게시글 작성
// @Description  특정 게시판에 새 게시글을 작성합니다 (인증 필요)
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        board_id  path      string                    true   "게시판 ID"
// @Param        request   body      domain.CreatePostRequest  true   "게시글 작성 요청"
// @Success      201  {object}  common.APIResponse{data=domain.Post}
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts [post]
func (h *PostHandler) CreatePost(c *gin.Context) {
	boardID := c.Param("board_id")

	var req domain.CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	data, err := h.service.CreatePost(boardID, &req, authorID)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to create post", err)
		return
	}

	c.JSON(201, common.APIResponse{Data: data})
}

// UpdatePost godoc
// @Summary      게시글 수정
// @Description  특정 게시판의 특정 게시글을 수정합니다 (작성자 본인만 가능)
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        board_id  path      string                    true   "게시판 ID"
// @Param        id        path      int                       true   "게시글 ID"
// @Param        request   body      domain.UpdatePostRequest  true   "게시글 수정 요청"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts/{id} [put]
//
//nolint:dupl // Post와 Comment의 Update/Delete 로직은 유사하지만 다른 타입을 다룸
func (h *PostHandler) UpdatePost(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	var req domain.UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, 400, "Invalid request body", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.UpdatePost(boardID, id, &req, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Post not found", err)
		return
	}
	if errors.Is(err, common.ErrUnauthorized) {
		common.ErrorResponse(c, 403, "Unauthorized", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to update post", err)
		return
	}

	c.Status(204)
}

// DeletePost godoc
// @Summary      게시글 삭제
// @Description  특정 게시판의 특정 게시글을 삭제합니다 (작성자 본인만 가능)
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        board_id  path      string  true   "게시판 ID"
// @Param        id        path      int     true   "게시글 ID"
// @Success      204  "No Content"
// @Failure      400  {object}  common.APIResponse
// @Failure      401  {object}  common.APIResponse
// @Failure      403  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts/{id} [delete]
func (h *PostHandler) DeletePost(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "id")
	if err != nil {
		common.ErrorResponse(c, 400, "Invalid post ID", err)
		return
	}

	// Get authenticated user ID from JWT middleware
	authorID := middleware.GetUserID(c)

	err = h.service.DeletePost(boardID, id, authorID)
	if errors.Is(err, common.ErrPostNotFound) {
		common.ErrorResponse(c, 404, "Post not found", err)
		return
	}
	if errors.Is(err, common.ErrUnauthorized) {
		common.ErrorResponse(c, 403, "Unauthorized", err)
		return
	}
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to delete post", err)
		return
	}

	c.Status(204)
}

// SearchPosts godoc
// @Summary      게시글 검색
// @Description  특정 게시판에서 키워드로 게시글을 검색합니다
// @Tags         posts
// @Accept       json
// @Produce      json
// @Param        board_id  path      string  true   "게시판 ID"
// @Param        q         query     string  true   "검색 키워드"
// @Param        page      query     int     false  "페이지 번호 (기본값: 1)"  default(1)
// @Param        limit     query     int     false  "페이지당 항목 수 (기본값: 20)"  default(20)
// @Success      200  {object}  common.APIResponse{data=[]domain.Post}
// @Failure      400  {object}  common.APIResponse
// @Failure      500  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts/search [get]
func (h *PostHandler) SearchPosts(c *gin.Context) {
	boardID := c.Param("board_id")
	keyword := c.Query("q")
	page := ginutil.QueryInt(c, "page", 1)
	limit := ginutil.QueryInt(c, "limit", 20)

	if keyword == "" {
		common.ErrorResponse(c, 400, "Search keyword required", nil)
		return
	}

	data, meta, err := h.service.SearchPosts(boardID, keyword, page, limit)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to search posts", err)
		return
	}

	common.SuccessResponse(c, data, meta)
}

// PostPreviewResponse represents the preview response
type PostPreviewResponse struct {
	Success bool             `json:"success"`
	Message string           `json:"message,omitempty"`
	Post    *PostPreviewData `json:"post,omitempty"`
}

// PostPreviewData represents post data for preview
type PostPreviewData struct {
	Subject      string   `json:"subject"`
	Content      string   `json:"content"`
	Name         string   `json:"name"`
	Datetime     string   `json:"datetime"`
	Hit          int      `json:"hit"`
	Good         int      `json:"good"`
	Nogood       int      `json:"nogood"`
	IP           string   `json:"ip"`
	IsComment    bool     `json:"is_comment"`
	CommentInfo  string   `json:"comment_info,omitempty"`
	Files        []string `json:"files"`
	Links        []string `json:"links"`
	BoardSubject string   `json:"board_subject"`
}

// GetPostPreview godoc
// @Summary      게시글 미리보기
// @Description  특정 게시글의 미리보기 정보를 조회합니다 (관리자용)
// @Tags         posts
// @Accept       json
// @Produce      json
// @Param        board_id  path      string  true   "게시판 ID"
// @Param        id        path      int     true   "게시글 ID"
// @Success      200  {object}  common.APIResponse{data=PostPreviewResponse}
// @Failure      400  {object}  common.APIResponse
// @Failure      404  {object}  common.APIResponse
// @Router       /boards/{board_id}/posts/{id}/preview [get]
func (h *PostHandler) GetPostPreview(c *gin.Context) {
	boardID := c.Param("board_id")
	id, err := ginutil.ParamInt(c, "id")
	if err != nil {
		c.JSON(200, PostPreviewResponse{
			Success: false,
			Message: "필수 파라미터가 누락되었습니다.",
		})
		return
	}

	post, err := h.service.GetPost(boardID, id)
	if err != nil {
		c.JSON(200, PostPreviewResponse{
			Success: false,
			Message: "게시물을 찾을 수 없습니다.",
		})
		return
	}

	// Prepare response
	previewData := &PostPreviewData{
		Subject:      post.Title,
		Content:      post.Content,
		Name:         post.Author,
		Datetime:     post.CreatedAt.Format("2006-01-02 15:04:05"),
		Hit:          post.Views,
		Good:         post.Likes,
		Nogood:       0,     // TODO: Add nogood field to PostResponse
		IP:           "",    // IP is hidden for privacy
		IsComment:    false, // Preview is only for posts, not comments
		Files:        []string{},
		Links:        []string{},
		BoardSubject: boardID, // TODO: Get board subject from board service
	}

	c.JSON(200, PostPreviewResponse{
		Success: true,
		Post:    previewData,
	})
}
