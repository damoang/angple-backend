package handler

import (
	"github.com/damoang/angple-backend/internal/common"
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
		common.ErrorResponse(c, 500, "Failed to fetch menus", err); return
	}

	common.SuccessResponse(c, data, nil); return
}

// GetSidebarMenus handles GET /api/v2/menus/sidebar
// Returns only sidebar menus
func (h *MenuHandler) GetSidebarMenus(c *gin.Context) {
	data, err := h.service.GetSidebarMenus()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch sidebar menus", err); return
	}

	common.SuccessResponse(c, data, nil); return
}

// GetHeaderMenus handles GET /api/v2/menus/header
// Returns only header menus
func (h *MenuHandler) GetHeaderMenus(c *gin.Context) {
	data, err := h.service.GetHeaderMenus()
	if err != nil {
		common.ErrorResponse(c, 500, "Failed to fetch header menus", err); return
	}

	common.SuccessResponse(c, data, nil); return
}
