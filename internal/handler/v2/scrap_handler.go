package v2

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// ScrapHandler handles v2 scrap API endpoints
type ScrapHandler struct {
	scrapRepo v2repo.ScrapRepository
}

// NewScrapHandler creates a new ScrapHandler
func NewScrapHandler(scrapRepo v2repo.ScrapRepository) *ScrapHandler {
	return &ScrapHandler{scrapRepo: scrapRepo}
}

// AddScrap handles POST /api/v2/posts/:id/scrap
func (h *ScrapHandler) AddScrap(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	postID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	exists, err := h.scrapRepo.Exists(userID, postID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "스크랩 확인 실패", err)
		return
	}
	if exists {
		common.V2ErrorResponse(c, http.StatusConflict, "이미 스크랩한 게시글입니다", errors.New("already scraped"))
		return
	}

	scrap := &v2domain.V2Scrap{
		UserID: userID,
		PostID: postID,
	}
	if err := h.scrapRepo.Create(scrap); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "스크랩 실패", err)
		return
	}
	common.V2Created(c, scrap)
}

// RemoveScrap handles DELETE /api/v2/posts/:id/scrap
func (h *ScrapHandler) RemoveScrap(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	postID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}

	if err := h.scrapRepo.Delete(userID, postID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "스크랩 취소 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "스크랩 취소 완료"})
}

// ListScraps handles GET /api/v2/me/scraps
func (h *ScrapHandler) ListScraps(c *gin.Context) {
	userID, err := strconv.ParseUint(middleware.GetUserID(c), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	page, perPage := parsePagination(c)
	scraps, total, err := h.scrapRepo.FindByUser(userID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "스크랩 목록 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, scraps, common.NewV2Meta(page, perPage, total))
}
