package v2

import (
	"errors"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// BlockHandler handles v2 block API endpoints
type BlockHandler struct {
	blockRepo v2repo.BlockRepository
}

// NewBlockHandler creates a new BlockHandler
func NewBlockHandler(blockRepo v2repo.BlockRepository) *BlockHandler {
	return &BlockHandler{blockRepo: blockRepo}
}

// BlockMember handles POST /api/v2/members/:id/block
func (h *BlockHandler) BlockMember(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	targetID := c.Param("id")
	if targetID == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "대상 회원 ID가 필요합니다", errors.New("missing target id"))
		return
	}

	if userID == targetID {
		common.V2ErrorResponse(c, http.StatusBadRequest, "자기 자신을 차단할 수 없습니다", errors.New("cannot block self"))
		return
	}

	exists, err := h.blockRepo.Exists(userID, targetID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "차단 확인 실패", err)
		return
	}
	if exists {
		common.V2ErrorResponse(c, http.StatusConflict, "이미 차단한 회원입니다", errors.New("already blocked"))
		return
	}

	block, err := h.blockRepo.Create(userID, targetID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "차단 실패", err)
		return
	}

	common.V2Created(c, gin.H{
		"block_id":   block.ID,
		"user_id":    targetID,
		"blocked_at": block.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// UnblockMember handles DELETE /api/v2/members/:id/block
func (h *BlockHandler) UnblockMember(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	targetID := c.Param("id")
	if targetID == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "대상 회원 ID가 필요합니다", errors.New("missing target id"))
		return
	}

	if err := h.blockRepo.Delete(userID, targetID); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBlocks handles GET /api/v2/members/me/blocks
func (h *BlockHandler) ListBlocks(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", errors.New("unauthorized"))
		return
	}

	page, perPage := parsePagination(c)
	blocks, total, err := h.blockRepo.FindByMember(userID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "차단 목록 조회 실패", err)
		return
	}

	common.V2SuccessWithMeta(c, blocks, common.NewV2Meta(page, perPage, total))
}
