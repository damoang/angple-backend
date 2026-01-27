package service

import (
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// MenuService business logic for menus
type MenuService interface {
	// Public API
	GetMenus() (*domain.MenuListResponse, error)
	GetSidebarMenus() ([]domain.MenuResponse, error)
	GetHeaderMenus() ([]domain.MenuResponse, error)

	// Admin API
	GetAllForAdmin() ([]domain.AdminMenuResponse, error)
	CreateMenu(req *domain.CreateMenuRequest) (*domain.AdminMenuResponse, error)
	UpdateMenu(id int64, req *domain.UpdateMenuRequest) (*domain.AdminMenuResponse, error)
	DeleteMenu(id int64) error
	ReorderMenus(req *domain.ReorderMenusRequest) error
}

type menuService struct {
	repo repository.MenuRepository
}

// NewMenuService creates a new MenuService
func NewMenuService(repo repository.MenuRepository) MenuService {
	return &menuService{repo: repo}
}

// GetMenus retrieves all menus (both sidebar and header)
func (s *menuService) GetMenus() (*domain.MenuListResponse, error) {
	sidebarMenus, err := s.GetSidebarMenus()
	if err != nil {
		return nil, err
	}

	headerMenus, err := s.GetHeaderMenus()
	if err != nil {
		return nil, err
	}

	return &domain.MenuListResponse{
		Sidebar: sidebarMenus,
		Header:  headerMenus,
	}, nil
}

// GetSidebarMenus retrieves sidebar menus
func (s *menuService) GetSidebarMenus() ([]domain.MenuResponse, error) {
	menus, err := s.repo.GetSidebarMenus()
	if err != nil {
		return nil, err
	}

	// Convert to response
	responses := make([]domain.MenuResponse, len(menus))
	for i, menu := range menus {
		responses[i] = menu.ToResponse()
	}

	return responses, nil
}

// GetHeaderMenus retrieves header menus
func (s *menuService) GetHeaderMenus() ([]domain.MenuResponse, error) {
	menus, err := s.repo.GetHeaderMenus()
	if err != nil {
		return nil, err
	}

	// Convert to response
	responses := make([]domain.MenuResponse, len(menus))
	for i, menu := range menus {
		responses[i] = menu.ToResponse()
	}

	return responses, nil
}

// ============================================
// Admin Menu CRUD Operations
// ============================================

// GetAllForAdmin retrieves all menus for admin (includes inactive)
func (s *menuService) GetAllForAdmin() ([]domain.AdminMenuResponse, error) {
	menus, err := s.repo.GetAllForAdmin()
	if err != nil {
		return nil, err
	}

	// Convert to admin response
	responses := make([]domain.AdminMenuResponse, len(menus))
	for i, menu := range menus {
		responses[i] = menu.ToAdminResponse()
	}

	return responses, nil
}

// CreateMenu creates a new menu
func (s *menuService) CreateMenu(req *domain.CreateMenuRequest) (*domain.AdminMenuResponse, error) {
	// Calculate depth based on parent
	depth := 0
	if req.ParentID != nil {
		parent, err := s.repo.FindByID(*req.ParentID)
		if err != nil {
			return nil, err
		}
		depth = parent.Depth + 1
	}

	// Get next order number
	maxOrder, err := s.repo.GetMaxOrderNumByParent(req.ParentID)
	if err != nil {
		return nil, err
	}

	// Create menu entity
	menu := &domain.Menu{
		ParentID:      req.ParentID,
		Title:         req.Title,
		URL:           req.URL,
		Icon:          req.Icon,
		Shortcut:      req.Shortcut,
		Description:   req.Description,
		Target:        req.Target,
		Depth:         depth,
		OrderNum:      maxOrder + 1,
		ViewLevel:     req.ViewLevel,
		ShowInHeader:  req.ShowInHeader,
		ShowInSidebar: req.ShowInSidebar,
		IsActive:      req.IsActive,
	}

	if err := s.repo.Create(menu); err != nil {
		return nil, err
	}

	response := menu.ToAdminResponse()
	return &response, nil
}

// UpdateMenu updates an existing menu
func (s *menuService) UpdateMenu(id int64, req *domain.UpdateMenuRequest) (*domain.AdminMenuResponse, error) {
	// Find existing menu
	menu, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.ParentID != nil {
		menu.ParentID = req.ParentID
		// Recalculate depth
		if req.ParentID == nil {
			menu.Depth = 0
		} else {
			parent, err := s.repo.FindByID(*req.ParentID)
			if err != nil {
				return nil, err
			}
			menu.Depth = parent.Depth + 1
		}
	}

	if req.Title != nil {
		menu.Title = *req.Title
	}
	if req.URL != nil {
		menu.URL = *req.URL
	}
	if req.Icon != nil {
		menu.Icon = *req.Icon
	}
	if req.Shortcut != nil {
		menu.Shortcut = *req.Shortcut
	}
	if req.Description != nil {
		menu.Description = *req.Description
	}
	if req.Target != nil {
		menu.Target = *req.Target
	}
	if req.ShowInHeader != nil {
		menu.ShowInHeader = *req.ShowInHeader
	}
	if req.ShowInSidebar != nil {
		menu.ShowInSidebar = *req.ShowInSidebar
	}
	if req.ViewLevel != nil {
		menu.ViewLevel = *req.ViewLevel
	}
	if req.IsActive != nil {
		menu.IsActive = *req.IsActive
	}

	if err := s.repo.Update(menu); err != nil {
		return nil, err
	}

	response := menu.ToAdminResponse()
	return &response, nil
}

// DeleteMenu deletes a menu by ID
func (s *menuService) DeleteMenu(id int64) error {
	// Verify menu exists
	_, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}

	return s.repo.Delete(id)
}

// ReorderMenus reorders menus based on the provided items
func (s *menuService) ReorderMenus(req *domain.ReorderMenusRequest) error {
	return s.repo.ReorderBulk(req.Items)
}
