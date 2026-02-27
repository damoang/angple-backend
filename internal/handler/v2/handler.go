package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// V2Handler handles all v2 API endpoints
type V2Handler struct {
	userRepo     v2repo.UserRepository
	postRepo     v2repo.PostRepository
	commentRepo  v2repo.CommentRepository
	boardRepo    v2repo.BoardRepository
	permChecker  middleware.BoardPermissionChecker
	pointRepo    v2repo.PointRepository
	revisionRepo v2repo.RevisionRepository
}

// NewV2Handler creates a new V2Handler
func NewV2Handler(
	userRepo v2repo.UserRepository,
	postRepo v2repo.PostRepository,
	commentRepo v2repo.CommentRepository,
	boardRepo v2repo.BoardRepository,
	permChecker middleware.BoardPermissionChecker,
) *V2Handler {
	return &V2Handler{
		userRepo:    userRepo,
		postRepo:    postRepo,
		commentRepo: commentRepo,
		boardRepo:   boardRepo,
		permChecker: permChecker,
	}
}

// SetPointRepository sets the optional point repository for point operations
func (h *V2Handler) SetPointRepository(repo v2repo.PointRepository) {
	h.pointRepo = repo
}

// SetRevisionRepository sets the optional revision repository for revision history
func (h *V2Handler) SetRevisionRepository(repo v2repo.RevisionRepository) {
	h.revisionRepo = repo
}

// isOwnerOrAdmin checks if the current user is the owner of the resource or an admin
func isOwnerOrAdmin(c *gin.Context, resourceUserID uint64) bool {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		return false
	}
	if userID == resourceUserID {
		return true
	}
	return middleware.GetUserLevel(c) >= 10
}

// === Users ===

// GetUser handles GET /api/v1/users/:id
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

// GetUserByUsername handles GET /api/v1/users/username/:username
func (h *V2Handler) GetUserByUsername(c *gin.Context) {
	username := c.Param("username")
	user, err := h.userRepo.FindByUsername(username)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "사용자를 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, user)
}

// ListUsers handles GET /api/v1/users
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

// boardWithPermissions wraps a board with user-specific permissions
type boardWithPermissions struct {
	*v2domain.V2Board
	Permissions *middleware.BoardPermissions `json:"permissions,omitempty"`
}

// ListBoards handles GET /api/v1/boards
func (h *V2Handler) ListBoards(c *gin.Context) {
	boards, err := h.boardRepo.FindAll()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시판 목록 조회 실패", err)
		return
	}

	// 인증된 사용자면 각 게시판별 permissions 포함
	memberLevel := middleware.GetUserLevel(c)
	if memberLevel > 0 && h.permChecker != nil {
		result := make([]boardWithPermissions, len(boards))
		for i, board := range boards {
			perms, _ := h.permChecker.GetAllPermissions(board.Slug, memberLevel)
			result[i] = boardWithPermissions{V2Board: board, Permissions: perms}
		}
		common.V2Success(c, result)
		return
	}

	common.V2Success(c, boards)
}

// GetBoard handles GET /api/v1/boards/:slug
func (h *V2Handler) GetBoard(c *gin.Context) {
	slug := c.Param("slug")
	board, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

	// 인증된 사용자면 permissions 포함
	memberLevel := middleware.GetUserLevel(c)
	if memberLevel > 0 && h.permChecker != nil {
		perms, _ := h.permChecker.GetAllPermissions(slug, memberLevel)
		common.V2Success(c, boardWithPermissions{V2Board: board, Permissions: perms})
		return
	}

	common.V2Success(c, board)
}

// === Posts ===

// ListPosts handles GET /api/v1/boards/:slug/posts
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

// GetPost handles GET /api/v1/boards/:slug/posts/:id
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

// CreatePost handles POST /api/v1/boards/:slug/posts
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

	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	// 레벨 체크 (미들웨어 우회 방어)
	userLevel := middleware.GetUserLevel(c)
	if userLevel < int(board.WriteLevel) {
		common.V2ErrorResponse(c, http.StatusForbidden, "글쓰기 권한이 없습니다. 레벨 "+strconv.Itoa(int(board.WriteLevel))+" 이상이 필요합니다.", nil)
		return
	}

	// 포인트 차감 게시판인 경우 잔액 확인
	if board.WritePoint < 0 && h.pointRepo != nil {
		canAfford, err := h.pointRepo.CanAfford(userID, board.WritePoint)
		if err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 확인 실패", err)
			return
		}
		if !canAfford {
			common.V2ErrorResponse(c, http.StatusForbidden,
				"포인트가 부족합니다. "+strconv.Itoa(-board.WritePoint)+"포인트가 필요합니다.", nil)
			return
		}
	}

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

	// 포인트 처리 (지급 또는 차감)
	if board.WritePoint != 0 && h.pointRepo != nil {
		_ = h.pointRepo.AddPoint(userID, board.WritePoint, "글쓰기", "v2_posts", post.ID) //nolint:errcheck
	}

	common.V2Created(c, post)
}

// UpdatePost handles PUT /api/v1/boards/:slug/posts/:id
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

	// 권한 체크: 작성자 또는 관리자만 수정 가능
	if !isOwnerOrAdmin(c, post.UserID) {
		common.V2ErrorResponse(c, http.StatusForbidden, "수정 권한이 없습니다", nil)
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

	// 리비전 저장 (수정 전 상태)
	if h.revisionRepo != nil && (req.Title != "" || req.Content != "") {
		userID, _ := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
		nickname := middleware.GetNickname(c)
		nextVersion, _ := h.revisionRepo.GetNextVersion(post.ID)
		revision := &v2domain.V2ContentRevision{
			PostID:       post.ID,
			Version:      nextVersion,
			ChangeType:   "update",
			Title:        post.Title,
			Content:      post.Content,
			EditedBy:     userID,
			EditedByName: nickname,
		}
		_ = h.revisionRepo.Create(revision) //nolint:errcheck
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

// DeletePost handles DELETE /api/v1/boards/:slug/posts/:id (soft delete)
func (h *V2Handler) DeletePost(c *gin.Context) {
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

	// 권한 체크: 작성자 또는 관리자만 삭제 가능
	if !isOwnerOrAdmin(c, post.UserID) {
		common.V2ErrorResponse(c, http.StatusForbidden, "삭제 권한이 없습니다", nil)
		return
	}

	userID, _ := strconv.ParseUint(middleware.GetUserID(c), 10, 64)

	// 리비전 저장 (삭제 전 상태)
	if h.revisionRepo != nil {
		nickname := middleware.GetNickname(c)
		nextVersion, _ := h.revisionRepo.GetNextVersion(post.ID)
		revision := &v2domain.V2ContentRevision{
			PostID:       post.ID,
			Version:      nextVersion,
			ChangeType:   "soft_delete",
			Title:        post.Title,
			Content:      post.Content,
			EditedBy:     userID,
			EditedByName: nickname,
		}
		_ = h.revisionRepo.Create(revision) //nolint:errcheck
	}

	if err := h.postRepo.SoftDelete(id, userID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}

// SoftDeletePost handles PATCH /api/v1/boards/:slug/posts/:id/soft-delete
func (h *V2Handler) SoftDeletePost(c *gin.Context) {
	h.DeletePost(c)
}

// RestorePost handles POST /api/v1/boards/:slug/posts/:id/restore (admin only)
func (h *V2Handler) RestorePost(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	post, err := h.postRepo.FindByIDIncludeDeleted(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시글을 찾을 수 없습니다", err)
		return
	}

	if post.Status != "deleted" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "삭제된 게시글이 아닙니다", nil)
		return
	}

	// 리비전 저장 (복구 기록)
	if h.revisionRepo != nil {
		userID, _ := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
		nickname := middleware.GetNickname(c)
		nextVersion, _ := h.revisionRepo.GetNextVersion(post.ID)
		revision := &v2domain.V2ContentRevision{
			PostID:       post.ID,
			Version:      nextVersion,
			ChangeType:   "restore",
			Title:        post.Title,
			Content:      post.Content,
			EditedBy:     userID,
			EditedByName: nickname,
		}
		_ = h.revisionRepo.Create(revision) //nolint:errcheck
	}

	if err := h.postRepo.Restore(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 복구 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "복구 완료"})
}

// PermanentDeletePost handles DELETE /api/v1/boards/:slug/posts/:id/permanent (admin only)
func (h *V2Handler) PermanentDeletePost(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	if err := h.postRepo.PermanentDelete(id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "영구 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "영구 삭제 완료"})
}

// GetDeletedPosts handles GET /api/v1/admin/posts/deleted (admin only)
func (h *V2Handler) GetDeletedPosts(c *gin.Context) {
	page, perPage := parsePagination(c)
	posts, total, err := h.postRepo.FindDeleted(page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "삭제된 게시글 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, posts, common.NewV2Meta(page, perPage, total))
}

// GetPostRevisions handles GET /api/v1/boards/:slug/posts/:id/revisions
func (h *V2Handler) GetPostRevisions(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	if h.revisionRepo == nil {
		common.V2Success(c, []any{})
		return
	}

	revisions, err := h.revisionRepo.FindByPostID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리비전 조회 실패", err)
		return
	}
	common.V2Success(c, revisions)
}

// RestoreRevision handles POST /api/v1/boards/:slug/posts/:id/revisions/:version/restore
func (h *V2Handler) RestoreRevision(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	version, err := strconv.ParseUint(c.Param("version"), 10, 32)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 버전 번호", err)
		return
	}

	post, err := h.postRepo.FindByIDIncludeDeleted(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시글을 찾을 수 없습니다", err)
		return
	}

	if h.revisionRepo == nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리비전 기능이 비활성화되어 있습니다", nil)
		return
	}

	revision, err := h.revisionRepo.FindByPostIDAndVersion(id, uint(version))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "해당 버전을 찾을 수 없습니다", err)
		return
	}

	// 현재 상태를 리비전으로 저장
	userID, _ := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	nickname := middleware.GetNickname(c)
	nextVersion, _ := h.revisionRepo.GetNextVersion(post.ID)
	_ = h.revisionRepo.Create(&v2domain.V2ContentRevision{
		PostID:       post.ID,
		Version:      nextVersion,
		ChangeType:   "restore",
		Title:        post.Title,
		Content:      post.Content,
		EditedBy:     userID,
		EditedByName: nickname,
	}) //nolint:errcheck

	// 리비전의 내용으로 복원
	post.Title = revision.Title
	post.Content = revision.Content
	if post.Status == "deleted" {
		post.Status = "published"
		post.DeletedAt = nil
		post.DeletedBy = nil
	}
	if err := h.postRepo.Update(post); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "리비전 복원 실패", err)
		return
	}
	common.V2Success(c, post)
}

// === Comments ===

// ListComments handles GET /api/v1/boards/:slug/posts/:id/comments
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

// CreateComment handles POST /api/v1/boards/:slug/posts/:id/comments
func (h *V2Handler) CreateComment(c *gin.Context) {
	slug := c.Param("slug")
	board, err := h.boardRepo.FindBySlug(slug)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시판을 찾을 수 없습니다", err)
		return
	}

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

	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	// 레벨 체크 (미들웨어 우회 방어)
	userLevel := middleware.GetUserLevel(c)
	if userLevel < int(board.CommentLevel) {
		common.V2ErrorResponse(c, http.StatusForbidden, "댓글 작성 권한이 없습니다. 레벨 "+strconv.Itoa(int(board.CommentLevel))+" 이상이 필요합니다.", nil)
		return
	}

	// 포인트 차감 게시판인 경우 잔액 확인
	if board.CommentPoint < 0 && h.pointRepo != nil {
		canAfford, err := h.pointRepo.CanAfford(userID, board.CommentPoint)
		if err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 확인 실패", err)
			return
		}
		if !canAfford {
			common.V2ErrorResponse(c, http.StatusForbidden,
				"포인트가 부족합니다. "+strconv.Itoa(-board.CommentPoint)+"포인트가 필요합니다.", nil)
			return
		}
	}

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

	// 포인트 처리 (지급 또는 차감)
	if board.CommentPoint != 0 && h.pointRepo != nil {
		_ = h.pointRepo.AddPoint(userID, board.CommentPoint, "댓글작성", "v2_comments", comment.ID) //nolint:errcheck
	}

	common.V2Created(c, comment)
}

// UpdateComment handles PUT /api/v1/boards/:slug/posts/:id/comments/:comment_id
func (h *V2Handler) UpdateComment(c *gin.Context) {
	commentID, err := strconv.ParseUint(c.Param("comment_id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID", err)
		return
	}

	comment, err := h.commentRepo.FindByID(commentID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "댓글을 찾을 수 없습니다", err)
		return
	}

	// 권한 체크: 작성자 또는 관리자만 수정 가능
	if !isOwnerOrAdmin(c, comment.UserID) {
		common.V2ErrorResponse(c, http.StatusForbidden, "수정 권한이 없습니다", nil)
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	comment.Content = req.Content
	if err := h.commentRepo.Update(comment); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 수정 실패", err)
		return
	}
	common.V2Success(c, comment)
}

// DeleteComment handles DELETE /api/v1/boards/:slug/posts/:post_id/comments/:comment_id
func (h *V2Handler) DeleteComment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("comment_id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 댓글 ID", err)
		return
	}

	comment, err := h.commentRepo.FindByID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "댓글을 찾을 수 없습니다", err)
		return
	}

	// 권한 체크: 작성자 또는 관리자만 삭제 가능
	if !isOwnerOrAdmin(c, comment.UserID) {
		common.V2ErrorResponse(c, http.StatusForbidden, "삭제 권한이 없습니다", nil)
		return
	}

	userID, _ := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err := h.commentRepo.SoftDelete(id, userID); err != nil {
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
