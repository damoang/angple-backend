package repository

import (
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"gorm.io/gorm"
)

// WishRepository 찜하기 저장소 인터페이스
type WishRepository interface {
	Create(wish *domain.Wish) error
	Delete(userID, itemID uint64) error
	Exists(userID, itemID uint64) (bool, error)
	ListByUser(userID uint64, page, limit int) ([]*domain.Wish, int64, error)
	GetWishedItemIDs(userID uint64, itemIDs []uint64) (map[uint64]bool, error)
	DeleteByItem(itemID uint64) error
}

type wishRepository struct {
	db *gorm.DB
}

// NewWishRepository 찜하기 저장소 생성
func NewWishRepository(db *gorm.DB) WishRepository {
	return &wishRepository{db: db}
}

func (r *wishRepository) Create(wish *domain.Wish) error {
	return r.db.Create(wish).Error
}

func (r *wishRepository) Delete(userID, itemID uint64) error {
	return r.db.Where("user_id = ? AND item_id = ?", userID, itemID).
		Delete(&domain.Wish{}).Error
}

func (r *wishRepository) Exists(userID, itemID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Wish{}).
		Where("user_id = ? AND item_id = ?", userID, itemID).
		Count(&count).Error
	return count > 0, err
}

func (r *wishRepository) ListByUser(userID uint64, page, limit int) ([]*domain.Wish, int64, error) {
	var wishes []*domain.Wish
	var total int64

	query := r.db.Model(&domain.Wish{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.Preload("Item").Preload("Item.Category").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&wishes).Error

	if err != nil {
		return nil, 0, err
	}

	return wishes, total, nil
}

func (r *wishRepository) GetWishedItemIDs(userID uint64, itemIDs []uint64) (map[uint64]bool, error) {
	if len(itemIDs) == 0 {
		return make(map[uint64]bool), nil
	}

	var wishes []domain.Wish
	err := r.db.Select("item_id").
		Where("user_id = ? AND item_id IN ?", userID, itemIDs).
		Find(&wishes).Error

	if err != nil {
		return nil, err
	}

	result := make(map[uint64]bool)
	for _, wish := range wishes {
		result[wish.ItemID] = true
	}

	return result, nil
}

func (r *wishRepository) DeleteByItem(itemID uint64) error {
	return r.db.Where("item_id = ?", itemID).Delete(&domain.Wish{}).Error
}
