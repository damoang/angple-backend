package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// PromotionRepository handles promotion post data access
type PromotionRepository interface {
	FindInsertPosts(count int) ([]*v2.PromotionPost, error)
}

type promotionRepository struct {
	db *gorm.DB
}

// NewPromotionRepository creates a new PromotionRepository
func NewPromotionRepository(db *gorm.DB) PromotionRepository {
	return &promotionRepository{db: db}
}

func (r *promotionRepository) FindInsertPosts(count int) ([]*v2.PromotionPost, error) {
	var posts []*v2.PromotionPost

	if err := r.db.Model(&v2.PromotionPost{}).
		Joins("JOIN advertisers ON advertisers.id = promotion_posts.advertiser_id").
		Where("promotion_posts.is_active = ?", true).
		Where("advertisers.is_active = ?", true).
		Where("(advertisers.start_date IS NULL OR advertisers.start_date <= CURDATE())").
		Where("(advertisers.end_date IS NULL OR advertisers.end_date >= CURDATE())").
		Preload("Advertiser").
		Order("advertisers.is_pinned DESC, RAND()").
		Limit(count).
		Find(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}
