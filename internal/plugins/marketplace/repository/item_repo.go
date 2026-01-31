package repository

import (
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"gorm.io/gorm"
)

// ItemRepository 상품 저장소 인터페이스
type ItemRepository interface {
	Create(item *domain.Item) error
	FindByID(id uint64) (*domain.Item, error)
	Update(item *domain.Item) error
	Delete(id uint64) error
	List(params *ItemListParams) ([]*domain.Item, int64, error)
	ListBySeller(sellerID uint64, page, limit int) ([]*domain.Item, int64, error)
	IncrementViewCount(id uint64) error
	IncrementWishCount(id uint64, delta int) error
	UpdateStatus(id uint64, status domain.ItemStatus, buyerID *uint64) error
	BumpItem(id uint64) error
}

// ItemListParams 상품 목록 조회 파라미터
type ItemListParams struct {
	CategoryID  *uint64
	Status      *domain.ItemStatus
	Condition   *domain.ItemCondition
	TradeMethod *domain.TradeMethod
	MinPrice    *int64
	MaxPrice    *int64
	Location    string
	Keyword     string
	SortBy      string // created_at, price, view_count, wish_count
	SortOrder   string // asc, desc
	Page        int
	Limit       int
}

type itemRepository struct {
	db *gorm.DB
}

// NewItemRepository 상품 저장소 생성
func NewItemRepository(db *gorm.DB) ItemRepository {
	return &itemRepository{db: db}
}

func (r *itemRepository) Create(item *domain.Item) error {
	return r.db.Create(item).Error
}

func (r *itemRepository) FindByID(id uint64) (*domain.Item, error) {
	var item domain.Item
	err := r.db.Preload("Category").Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *itemRepository) Update(item *domain.Item) error {
	return r.db.Save(item).Error
}

func (r *itemRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Item{}, id).Error
}

func (r *itemRepository) List(params *ItemListParams) ([]*domain.Item, int64, error) {
	query := r.db.Model(&domain.Item{}).Preload("Category")

	// 필터 적용
	if params.CategoryID != nil {
		query = query.Where("category_id = ?", *params.CategoryID)
	}
	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	} else {
		// 기본적으로 숨김 상품 제외
		query = query.Where("status != ?", domain.ItemStatusHidden)
	}
	if params.Condition != nil {
		query = query.Where("condition = ?", *params.Condition)
	}
	if params.TradeMethod != nil {
		query = query.Where("trade_method = ?", *params.TradeMethod)
	}
	if params.MinPrice != nil {
		query = query.Where("price >= ?", *params.MinPrice)
	}
	if params.MaxPrice != nil {
		query = query.Where("price <= ?", *params.MaxPrice)
	}
	if params.Location != "" {
		query = query.Where("location LIKE ?", "%"+params.Location+"%")
	}
	if params.Keyword != "" {
		query = query.Where("(title LIKE ? OR description LIKE ?)",
			"%"+params.Keyword+"%", "%"+params.Keyword+"%")
	}

	// 총 개수 조회
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	orderClause := "created_at DESC"
	if params.SortBy != "" {
		order := "DESC"
		if params.SortOrder == "asc" {
			order = "ASC"
		}
		switch params.SortBy {
		case "price":
			orderClause = "price " + order
		case "view_count":
			orderClause = "view_count " + order
		case "wish_count":
			orderClause = "wish_count " + order
		default:
			orderClause = "created_at " + order
		}
	}
	query = query.Order(orderClause)

	// 페이지네이션
	offset := (params.Page - 1) * params.Limit
	query = query.Offset(offset).Limit(params.Limit)

	var items []*domain.Item
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *itemRepository) ListBySeller(sellerID uint64, page, limit int) ([]*domain.Item, int64, error) {
	var items []*domain.Item
	var total int64

	query := r.db.Model(&domain.Item{}).Where("seller_id = ?", sellerID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Preload("Category").Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *itemRepository) IncrementViewCount(id uint64) error {
	return r.db.Model(&domain.Item{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

func (r *itemRepository) IncrementWishCount(id uint64, delta int) error {
	return r.db.Model(&domain.Item{}).Where("id = ?", id).
		UpdateColumn("wish_count", gorm.Expr("wish_count + ?", delta)).Error
}

func (r *itemRepository) UpdateStatus(id uint64, status domain.ItemStatus, buyerID *uint64) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if status == domain.ItemStatusSold && buyerID != nil {
		updates["buyer_id"] = buyerID
		updates["sold_at"] = gorm.Expr("NOW()")
	}
	return r.db.Model(&domain.Item{}).Where("id = ?", id).Updates(updates).Error
}

func (r *itemRepository) BumpItem(id uint64) error {
	return r.db.Model(&domain.Item{}).Where("id = ?", id).
		UpdateColumn("bumped_at", gorm.Expr("NOW()")).Error
}
