package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// V2Handler handles all v2 API endpoints
type V2Handler struct {
	userRepo    v2repo.UserRepository
	postRepo    v2repo.PostRepository
	commentRepo v2repo.CommentRepository
	boardRepo   v2repo.BoardRepository
}

// NewV2Handler creates a new V2Handler
func NewV2Handler(
	userRepo v2repo.UserRepository,
	postRepo v2repo.PostRepository,
	commentRepo v2repo.CommentRepository,
	boardRepo v2repo.BoardRepository,
) *V2Handler {
	return &V2Handler{
		userRepo:    userRepo,
		postRepo:    postRepo,
		commentRepo: commentRepo,
		boardRepo:   boardRepo,
	}
}

// === Users ===

// GetUser handles GET /api/v2-next/users/:id
func (h *V2Handler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 사용자 ID", err)
		return
	}
	user, err := h.userRepo.FindByID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "사용자를 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, user)
}

// GetUserByUsername handles GET /api/v2-next/users/username/:username
func (h *V2Handler) GetUserByUsername(c *gin.Context) {
	username := c.Param("username")
	user, err := h.userRepo.FindByUsername(username)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "사용자를 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, user)
}

// ListUsers handles GET /api/v2-next/users
func (h *V2Handler) ListUsers(c *gin.Context) {
	page, perPage := parsePagination(c)
	keyword := c.Query("keyword")

	users, total, err := h.userRepo.FindAll(page, perPage, keyword)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "사용자 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, users, common.NewV2Meta(page, perPage, total))
}

// === Boards ===

// ListBoards handles GET /api/v2-next/boards
func (h *V2Handler) ListBoards(c *gin.Context) {
	boards, err := h.boardRepo.FindAll()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 목록 조회 실패", err)
		return
	}
	common.V2Success(c, boards)
}

// GetBoard handles GET /api/v2-next/boards/:slug
func (h *V2Handler) GetBoard(c *gin.Context) {
	slug := c.Param("slug")
	board, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, board)
}

// === Posts ===

// ListPosts handles GET /api/v2-next/boards/:slug/posts
func (h *V2Handler) ListPosts(c *gin.Context) {
	slug := c.Param("slug")
	board, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	page, perPage := parsePagination(c)
	posts, total, err := h.postRepo.FindByBoard(board.ID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, posts, common.NewV2Meta(page, perPage, total))
}

// GetPost handles GET /api/v2-next/boards/:slug/posts/:id
func (h *V2Handler) GetPost(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	post, err := h.postRepo.FindByID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시글을 찾을 수 없습니다", err)
		return
	}

	_ = h.postRepo.IncrementViewCount(id) //nolint:errcheck
	common.V2Success(c, post)
}

// CreatePost handles POST /api/v2-next/boards/:slug/posts
func (h *V2Handler) CreatePost(c *gin.Context) {
	slug := c.Param("slug")
	board, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	var req struct {
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// TODO: get user_id from JWT claims
	userID := uint64(1) // placeholder

	post := &v2domain.V2Post{
		BoardID: board.ID,
		UserID:  userID,
		Title:   req.Title,
		Content: req.Content,
		Status:  "published",
	}
	if err := h.postRepo.Create(post); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 작성 실패", err)
		return
	}
	common.V2Created(c, post)
}

// UpdatePost handles PUT /api/v2-next/boards/:slug/posts/:id
func (h *V2Handler) UpdatePost(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	post, err := h.postRepo.FindByID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시글을 찾을 수 없습니다", err)
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	if req.Title != "" {
		post.Title = req.Title
	}
	if req.Content != "" {
		post.Content = req.Content
	}
	if err := h.postRepo.Update(post); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 수정 실패", err)
		return
	}
	common.V2Success(c, post)
}

// DeletePost handles DELETE /api/v2-next/boards/:slug/posts/:id
func (h *V2Handler) DeletePost(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	if err := h.postRepo.Delete(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}

// === Comments ===

// ListComments handles GET /api/v2-next/boards/:slug/posts/:id/comments
func (h *V2Handler) ListComments(c *gin.Context) {
	postID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	page, perPage := parsePagination(c)
	comments, total, err := h.commentRepo.FindByPost(postID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, comments, common.NewV2Meta(page, perPage, total))
}

// CreateComment handles POST /api/v2-next/boards/:slug/posts/:id/comments
func (h *V2Handler) CreateComment(c *gin.Context) {
	postID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	var req struct {
		Content  string  `json:"content" binding:"required"`
		ParentID *uint64 `json:"parent_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// TODO: get user_id from JWT claims
	userID := uint64(1)

	comment := &v2domain.V2Comment{
		PostID:   postID,
		UserID:   userID,
		ParentID: req.ParentID,
		Content:  req.Content,
		Status:   "active",
	}
	if req.ParentID != nil {
		comment.Depth = 1
	}

	if err := h.commentRepo.Create(comment); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 작성 실패", err)
		return
	}
	common.V2Created(c, comment)
}

// DeleteComment handles DELETE /api/v2-next/boards/:slug/posts/:post_id/comments/:comment_id
func (h *V2Handler) DeleteComment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("comment_id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID", err)
		return
	}
	if err := h.commentRepo.Delete(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}

// === Helpers ===

func parsePagination(c *gin.Context) (int, int) {
	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	perPage := 20
	if l, err := strconv.Atoi(c.Query("per_page")); err == nil && l > 0 && l <= 100 {
		perPage = l
	}
	return page, perPage
}
