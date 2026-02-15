package repository

import (
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"gorm.io/gorm"
)

// CategoryRepository 카테고리 저장소 인터페이스
type CategoryRepository interface {
	Create(category *domain.Category) error
	FindByID(id uint64) (*domain.Category, error)
	FindBySlug(slug string) (*domain.Category, error)
	Update(category *domain.Category) error
	Delete(id uint64) error
	ListAll() ([]*domain.Category, error)
	ListActive() ([]*domain.Category, error)
	ListRoots() ([]*domain.Category, error)
	ListByParent(parentID uint64) ([]*domain.Category, error)
	IncrementItemCount(id uint64, delta int) error
}

type categoryRepository struct {
	db *gorm.DB
}

// NewCategoryRepository 카테고리 저장소 생성
func NewCategoryRepository(db *gorm.DB) CategoryRepository {
	return &categoryRepository{db: db}
}

func (r *categoryRepository) Create(category *domain.Category) error {
	return r.db.Create(category).Error
}

func (r *categoryRepository) FindByID(id uint64) (*domain.Category, error) {
	var category domain.Category
	err := r.db.Where("id = ?", id).First(&category).Error
	if err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *categoryRepository) FindBySlug(slug string) (*domain.Category, error) {
	var category domain.Category
	err := r.db.Where("slug = ?", slug).First(&category).Error
	if err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *categoryRepository) Update(category *domain.Category) error {
	return r.db.Save(category).Error
}

func (r *categoryRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Category{}, id).Error
}

func (r *categoryRepository) ListAll() ([]*domain.Category, error) {
	var categories []*domain.Category
	err := r.db.Order("order_num ASC, name ASC").Find(&categories).Error
	return categories, err
}

func (r *categoryRepository) ListActive() ([]*domain.Category, error) {
	var categories []*domain.Category
	err := r.db.Where("is_active = ?", true).Order("order_num ASC, name ASC").Find(&categories).Error
	return categories, err
}

func (r *categoryRepository) ListRoots() ([]*domain.Category, error) {
	var categories []*domain.Category
	err := r.db.Where("parent_id IS NULL AND is_active = ?", true).
		Preload("Children", "is_active = ?", true).
		Order("order_num ASC, name ASC").
		Find(&categories).Error
	return categories, err
}

func (r *categoryRepository) ListByParent(parentID uint64) ([]*domain.Category, error) {
	var categories []*domain.Category
	err := r.db.Where("parent_id = ? AND is_active = ?", parentID, true).
		Order("order_num ASC, name ASC").
		Find(&categories).Error
	return categories, err
}

func (r *categoryRepository) IncrementItemCount(id uint64, delta int) error {
	return r.db.Model(&domain.Category{}).Where("id = ?", id).
		UpdateColumn("item_count", gorm.Expr("GREATEST(0, item_count + ?)", delta)).Error
}
