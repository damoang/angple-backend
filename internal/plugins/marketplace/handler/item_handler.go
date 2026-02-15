package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/repository"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/service"
	"github.com/gin-gonic/gin"
)

// ItemHandler 상품 핸들러
type ItemHandler struct {
	itemService service.ItemService
}

// NewItemHandler 상품 핸들러 생성
func NewItemHandler(itemService service.ItemService) *ItemHandler {
	return &ItemHandler{
		itemService: itemService,
	}
}

// ListItems 상품 목록 조회
func (h *ItemHandler) ListItems(c *gin.Context) {
	params := &repository.ItemListParams{
		Location:  c.Query("location"),
		Keyword:   c.Query("keyword"),
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
		Page:      1,
		Limit:     20,
	}

	if categoryID, err := strconv.ParseUint(c.Query("category_id"), 10, 64); err == nil {
		params.CategoryID = &categoryID
	}
	if status := c.Query("status"); status != "" {
		s := domain.ItemStatus(status)
		params.Status = &s
	}
	if condition := c.Query("condition"); condition != "" {
		cond := domain.ItemCondition(condition)
		params.Condition = &cond
	}
	if tradeMethod := c.Query("trade_method"); tradeMethod != "" {
		tm := domain.TradeMethod(tradeMethod)
		params.TradeMethod = &tm
	}
	if minPrice, err := strconv.ParseInt(c.Query("min_price"), 10, 64); err == nil {
		params.MinPrice = &minPrice
	}
	if maxPrice, err := strconv.ParseInt(c.Query("max_price"), 10, 64); err == nil {
		params.MaxPrice = &maxPrice
	}
	if page, err := strconv.Atoi(c.Query("page")); err == nil && page > 0 {
		params.Page = page
	}
	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 {
		params.Limit = limit
	}

	// 로그인 사용자 ID 추출
	var viewerID *uint64
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uint64); ok {
			viewerID = &id
		}
	}

	items, meta, err := h.itemService.ListItems(params, viewerID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch items", err)
		return
	}

	common.SuccessResponse(c, items, meta)
}

// GetItem 상품 상세 조회
func (h *ItemHandler) GetItem(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	// 로그인 사용자 ID 추출
	var viewerID *uint64
	if userID, exists := c.Get("user_id"); exists {
		if uid, ok := userID.(uint64); ok {
			viewerID = &uid
		}
	}

	item, err := h.itemService.GetItem(id, viewerID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch item", err)
		return
	}

	common.SuccessResponse(c, item, nil)
}

// CreateItem 상품 등록
func (h *ItemHandler) CreateItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	var req domain.CreateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	item, err := h.itemService.CreateItem(userID.(uint64), &req)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create item", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: item})
}

// UpdateItem 상품 수정
func (h *ItemHandler) UpdateItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	var req domain.UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.itemService.UpdateItem(id, userID.(uint64), &req); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		if errors.Is(err, common.ErrForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Permission denied", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update item", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Item updated"}, nil)
}

// DeleteItem 상품 삭제
func (h *ItemHandler) DeleteItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	if err := h.itemService.DeleteItem(id, userID.(uint64)); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		if errors.Is(err, common.ErrForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Permission denied", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete item", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Item deleted"}, nil)
}

// UpdateStatus 상품 상태 변경
func (h *ItemHandler) UpdateStatus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	var req domain.UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.itemService.UpdateStatus(id, userID.(uint64), &req); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		if errors.Is(err, common.ErrForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Permission denied", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update status", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Status updated"}, nil)
}

// ListMyItems 내 상품 목록 조회
func (h *ItemHandler) ListMyItems(c *gin.Context) {
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

	items, meta, err := h.itemService.ListMyItems(userID.(uint64), page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch items", err)
		return
	}

	common.SuccessResponse(c, items, meta)
}

// BumpItem 상품 끌올
func (h *ItemHandler) BumpItem(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		common.ErrorResponse(c, http.StatusUnauthorized, "Login required", nil)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid item ID", err)
		return
	}

	if err := h.itemService.BumpItem(id, userID.(uint64)); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Item not found", err)
			return
		}
		if errors.Is(err, common.ErrForbidden) {
			common.ErrorResponse(c, http.StatusForbidden, "Permission denied", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to bump item", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Item bumped"}, nil)
}
