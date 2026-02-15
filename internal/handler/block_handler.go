package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// BlockHandler handles member block HTTP requests
type BlockHandler struct {
	service service.BlockService
}

// NewBlockHandler creates a new BlockHandler
func NewBlockHandler(service service.BlockService) *BlockHandler {
	return &BlockHandler{service: service}
}

// BlockMember handles POST /members/:user_id/block
// @Summary 회원 차단
// @Tags block
// @Produce json
// @Param user_id path string true "차단할 회원 ID"
// @Success 200 {object} common.APIResponse{data=domain.BlockResponse}
// @Router /members/{user_id}/block [post]
func (h *BlockHandler) BlockMember(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	targetID := c.Param("user_id")
	if targetID == "" {
		common.ErrorResponse(c, http.StatusBadRequest, "대상 회원 ID가 필요합니다", nil)
		return
	}

	result, err := h.service.BlockMember(userID, targetID)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// UnblockMember handles DELETE /members/:user_id/block
// @Summary 차단 해제
// @Tags block
// @Produce json
// @Param user_id path string true "차단 해제할 회원 ID"
// @Success 204
// @Router /members/{user_id}/block [delete]
func (h *BlockHandler) UnblockMember(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	targetID := c.Param("user_id")
	if err := h.service.UnblockMember(userID, targetID); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListBlocks handles GET /members/me/blocks
// @Summary 차단 목록
// @Tags block
// @Produce json
// @Success 200 {object} common.APIResponse{data=[]domain.BlockResponse}
// @Router /members/me/blocks [get]
func (h *BlockHandler) ListBlocks(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	blocks, err := h.service.ListBlocks(userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "차단 목록 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: blocks})
}
