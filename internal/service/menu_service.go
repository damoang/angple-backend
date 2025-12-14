package service

import (
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// MenuService business logic for menus
type MenuService interface {
	GetMenus() (*domain.MenuListResponse, error)
	GetSidebarMenus() ([]domain.MenuResponse, error)
	GetHeaderMenus() ([]domain.MenuResponse, error)
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
