package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// MenuRepository 메뉴 저장소 인터페이스
type MenuRepository interface {
	// 조회
	GetAll() ([]*domain.Menu, error)
	GetSidebarMenus() ([]*domain.Menu, error)
	GetHeaderMenus() ([]*domain.Menu, error)
	FindByID(id int64) (*domain.Menu, error)
}

// menuRepository GORM 구현체
type menuRepository struct {
	db *gorm.DB
}

// NewMenuRepository 생성자
func NewMenuRepository(db *gorm.DB) MenuRepository {
	return &menuRepository{db: db}
}

// GetAll 모든 활성화된 메뉴 조회
func (r *menuRepository) GetAll() ([]*domain.Menu, error) {
	var menus []*domain.Menu

	err := r.db.
		Where("is_active = ?", true).
		Order("depth ASC, order_num ASC").
		Find(&menus).Error

	if err != nil {
		return nil, err
	}

	return r.buildHierarchy(menus), nil
}

// GetSidebarMenus 사이드바 메뉴 조회
func (r *menuRepository) GetSidebarMenus() ([]*domain.Menu, error) {
	var menus []*domain.Menu

	err := r.db.
		Where("is_active = ? AND show_in_sidebar = ?", true, true).
		Order("depth ASC, order_num ASC").
		Find(&menus).Error

	if err != nil {
		return nil, err
	}

	return r.buildHierarchy(menus), nil
}

// GetHeaderMenus 헤더 메뉴 조회
func (r *menuRepository) GetHeaderMenus() ([]*domain.Menu, error) {
	var menus []*domain.Menu

	err := r.db.
		Where("is_active = ? AND show_in_header = ?", true, true).
		Order("depth ASC, order_num ASC").
		Find(&menus).Error

	if err != nil {
		return nil, err
	}

	return r.buildHierarchy(menus), nil
}

// FindByID ID로 메뉴 조회
func (r *menuRepository) FindByID(id int64) (*domain.Menu, error) {
	var menu domain.Menu

	err := r.db.
		Where("id = ? AND is_active = ?", id, true).
		First(&menu).Error

	if err != nil {
		return nil, err
	}

	return &menu, nil
}

// buildHierarchy 평면 메뉴 배열을 계층 구조로 변환
func (r *menuRepository) buildHierarchy(menus []*domain.Menu) []*domain.Menu {
	// Map으로 모든 메뉴 인덱싱
	menuMap := make(map[int64]*domain.Menu)
	for _, menu := range menus {
		menu.Children = make([]*domain.Menu, 0)
		menuMap[menu.ID] = menu
	}

	// 루트 메뉴 수집 및 계층 구조 구축
	var rootMenus []*domain.Menu
	for _, menu := range menus {
		if menu.ParentID == nil {
			// 루트 메뉴
			rootMenus = append(rootMenus, menu)
		} else if parent, exists := menuMap[*menu.ParentID]; exists {
			// 부모가 존재하면 자식으로 추가
			parent.Children = append(parent.Children, menu)
		}
	}

	return rootMenus
}
