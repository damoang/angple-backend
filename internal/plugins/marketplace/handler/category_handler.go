package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/service"
	"github.com/gin-gonic/gin"
)

// CategoryHandler 카테고리 핸들러
type CategoryHandler struct {
	categoryService service.CategoryService
}

// NewCategoryHandler 카테고리 핸들러 생성
func NewCategoryHandler(categoryService service.CategoryService) *CategoryHandler {
	return &CategoryHandler{
		categoryService: categoryService,
	}
}

// ListCategories 카테고리 목록 조회
func (h *CategoryHandler) ListCategories(c *gin.Context) {
	categories, err := h.categoryService.ListCategories()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch categories", err)
		return
	}

	common.SuccessResponse(c, categories, nil)
}

// ListCategoryTree 카테고리 트리 조회
func (h *CategoryHandler) ListCategoryTree(c *gin.Context) {
	categories, err := h.categoryService.ListCategoryTree()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch categories", err)
		return
	}

	common.SuccessResponse(c, categories, nil)
}

// GetCategory 카테고리 상세 조회
func (h *CategoryHandler) GetCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid category ID", err)
		return
	}

	category, err := h.categoryService.GetCategory(id)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Category not found", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch category", err)
		return
	}

	common.SuccessResponse(c, category, nil)
}

// CreateCategory 카테고리 생성 (관리자 전용)
func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	var req domain.CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	category, err := h.categoryService.CreateCategory(&req)
	if err != nil {
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid category data", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to create category", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: category})
}

// UpdateCategory 카테고리 수정 (관리자 전용)
func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid category ID", err)
		return
	}

	var req domain.UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.categoryService.UpdateCategory(id, &req); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Category not found", err)
			return
		}
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, "Invalid category data", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to update category", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Category updated"}, nil)
}

// DeleteCategory 카테고리 삭제 (관리자 전용)
func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid category ID", err)
		return
	}

	if err := h.categoryService.DeleteCategory(id); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			common.ErrorResponse(c, http.StatusNotFound, "Category not found", err)
			return
		}
		if errors.Is(err, common.ErrBadRequest) {
			common.ErrorResponse(c, http.StatusBadRequest, "Cannot delete category with items or children", err)
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete category", err)
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Category deleted"}, nil)
}
