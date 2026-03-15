package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	pkgcache "github.com/damoang/angple-backend/pkg/cache"
	"github.com/gin-gonic/gin"
)

const activityCacheTTL = 30 * time.Second

// MyPageHandler handles /api/v1/my/* endpoints for user's posts, comments, liked posts, and stats
type MyPageHandler struct {
	myPageRepo gnurepo.MyPageRepository
	cache      pkgcache.Service
}

// NewMyPageHandler creates a new MyPageHandler
func NewMyPageHandler(myPageRepo gnurepo.MyPageRepository) *MyPageHandler {
	return &MyPageHandler{myPageRepo: myPageRepo}
}

// SetCache sets the cache service for the handler
func (h *MyPageHandler) SetCache(c pkgcache.Service) {
	h.cache = c
}

// GetMyPosts handles GET /api/v1/my/posts
func (h *MyPageHandler) GetMyPosts(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	page, limit := parseMyPagePagination(c)

	posts, total, err := h.myPageRepo.FindPostsByMember(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "내 글 조회에 실패했습니다", err)
		return
	}

	items := make([]map[string]interface{}, 0, len(posts))
	for _, p := range posts {
		items = append(items, p.ToPostResponse())
	}

	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, limit, total))
}

// GetMyComments handles GET /api/v1/my/comments
func (h *MyPageHandler) GetMyComments(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	page, limit := parseMyPagePagination(c)

	comments, total, err := h.myPageRepo.FindCommentsByMember(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "내 댓글 조회에 실패했습니다", err)
		return
	}

	items := make([]map[string]interface{}, 0, len(comments))
	for _, c := range comments {
		items = append(items, c.ToCommentResponse())
	}

	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, limit, total))
}

// GetMyLikedPosts handles GET /api/v1/my/liked-posts
func (h *MyPageHandler) GetMyLikedPosts(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	page, limit := parseMyPagePagination(c)

	posts, total, err := h.myPageRepo.FindLikedPostsByMember(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "추천한 글 조회에 실패했습니다", err)
		return
	}

	items := make([]map[string]interface{}, 0, len(posts))
	for _, p := range posts {
		items = append(items, p.ToPostResponse())
	}

	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, limit, total))
}

// GetBoardStats handles GET /api/v1/my/stats
func (h *MyPageHandler) GetBoardStats(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	stats, err := h.myPageRepo.GetBoardStats(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "통계 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, stats)
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var emoRe = regexp.MustCompile(`\{emo:[^}]+\}`)
var whitespaceRe = regexp.MustCompile(`\s+`)

// stripHTMLPreview removes HTML tags, emoji codes, HTML entities and truncates
func stripHTMLPreview(content string, maxLen int) string {
	s := htmlTagRe.ReplaceAllString(content, "")
	s = emoRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = whitespaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

// GetMemberActivity handles GET /api/v1/members/:mb_id/activity
// Results are cached in Redis for 30s to prevent UNION ALL storms under high concurrency.
func (h *MyPageHandler) GetMemberActivity(c *gin.Context) {
	mbID := c.Param("id")
	emptyResponse := gin.H{"recentPosts": []interface{}{}, "recentComments": []interface{}{}}
	if mbID == "" {
		c.JSON(http.StatusBadRequest, emptyResponse)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}

	// Try Redis cache first
	cacheKey := fmt.Sprintf("activity:%s:%d", mbID, limit)
	if h.cache != nil {
		var cached json.RawMessage
		if err := h.cache.Get(context.Background(), cacheKey, &cached); err == nil {
			c.Data(http.StatusOK, "application/json", cached)
			return
		}
	}

	// Get board subjects map
	boards, err := h.myPageRepo.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		c.JSON(http.StatusOK, emptyResponse)
		return
	}
	boardSubjects := make(map[string]string, len(boards))
	for _, b := range boards {
		boardSubjects[b.BoTable] = b.BoSubject
	}

	// Parallel fetch posts and comments
	var (
		wg       sync.WaitGroup
		posts    []map[string]interface{}
		comments []map[string]interface{}
		postsErr error
		commsErr error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		rawPosts, fetchErr := h.myPageRepo.FindPublicPostsByMember(mbID, limit)
		if fetchErr != nil {
			postsErr = fetchErr
			return
		}
		posts = make([]map[string]interface{}, 0, len(rawPosts))
		for _, p := range rawPosts {
			posts = append(posts, map[string]interface{}{
				"bo_table":    p.BoardID,
				"bo_subject":  boardSubjects[p.BoardID],
				"wr_id":       p.WrID,
				"wr_subject":  p.WrSubject,
				"wr_datetime": p.WrDatetime.Format("2006-01-02 15:04:05"),
				"href":        fmt.Sprintf("/%s/%d", p.BoardID, p.WrID),
			})
		}
	}()
	go func() {
		defer wg.Done()
		rawComments, fetchErr := h.myPageRepo.FindPublicCommentsByMember(mbID, limit)
		if fetchErr != nil {
			commsErr = fetchErr
			return
		}
		comments = make([]map[string]interface{}, 0, len(rawComments))
		for _, cm := range rawComments {
			comments = append(comments, map[string]interface{}{
				"bo_table":     cm.BoardID,
				"bo_subject":   boardSubjects[cm.BoardID],
				"wr_id":        cm.WrID,
				"parent_wr_id": cm.WrParent,
				"preview":      stripHTMLPreview(cm.WrContent, 80),
				"wr_datetime":  cm.WrDatetime.Format("2006-01-02 15:04:05"),
				"href":         fmt.Sprintf("/%s/%d#c_%d", cm.BoardID, cm.WrParent, cm.WrID),
			})
		}
	}()
	wg.Wait()

	if postsErr != nil || commsErr != nil {
		c.JSON(http.StatusOK, emptyResponse)
		return
	}
	if posts == nil {
		posts = make([]map[string]interface{}, 0)
	}
	if comments == nil {
		comments = make([]map[string]interface{}, 0)
	}

	result := gin.H{
		"recentPosts":    posts,
		"recentComments": comments,
	}

	// Cache in Redis (best-effort, ignore errors)
	if h.cache != nil {
		_ = h.cache.Set(context.Background(), cacheKey, result, activityCacheTTL)
	}

	c.JSON(http.StatusOK, result)
}

func parseMyPagePagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}
