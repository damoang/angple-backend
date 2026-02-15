package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// CartRepository 장바구니 저장소 인터페이스
type CartRepository interface {
	// 생성/수정/삭제
	Create(cart *domain.Cart) error
	Update(id uint64, cart *domain.Cart) error
	Delete(id uint64) error
	DeleteByUserID(userID uint64) error

	// 조회
	FindByID(id uint64) (*domain.Cart, error)
	FindByUserAndProduct(userID, productID uint64) (*domain.Cart, error)
	ListByUser(userID uint64) ([]*domain.Cart, error)
	ListByUserWithProducts(userID uint64) ([]*domain.Cart, error)

	// 수량 업데이트
	IncrementQuantity(id uint64, quantity int) error
	SetQuantity(id uint64, quantity int) error
}

// cartRepository GORM 구현체
type cartRepository struct {
	db *gorm.DB
}

// NewCartRepository 생성자
func NewCartRepository(db *gorm.DB) CartRepository {
	return &cartRepository{db: db}
}

// Create 장바구니 아이템 생성
func (r *cartRepository) Create(cart *domain.Cart) error {
	now := time.Now()
	cart.CreatedAt = now
	cart.UpdatedAt = now
	return r.db.Create(cart).Error
}

// Update 장바구니 아이템 수정
func (r *cartRepository) Update(id uint64, cart *domain.Cart) error {
	cart.UpdatedAt = time.Now()
	return r.db.Model(&domain.Cart{}).Where("id = ?", id).Updates(cart).Error
}

// Delete 장바구니 아이템 삭제
func (r *cartRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Cart{}, id).Error
}

// DeleteByUserID 사용자의 모든 장바구니 삭제
func (r *cartRepository) DeleteByUserID(userID uint64) error {
	return r.db.Where("user_id = ?", userID).Delete(&domain.Cart{}).Error
}

// FindByID ID로 장바구니 아이템 조회
func (r *cartRepository) FindByID(id uint64) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.Where("id = ?", id).First(&cart).Error
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

// FindByUserAndProduct 사용자와 상품으로 장바구니 아이템 조회
func (r *cartRepository) FindByUserAndProduct(userID, productID uint64) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.Where("user_id = ? AND product_id = ?", userID, productID).First(&cart).Error
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

// ListByUser 사용자의 장바구니 목록 조회
func (r *cartRepository) ListByUser(userID uint64) ([]*domain.Cart, error) {
	var carts []*domain.Cart
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&carts).Error
	if err != nil {
		return nil, err
	}
	return carts, nil
}

// ListByUserWithProducts 사용자의 장바구니 목록 조회 (상품 정보 포함)
func (r *cartRepository) ListByUserWithProducts(userID uint64) ([]*domain.Cart, error) {
	var carts []*domain.Cart
	err := r.db.Preload("Product").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&carts).Error
	if err != nil {
		return nil, err
	}
	return carts, nil
}

// IncrementQuantity 장바구니 수량 증가
func (r *cartRepository) IncrementQuantity(id uint64, quantity int) error {
	return r.db.Model(&domain.Cart{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"quantity":   gorm.Expr("quantity + ?", quantity),
			"updated_at": time.Now(),
		}).Error
}

// SetQuantity 장바구니 수량 설정
func (r *cartRepository) SetQuantity(id uint64, quantity int) error {
	return r.db.Model(&domain.Cart{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"quantity":   quantity,
			"updated_at": time.Now(),
		}).Error
}
