package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// ScrapRepository v2 scrap data access
type ScrapRepository interface {
	Create(scrap *v2.V2Scrap) error
	Delete(userID, postID uint64) error
	Exists(userID, postID uint64) (bool, error)
	FindByUser(userID uint64, page, limit int) ([]*v2.V2Scrap, int64, error)
}

type scrapRepository struct {
	db *gorm.DB
}

// NewScrapRepository creates a new v2 ScrapRepository
func NewScrapRepository(db *gorm.DB) ScrapRepository {
	return &scrapRepository{db: db}
}

func (r *scrapRepository) Create(scrap *v2.V2Scrap) error {
	return r.db.Create(scrap).Error
}

func (r *scrapRepository) Delete(userID, postID uint64) error {
	return r.db.Where("user_id = ? AND post_id = ?", userID, postID).Delete(&v2.V2Scrap{}).Error
}

func (r *scrapRepository) Exists(userID, postID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&v2.V2Scrap{}).Where("user_id = ? AND post_id = ?", userID, postID).Count(&count).Error
	return count > 0, err
}

func (r *scrapRepository) FindByUser(userID uint64, page, limit int) ([]*v2.V2Scrap, int64, error) {
	var scraps []*v2.V2Scrap
	var total int64

	query := r.db.Model(&v2.V2Scrap{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&scraps).Error; err != nil {
		return nil, 0, err
	}
	return scraps, total, nil
}
