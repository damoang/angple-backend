package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// MenuRepository 메뉴 저장소 인터페이스
type MenuRepository interface {
	// 조회 (Public)
	GetAll() ([]*domain.Menu, error)
	GetSidebarMenus() ([]*domain.Menu, error)
	GetHeaderMenus() ([]*domain.Menu, error)
	FindByID(id int64) (*domain.Menu, error)

	// Admin CRUD
	GetAllForAdmin() ([]*domain.Menu, error)    // includes inactive
	FindByIDForAdmin(id int64) (*domain.Menu, error) // includes inactive
	Create(menu *domain.Menu) error
	Update(menu *domain.Menu) error
	Delete(id int64) error
	ReorderBulk(items []domain.ReorderMenuItem) error
	GetMaxOrderNumByParent(parentID *int64) (int, error)
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

// ============================================
// Admin Menu CRUD Operations
// ============================================

// FindByIDForAdmin ID로 메뉴 조회 (비활성 메뉴 포함, Admin용)
func (r *menuRepository) FindByIDForAdmin(id int64) (*domain.Menu, error) {
	var menu domain.Menu

	err := r.db.
		Where("id = ?", id).
		First(&menu).Error

	if err != nil {
		return nil, err
	}

	return &menu, nil
}

// GetAllForAdmin 모든 메뉴 조회 (비활성 메뉴 포함, Admin용)
func (r *menuRepository) GetAllForAdmin() ([]*domain.Menu, error) {
	var menus []*domain.Menu

	err := r.db.
		Order("depth ASC, order_num ASC").
		Find(&menus).Error

	if err != nil {
		return nil, err
	}

	return r.buildHierarchy(menus), nil
}

// Create 새 메뉴 생성
func (r *menuRepository) Create(menu *domain.Menu) error {
	return r.db.Create(menu).Error
}

// Update 메뉴 수정
func (r *menuRepository) Update(menu *domain.Menu) error {
	return r.db.Save(menu).Error
}

// Delete 메뉴 삭제 (soft delete 아닌 실제 삭제)
func (r *menuRepository) Delete(id int64) error {
	return r.db.Delete(&domain.Menu{}, id).Error
}

// ReorderBulk 메뉴 순서 일괄 변경 (트랜잭션 사용)
func (r *menuRepository) ReorderBulk(items []domain.ReorderMenuItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			// 각 메뉴의 parent_id, order_num, depth 업데이트
			updates := map[string]interface{}{
				"parent_id": item.ParentID,
				"order_num": item.OrderNum,
			}

			// depth 계산: parent가 없으면 0, 있으면 부모의 depth + 1
			if item.ParentID == nil {
				updates["depth"] = 0
			} else {
				var parentMenu domain.Menu
				if err := tx.First(&parentMenu, *item.ParentID).Error; err == nil {
					updates["depth"] = parentMenu.Depth + 1
				}
			}

			if err := tx.Model(&domain.Menu{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetMaxOrderNumByParent 특정 부모 아래에서 가장 큰 order_num 조회
func (r *menuRepository) GetMaxOrderNumByParent(parentID *int64) (int, error) {
	var maxOrder int

	query := r.db.Model(&domain.Menu{}).Select("COALESCE(MAX(order_num), 0)")

	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	err := query.Scan(&maxOrder).Error
	if err != nil {
		return 0, err
	}

	return maxOrder, nil
}
