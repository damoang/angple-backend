package handler

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// MyPageHandler handles /api/v1/my/* endpoints for user's posts, comments, liked posts, and stats
type MyPageHandler struct {
	myPageRepo gnurepo.MyPageRepository
}

// NewMyPageHandler creates a new MyPageHandler
func NewMyPageHandler(myPageRepo gnurepo.MyPageRepository) *MyPageHandler {
	return &MyPageHandler{myPageRepo: myPageRepo}
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
// Temporarily disabled: returns empty arrays to prevent slow queries
// under high concurrency (~10k concurrent users, ~90 boards).
func (h *MyPageHandler) GetMemberActivity(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"recentPosts":    []interface{}{},
		"recentComments": []interface{}{},
	})
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
