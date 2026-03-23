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
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const activityCacheTTL = 60 * time.Second

// MyPageHandler handles /api/v1/my/* endpoints for user's posts, comments, liked posts, and stats
type MyPageHandler struct {
	myPageRepo  gnurepo.MyPageRepository
	redisClient *redis.Client
}

// NewMyPageHandler creates a new MyPageHandler
func NewMyPageHandler(myPageRepo gnurepo.MyPageRepository) *MyPageHandler {
	return &MyPageHandler{myPageRepo: myPageRepo}
}

// SetRedisClient injects Redis client for response caching
func (h *MyPageHandler) SetRedisClient(client *redis.Client) {
	h.redisClient = client
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

// activityResponse is the JSON structure for member activity (cached in Redis)
type activityResponse struct {
	RecentPosts    []map[string]interface{} `json:"recentPosts"`
	RecentComments []map[string]interface{} `json:"recentComments"`
}

func formatNullableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// GetMemberActivity handles GET /api/v1/members/:id/activity
func (h *MyPageHandler) GetMemberActivity(c *gin.Context) {
	mbID := c.Param("id")
	if mbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"recentPosts": []interface{}{}, "recentComments": []interface{}{}})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}

	// 1. Try Redis cache (DB 0 queries on hit)
	cacheKey := fmt.Sprintf("member_activity:%s:%d", mbID, limit)
	if h.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if cached, err := h.redisClient.Get(ctx, cacheKey).Bytes(); err == nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", cached)
			return
		}
	}

	// 2. Cache miss — get board subjects map (memory cached, 5 min TTL)
	boards, err := h.myPageRepo.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		c.JSON(http.StatusOK, gin.H{"recentPosts": []interface{}{}, "recentComments": []interface{}{}})
		return
	}
	boardSubjects := make(map[string]string, len(boards))
	for _, b := range boards {
		boardSubjects[b.BoTable] = b.BoSubject
	}

	// 3. Parallel fetch posts and comments (2 UNION ALL queries)
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
		rawPosts, err := h.myPageRepo.FindPublicPostsByMember(mbID, limit)
		if err != nil {
			postsErr = err
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
				"deleted_at":  formatNullableTime(p.DeletedAt),
				"href":        fmt.Sprintf("/%s/%d", p.BoardID, p.WrID),
			})
		}
	}()
	go func() {
		defer wg.Done()
		rawComments, err := h.myPageRepo.FindPublicCommentsByMember(mbID, limit)
		if err != nil {
			commsErr = err
			return
		}
		comments = make([]map[string]interface{}, 0, len(rawComments))
		for _, cm := range rawComments {
			comments = append(comments, map[string]interface{}{
				"bo_table":        cm.BoardID,
				"bo_subject":      boardSubjects[cm.BoardID],
				"wr_id":           cm.WrID,
				"parent_wr_id":    cm.WrParent,
				"preview":         stripHTMLPreview(cm.WrContent, 80),
				"wr_datetime":     cm.WrDatetime.Format("2006-01-02 15:04:05"),
				"deleted_at":      formatNullableTime(cm.DeletedAt),
				"post_deleted_at": formatNullableTime(cm.ParentDeletedAt),
				"href":            fmt.Sprintf("/%s/%d#c_%d", cm.BoardID, cm.WrParent, cm.WrID),
			})
		}
	}()
	wg.Wait()

	if postsErr != nil || commsErr != nil {
		c.JSON(http.StatusOK, gin.H{"recentPosts": []interface{}{}, "recentComments": []interface{}{}})
		return
	}
	if posts == nil {
		posts = make([]map[string]interface{}, 0)
	}
	if comments == nil {
		comments = make([]map[string]interface{}, 0)
	}

	resp := activityResponse{RecentPosts: posts, RecentComments: comments}

	// 4. Store in Redis (60s TTL, non-blocking)
	if h.redisClient != nil {
		if data, err := json.Marshal(resp); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			h.redisClient.Set(ctx, cacheKey, data, activityCacheTTL)
		}
	}

	c.JSON(http.StatusOK, resp)
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
