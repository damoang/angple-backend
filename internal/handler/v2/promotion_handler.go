package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// PromotionHandler handles promotion post API endpoints
type PromotionHandler struct {
	promotionRepo v2repo.PromotionRepository
}

// NewPromotionHandler creates a new PromotionHandler
func NewPromotionHandler(promotionRepo v2repo.PromotionRepository) *PromotionHandler {
	return &PromotionHandler{promotionRepo: promotionRepo}
}

// GetInsertPosts handles GET /api/v1/promotion/posts/insert?count=N
// Returns promotion posts to be inserted into board lists
// TODO: v2 마이그레이션 - DB 재설계 후 /api/v2/promotion/posts/insert로 전환
func (h *PromotionHandler) GetInsertPosts(c *gin.Context) {
	count := 3 // default
	if n, err := strconv.Atoi(c.Query("count")); err == nil && n > 0 && n <= 20 {
		count = n
	}

	posts, err := h.promotionRepo.FindInsertPosts(count)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "직홍 게시글 조회 실패", err)
		return
	}

	common.V2Success(c, posts)
}
