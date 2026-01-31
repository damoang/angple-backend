package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/service"
	"github.com/gin-gonic/gin"
)

// WishHandler 찜하기 핸들러
type WishHandler struct {
	wishService service.WishService
}

// NewWishHandler 찜하기 핸들러 생성
func NewWishHandler(wishService service.WishService) *WishHandler {
	return &WishHandler{
		wishService: wishService,
	}
}

// ToggleWish 찜하기 토글
func (h *WishHandler) ToggleWish(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	itemID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	isWished, err := h.wishService.ToggleWish(userID.(uint64), itemID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to toggle wish", err)
		return
	}

	common.SuccessResponse(c, gin.H{
		"is_wished": isWished,
		"message":   getWishMessage(isWished),
	}, nil)
}

// ListWishes 찜 목록 조회
func (h *WishHandler) ListWishes(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	page := 1
	limit := 20
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	wishes, meta, err := h.wishService.ListWishes(userID.(uint64), page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch wishes", err)
		return
	}

	common.SuccessResponse(c, wishes, meta)
}

func getWishMessage(isWished bool) string {
	if isWished {
		return "찜 목록에 추가되었습니다"
	}
	return "찜 목록에서 제거되었습니다"
}
