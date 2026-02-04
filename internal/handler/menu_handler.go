package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MenuHandler handles HTTP requests for menus
type MenuHandler struct {
	service service.MenuService
}

// NewMenuHandler creates a new MenuHandler
func NewMenuHandler(service service.MenuService) *MenuHandler {
	return &MenuHandler{service: service}
}

// GetMenus handles GET /api/v2/menus
// Returns both sidebar and header menus
func (h *MenuHandler) GetMenus(c *gin.Context) {
	data, err := h.service.GetMenus()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch menus", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// GetSidebarMenus handles GET /api/v2/menus/sidebar
// Returns only sidebar menus
func (h *MenuHandler) GetSidebarMenus(c *gin.Context) {
	data, err := h.service.GetSidebarMenus()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch sidebar menus", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// GetHeaderMenus handles GET /api/v2/menus/header
// Returns only header menus
func (h *MenuHandler) GetHeaderMenus(c *gin.Context) {
	data, err := h.service.GetHeaderMenus()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch header menus", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// ============================================
// Admin Menu Handlers
// ============================================

// GetAllMenusForAdmin handles GET /api/v2/admin/menus
// Returns all menus (includes inactive) for admin management
func (h *MenuHandler) GetAllMenusForAdmin(c *gin.Context) {
	data, err := h.service.GetAllForAdmin()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch menus", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// CreateMenu handles POST /api/v2/admin/menus
// Creates a new menu
func (h *MenuHandler) CreateMenu(c *gin.Context) {
	var req domain.CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	data, err := h.service.CreateMenu(&req)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to create menu", err)
		return
	}

	c.JSON(http.StatusCreated, common.APIResponse{Data: data})
}

// UpdateMenu handles PUT /api/v2/admin/menus/:id
// Updates an existing menu
func (h *MenuHandler) UpdateMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid menu ID", err)
		return
	}

	var req domain.UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	data, err := h.service.UpdateMenu(id, &req)
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to update menu", err)
		return
	}

	common.SuccessResponse(c, data, nil)
}

// DeleteMenu handles DELETE /api/v2/admin/menus/:id
// Deletes a menu
func (h *MenuHandler) DeleteMenu(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid menu ID", err)
		return
	}

	if err := h.service.DeleteMenu(id); err != nil {
		common.ErrorResponse(c, 500, "Failed to delete menu", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: map[string]string{"message": "Menu deleted successfully"}})
}

// ReorderMenus handles POST /api/v2/admin/menus/reorder
// Reorders menus based on the provided items
func (h *MenuHandler) ReorderMenus(c *gin.Context) {
	var req domain.ReorderMenusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if err := h.service.ReorderMenus(&req); err != nil {
		common.ErrorResponse(c, 500, "Failed to reorder menus", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: map[string]string{"message": "Menus reordered successfully"}})
}
