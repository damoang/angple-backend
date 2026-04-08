package v2

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// V2Handler handles all v2 API endpoints
type V2Handler struct {
	userRepo          v2repo.UserRepository
	postRepo          v2repo.PostRepository
	commentRepo       v2repo.CommentRepository
	boardRepo         v2repo.BoardRepository
	permChecker       middleware.BoardPermissionChecker
	pointRepo         v2repo.PointRepository
	revisionRepo      v2repo.RevisionRepository
	notiRepo          gnurepo.NotiRepository
	notiPrefRepo      gnurepo.NotiPreferenceRepository
	expRepo           v2repo.ExpRepository
	gnuDB             *gorm.DB // gnuboard g5_member 조회용
	gnuPointWriteRepo v2repo.GnuboardPointWriteRepository
	pointConfigRepo   v2repo.PointConfigRepository
	blockRepo         v2repo.BlockRepository
	tagRepo           gnurepo.TagRepository
}

const claimBoardSlug = "claim"

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

// SetNotiRepository sets the notification repository for creating notifications
func (h *V2Handler) SetNotiRepository(repo gnurepo.NotiRepository) {
	h.notiRepo = repo
}

// SetNotiPreferenceRepository sets the notification preference repository
func (h *V2Handler) SetNotiPreferenceRepository(repo gnurepo.NotiPreferenceRepository) {
	h.notiPrefRepo = repo
}

// SetGnuDB sets the gnuboard database connection for mb_id → mb_no lookup
func (h *V2Handler) SetGnuDB(db *gorm.DB) {
	h.gnuDB = db
}

// SetExpRepository sets the optional exp repository for XP operations
func (h *V2Handler) SetExpRepository(repo v2repo.ExpRepository) {
	h.expRepo = repo
}

// SetGnuboardPointWriteRepository sets the gnuboard point write repository for g5_point operations
func (h *V2Handler) SetGnuboardPointWriteRepository(repo v2repo.GnuboardPointWriteRepository) {
	h.gnuPointWriteRepo = repo
}

// SetPointConfigRepository sets the point config repository for point expiry settings
func (h *V2Handler) SetPointConfigRepository(repo v2repo.PointConfigRepository) {
	h.pointConfigRepo = repo
}

// SetBlockRepository sets the block repository for filtering blocked users
func (h *V2Handler) SetBlockRepository(repo v2repo.BlockRepository) {
	h.blockRepo = repo
}

// SetTagRepository sets the tag repository for tag cleanup on post deletion
func (h *V2Handler) SetTagRepository(repo gnurepo.TagRepository) {
	h.tagRepo = repo
}

// getBlockedUserIDs returns blocked user IDs (as uint64) for the given mb_id
func (h *V2Handler) getBlockedUserIDs(mbID string) []uint64 {
	if h.blockRepo == nil || mbID == "" || h.gnuDB == nil {
		return nil
	}
	blockedMbIDs, err := h.blockRepo.GetBlockedUserIDs(mbID)
	if err != nil || len(blockedMbIDs) == 0 {
		return nil
	}
	var userIDs []uint64
	if err := h.gnuDB.Table("g5_member").Select("mb_no").Where("mb_id IN ?", blockedMbIDs).Pluck("mb_no", &userIDs).Error; err != nil || len(userIDs) == 0 {
		return nil
	}
	return userIDs
}

// createLevelUpNoti inserts a level-up notification into g5_na_noti
func (h *V2Handler) createLevelUpNoti(mbID string, newLevel int) {
	noti := &gnurepo.Notification{
		MbID:          mbID,
		PhFromCase:    "levelup",
		PhToCase:      "me",
		BoTable:       "@system",
		WrID:          0,
		RelMbID:       "system",
		RelMbNick:     "시스템",
		RelMsg:        fmt.Sprintf("레벨 %d로 승급했습니다!", newLevel),
		RelURL:        "/mypage/exp",
		PhReaded:      "N",
		ParentSubject: fmt.Sprintf("레벨 %d 달성", newLevel),
	}
	if err := h.notiRepo.Create(noti); err != nil {
		log.Printf("[xp] levelup notification failed for %s: %v", mbID, err)
	}
}

// resolveUserIDToMbNo converts gnuboard mb_id (string) to mb_no (uint64)
func (h *V2Handler) resolveUserIDToMbNo(mbID string) (uint64, error) {
	// 먼저 숫자로 직접 변환 시도
	if id, err := strconv.ParseUint(mbID, 10, 64); err == nil {
		return id, nil
	}
	// 실패 시 g5_member에서 mb_no 조회
	if h.gnuDB != nil {
		var mbNo uint64
		err := h.gnuDB.Table("g5_member").Select("mb_no").Where("mb_id = ?", mbID).Scan(&mbNo).Error
		if err != nil {
			return 0, err
		}
		return mbNo, nil
	}
	return 0, strconv.ErrSyntax
}

// certExemptBoards are boards that don't require certification
var certExemptBoards = map[string]bool{
	"verification": true,
	"promotion":    true,
	"overseas":     true,
}

// checkCertification checks if the board requires certification and if the user is certified
// Returns error message string if blocked, empty string if allowed
func (h *V2Handler) checkCertification(c *gin.Context, boardSlug string) string {
	if h.gnuDB == nil {
		return ""
	}
	// 예외 게시판은 무조건 통과
	if certExemptBoards[boardSlug] {
		return ""
	}
	// 관리자(레벨 10+)는 바이패스
	if middleware.GetUserLevel(c) >= 10 {
		return ""
	}
	// g5_board에서 bo_use_cert 확인
	var boUseCert string
	if err := h.gnuDB.Table("g5_board").Select("bo_use_cert").Where("bo_table = ?", boardSlug).Scan(&boUseCert).Error; err != nil || boUseCert == "" {
		return ""
	}
	// bo_use_cert = 'cert': 실명인증 필수
	if boUseCert == "cert" {
		mbID := middleware.GetUserID(c)
		var mbCertify string
		if err := h.gnuDB.Table("g5_member").Select("mb_certify").Where("mb_id = ?", mbID).Scan(&mbCertify).Error; err != nil || mbCertify == "" {
			return "이 게시판은 본인확인 하신 회원님만 이용 가능합니다. 회원정보 수정에서 본인확인을 해주시기 바랍니다."
		}
	}
	return ""
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

	// 검색 파라미터 (sfl: 검색 필드, stx: 검색어)
	searchField := c.Query("sfl")
	searchQuery := c.Query("stx")

	// 차단 사용자 필터링
	blockedUserIDs := h.getBlockedUserIDs(middleware.GetUserID(c))

	var posts []*v2domain.V2Post
	var total int64

	if searchField != "" && searchQuery != "" {
		posts, total, err = h.postRepo.SearchByBoardFiltered(board.ID, searchField, searchQuery, page, perPage, blockedUserIDs)
	} else {
		posts, total, err = h.postRepo.FindByBoardFiltered(board.ID, page, perPage, blockedUserIDs)
	}
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

	// 조회수는 프론트엔드 /api/viewcount 에서 쿠키 기반 중복방지로 처리
	// 백엔드에서 매 요청마다 증가시키면 새로고침할 때마다 무한 증가 (버그)
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
		Title              string `json:"title" binding:"required"`
		Content            string `json:"content" binding:"required"`
		IsSecret           *bool  `json:"is_secret,omitempty"`
		IsCommentsDisabled *bool  `json:"is_comments_disabled,omitempty"`
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

	// 제휴 링크 차단 검증 (deal, economy 게시판)
	if err := common.ValidateAffiliateLinks(req.Content, slug, userLevel, false); err != nil {
		common.V2ErrorResponse(c, http.StatusForbidden, err.Error(), err)
		return
	}
	if userLevel < int(board.WriteLevel) {
		common.V2ErrorResponse(c, http.StatusForbidden, "글쓰기 권한이 없습니다. 레벨 "+strconv.Itoa(int(board.WriteLevel))+" 이상이 필요합니다.", nil)
		return
	}

	// 실명인증 체크
	if certMsg := h.checkCertification(c, slug); certMsg != "" {
		common.V2ErrorResponse(c, http.StatusForbidden, certMsg, nil)
		return
	}

	// 포인트 차감 게시판인 경우 잔액 확인
	if board.WritePoint < 0 {
		mbIDForCheck := middleware.GetUserID(c)
		if h.gnuPointWriteRepo != nil {
			canAfford, err := h.gnuPointWriteRepo.CanAfford(mbIDForCheck, board.WritePoint)
			if err != nil {
				common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 확인 실패", err)
				return
			}
			if !canAfford {
				common.V2ErrorResponse(c, http.StatusForbidden,
					"포인트가 부족합니다. "+strconv.Itoa(-board.WritePoint)+"포인트가 필요합니다.", nil)
				return
			}
		} else if h.pointRepo != nil {
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
	}

	post := &v2domain.V2Post{
		BoardID:  board.ID,
		UserID:   userID,
		Title:    req.Title,
		Content:  common.SanitizePostContent(req.Content),
		Status:   "published",
		IsSecret: req.IsSecret != nil && *req.IsSecret,
	}
	if err := h.postRepo.Create(post); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 작성 실패", err)
		return
	}

	// 포인트 처리 (지급 또는 차감) — g5_point 기반 FIFO 소비
	if board.WritePoint != 0 {
		mbID := middleware.GetUserID(c)
		if h.gnuPointWriteRepo != nil {
			var pc *v2repo.PointConfig
			if h.pointConfigRepo != nil {
				pc, _ = h.pointConfigRepo.GetPointConfig()
			}
			_ = h.gnuPointWriteRepo.AddPoint(mbID, board.WritePoint, "글쓰기", "v2_posts", fmt.Sprintf("%d", post.ID), "@write", pc) //nolint:errcheck
		} else if h.pointRepo != nil {
			_ = h.pointRepo.AddPoint(userID, board.WritePoint, "글쓰기", "v2_posts", post.ID) //nolint:errcheck
		}
	}

	// 경험치 부여 (비동기, best-effort) — 게시판별 XP 사용
	if h.expRepo != nil {
		mbID := middleware.GetUserID(c)
		tableName := fmt.Sprintf("v2_posts_%s", slug)
		wrID := fmt.Sprintf("%d", post.ID)
		boardWriteXP := board.WriteXP
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[xp] write XP panic for %s: %v", mbID, r)
				}
			}()
			xpConfig, err := h.expRepo.GetXPConfig()
			if err != nil || xpConfig == nil || !xpConfig.WriteEnabled {
				return
			}
			xpAmount := boardWriteXP
			if xpAmount <= 0 {
				xpAmount = xpConfig.WriteXP // 게시판 설정 없으면 전역 기본값
			}
			if xpAmount <= 0 {
				return
			}
			result, err := h.expRepo.AddExp(mbID, xpAmount, "글쓰기", tableName, wrID, "@write")
			if err != nil {
				log.Printf("[xp] write XP grant failed for %s: %v", mbID, err)
				return
			}
			if result.LevelUp && h.notiRepo != nil {
				h.createLevelUpNoti(mbID, result.NewLevel)
			}
		}()
	}

	common.V2Created(c, post)
}

// UpdatePost handles PUT /api/v1/boards/:slug/posts/:id
func (h *V2Handler) UpdatePost(c *gin.Context) {
	slug := c.Param("slug")
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

	// 소명글 수정 차단: claim 게시판에서 disciplinelog 연동 글은 수정 불가
	if slug == claimBoardSlug && h.gnuDB != nil && middleware.GetUserLevel(c) < 10 {
		var link1 string
		h.gnuDB.Table("g5_write_claim").Select("wr_link1").Where("wr_id = ?", id).Scan(&link1)
		if strings.HasPrefix(link1, "disciplinelog") {
			common.V2ErrorResponse(c, http.StatusForbidden, "소명글은 수정할 수 없습니다.", nil)
			return
		}
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
		post.Content = common.SanitizePostContent(req.Content)
	}
	if err := h.postRepo.Update(post); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 수정 실패", err)
		return
	}
	common.V2Success(c, post)
}

// DeletePost handles DELETE /api/v1/boards/:slug/posts/:id (soft delete)
func (h *V2Handler) DeletePost(c *gin.Context) {
	slug := c.Param("slug")
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

	// 소명글 삭제 차단: claim 게시판에서 disciplinelog 연동 글은 삭제 불가
	if slug == claimBoardSlug && h.gnuDB != nil && middleware.GetUserLevel(c) < 10 {
		var link1 string
		h.gnuDB.Table("g5_write_claim").Select("wr_link1").Where("wr_id = ?", id).Scan(&link1)
		if strings.HasPrefix(link1, "disciplinelog") {
			common.V2ErrorResponse(c, http.StatusForbidden, "소명글은 삭제할 수 없습니다.", nil)
			return
		}
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

	// 태그 로그 삭제 (삭제된 글의 태그가 검색에 잔존하지 않도록)
	if h.tagRepo != nil {
		boTable := fmt.Sprintf("g5_write_%s", slug)
		if id <= math.MaxInt32 {
			_ = h.tagRepo.DeletePostTags(boTable, int(id)) //nolint:errcheck
		}
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
	blockedUserIDs := h.getBlockedUserIDs(middleware.GetUserID(c))
	comments, total, err := h.commentRepo.FindByPostFiltered(postID, page, perPage, blockedUserIDs)
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

	userID, err := h.resolveUserIDToMbNo(middleware.GetUserID(c))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	// 레벨 체크 (미들웨어 우회 방어)
	userLevel := middleware.GetUserLevel(c)

	// 소명 게시판: admin과 글 작성자만 댓글 가능
	if slug == claimBoardSlug {
		isAdmin := userLevel >= 10
		if !isAdmin {
			// g5_write_claim에서 직접 작성자 확인 (v2_posts와 ID가 다르므로)
			var postAuthorID string
			if h.gnuDB != nil {
				h.gnuDB.Table("g5_write_claim").
					Select("mb_id").
					Where("wr_id = ? AND wr_is_comment = 0", postID).
					Scan(&postAuthorID)
			}
			currentUserMbID := middleware.GetUserID(c)
			if postAuthorID == "" || postAuthorID != currentUserMbID {
				common.V2ErrorResponse(c, http.StatusForbidden, "소명 게시판에서는 관리자와 글 작성자만 댓글을 작성할 수 있습니다.", nil)
				return
			}
		}
	}

	// 제휴 링크 차단 검증 (deal, economy 게시판)
	if err := common.ValidateAffiliateLinks(req.Content, slug, userLevel, false); err != nil {
		common.V2ErrorResponse(c, http.StatusForbidden, err.Error(), err)
		return
	}

	if userLevel < int(board.CommentLevel) {
		common.V2ErrorResponse(c, http.StatusForbidden, "댓글 작성 권한이 없습니다. 레벨 "+strconv.Itoa(int(board.CommentLevel))+" 이상이 필요합니다.", nil)
		return
	}

	// 실명인증 체크
	if certMsg := h.checkCertification(c, slug); certMsg != "" {
		common.V2ErrorResponse(c, http.StatusForbidden, certMsg, nil)
		return
	}

	// 포인트 차감 게시판인 경우 잔액 확인
	if board.CommentPoint < 0 {
		mbIDForCheck := middleware.GetUserID(c)
		if h.gnuPointWriteRepo != nil {
			canAfford, err := h.gnuPointWriteRepo.CanAfford(mbIDForCheck, board.CommentPoint)
			if err != nil {
				common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 확인 실패", err)
				return
			}
			if !canAfford {
				common.V2ErrorResponse(c, http.StatusForbidden,
					"포인트가 부족합니다. "+strconv.Itoa(-board.CommentPoint)+"포인트가 필요합니다.", nil)
				return
			}
		} else if h.pointRepo != nil {
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
	}

	comment := &v2domain.V2Comment{
		PostID:   postID,
		UserID:   userID,
		ParentID: req.ParentID,
		Content:  common.SanitizeComment(req.Content),
		Status:   "active",
	}
	if req.ParentID != nil {
		comment.Depth = 1
	}

	if err := h.commentRepo.Create(comment); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 작성 실패", err)
		return
	}

	// 30일 이전 글 체크 (포인트 + XP 모두 적용)
	// postRepo.FindByID는 PK 조회이므로 빠름 (1회만 조회)
	isOldPost := false
	if parentPost, postErr := h.postRepo.FindByID(postID); postErr == nil {
		isOldPost = time.Since(parentPost.CreatedAt) > 30*24*time.Hour
	}

	// 알림 + 포인트 + XP에서 사용할 mb_id
	mbID := middleware.GetUserID(c)

	// 포인트 처리 (지급 또는 차감) — 30일 이전 글에는 지급 포인트 미부여
	if board.CommentPoint != 0 {
		// 차감 포인트(음수)는 항상 적용, 지급 포인트(양수)는 30일 이내만
		if board.CommentPoint < 0 || !isOldPost {
			if h.gnuPointWriteRepo != nil {
				var pc *v2repo.PointConfig
				if h.pointConfigRepo != nil {
					pc, _ = h.pointConfigRepo.GetPointConfig()
				}
				_ = h.gnuPointWriteRepo.AddPoint(mbID, board.CommentPoint, "댓글작성", fmt.Sprintf("v2_comments_%s", slug), fmt.Sprintf("%d", comment.ID), "@comment", pc) //nolint:errcheck
			} else if h.pointRepo != nil {
				_ = h.pointRepo.AddPoint(userID, board.CommentPoint, "댓글작성", "v2_comments", comment.ID) //nolint:errcheck
			}
		}
	}

	// 알림 생성 (비동기, 에러 무시)
	authorName := middleware.GetNickname(c)
	if authorName == "" && h.gnuDB != nil {
		h.gnuDB.Table("g5_member").Select("mb_nick").Where("mb_id = ?", mbID).Scan(&authorName)
	}
	if h.notiRepo != nil {
		go h.createCommentNotification(slug, postID, comment, mbID, authorName)
	}

	// 경험치 부여 (비동기, best-effort) — 게시판별 XP 사용
	// 30일 이전 글에 달린 댓글은 XP 미부여 (스팸 방지)
	if h.expRepo != nil && !isOldPost {
		tableName := fmt.Sprintf("v2_comments_%s", slug)
		wrID := fmt.Sprintf("%d", comment.ID)
		boardCommentXP := board.CommentXP
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[xp] comment XP panic for %s: %v", mbID, r)
				}
			}()
			xpConfig, err := h.expRepo.GetXPConfig()
			if err != nil || xpConfig == nil || !xpConfig.CommentEnabled {
				return
			}
			xpAmount := boardCommentXP
			if xpAmount <= 0 {
				xpAmount = xpConfig.CommentXP
			}
			if xpAmount <= 0 {
				return
			}
			result, err := h.expRepo.AddExp(mbID, xpAmount, "댓글 작성", tableName, wrID, "@comment")
			if err != nil {
				log.Printf("[xp] comment XP grant failed for %s: %v", mbID, err)
				return
			}
			if result.LevelUp && h.notiRepo != nil {
				h.createLevelUpNoti(mbID, result.NewLevel)
			}
		}()
	}

	// FreeComment 형태로 응답 (프론트엔드 호환)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":         comment.ID,
			"post_id":    comment.PostID,
			"content":    comment.Content,
			"author":     authorName,
			"author_id":  mbID,
			"likes":      0,
			"dislikes":   0,
			"depth":      comment.Depth,
			"created_at": comment.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			"is_secret":  false,
		},
	})
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

	comment.Content = common.SanitizeComment(req.Content)
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

	// 원글의 comment_count 차감
	if comment.PostID > 0 {
		h.postRepo.DecrementCommentCount(comment.PostID)
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

// safeUint64ToInt converts uint64 to int with overflow protection
func safeUint64ToInt(v uint64) int {
	return common.SafeUint64ToInt(v)
}

// createCommentNotification creates a notification for the post author when a comment is posted
func (h *V2Handler) createCommentNotification(boardSlug string, postID uint64, comment *v2domain.V2Comment, commenterMbID, commenterNick string) {
	if h.gnuDB == nil || h.notiRepo == nil {
		return
	}

	// 게시글 작성자 조회
	post, err := h.postRepo.FindByID(postID)
	if err != nil {
		return
	}

	// 게시글 작성자의 mb_id 조회
	var postAuthorMbID string
	if err := h.gnuDB.Table("g5_member").Select("mb_id").Where("mb_no = ?", post.UserID).Scan(&postAuthorMbID).Error; err != nil || postAuthorMbID == "" {
		return
	}

	// 자기 글에 자기가 댓글 달면 알림 안 보냄
	if postAuthorMbID == commenterMbID {
		return
	}

	// 대댓글인 경우 부모 댓글 작성자에게도 알림
	if comment.ParentID != nil {
		parentComment, err := h.commentRepo.FindByID(*comment.ParentID)
		if err == nil {
			var parentAuthorMbID string
			if err := h.gnuDB.Table("g5_member").Select("mb_id").Where("mb_no = ?", parentComment.UserID).Scan(&parentAuthorMbID).Error; err == nil && parentAuthorMbID != "" && parentAuthorMbID != commenterMbID {
				// 수신자가 발신자를 차단했는지 확인
				isBlockedByParent := false
				if h.blockRepo != nil {
					if blockedIDs, err := h.blockRepo.GetBlockedUserIDs(parentAuthorMbID); err == nil {
						isBlockedByParent = slices.Contains(blockedIDs, commenterMbID)
					}
				}
				// 답글 알림 설정 확인
				sendReply := true
				if isBlockedByParent {
					sendReply = false
				} else if h.notiPrefRepo != nil {
					if pref, err := h.notiPrefRepo.Get(parentAuthorMbID); err == nil && !pref.NotiReply {
						sendReply = false
					}
				}
				if sendReply {
					wrID := safeUint64ToInt(postID)
					if exists, _ := h.notiRepo.Exists(parentAuthorMbID, boardSlug, wrID, "comment", commenterMbID); !exists {
						noti := &gnurepo.Notification{
							PhToCase:      "comment_reply",
							PhFromCase:    "comment",
							BoTable:       boardSlug,
							WrID:          wrID,
							MbID:          parentAuthorMbID,
							RelMbID:       commenterMbID,
							RelMbNick:     commenterNick,
							RelMsg:        fmt.Sprintf("%s님이 회원님의 댓글에 답글을 남겼습니다.", commenterNick),
							RelURL:        fmt.Sprintf("/%s/%d#c_%d", boardSlug, postID, comment.ID),
							PhReaded:      "N",
							PhDatetime:    time.Now(),
							ParentSubject: post.Title,
							WrParent:      wrID,
						}
						_ = h.notiRepo.Create(noti)
					}
				}
			}
		}
	}

	// 게시글 작성자가 댓글 작성자를 차단한 경우 알림 생략
	if h.blockRepo != nil {
		if blockedIDs, err := h.blockRepo.GetBlockedUserIDs(postAuthorMbID); err == nil && slices.Contains(blockedIDs, commenterMbID) {
			return
		}
	}

	// 댓글 알림 설정 확인
	if h.notiPrefRepo != nil {
		if pref, err := h.notiPrefRepo.Get(postAuthorMbID); err == nil && !pref.NotiComment {
			return
		}
	}

	// 게시글 작성자에게 알림
	wrID := safeUint64ToInt(postID)
	if exists, _ := h.notiRepo.Exists(postAuthorMbID, boardSlug, wrID, "comment", commenterMbID); !exists {
		noti := &gnurepo.Notification{
			PhToCase:      "comment",
			PhFromCase:    "comment",
			BoTable:       boardSlug,
			WrID:          wrID,
			MbID:          postAuthorMbID,
			RelMbID:       commenterMbID,
			RelMbNick:     commenterNick,
			RelMsg:        fmt.Sprintf("%s님이 회원님의 글에 댓글을 남겼습니다.", commenterNick),
			RelURL:        fmt.Sprintf("/%s/%d#c_%d", boardSlug, postID, comment.ID),
			PhReaded:      "N",
			PhDatetime:    time.Now(),
			ParentSubject: post.Title,
			WrParent:      wrID,
		}
		_ = h.notiRepo.Create(noti)
	}
}
