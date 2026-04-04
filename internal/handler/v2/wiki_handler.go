package v2

import (
	"net/http"
	"strconv"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// WikiHandler 위키 관련 API 핸들러
type WikiHandler struct {
	backlinkRepo *v2repo.WikiBacklinkRepository
}

// NewWikiHandler 위키 핸들러 생성
func NewWikiHandler(backlinkRepo *v2repo.WikiBacklinkRepository) *WikiHandler {
	return &WikiHandler{backlinkRepo: backlinkRepo}
}

// GetBacklinks 특정 게시글을 참조하는 백링크 목록 조회
// GET /api/v2/posts/:id/backlinks
func (h *WikiHandler) GetBacklinks(c *gin.Context) {
	postID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid post ID"})
		return
	}

	backlinks, err := h.backlinkRepo.FindByTargetID(postID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch backlinks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    backlinks,
	})
}
