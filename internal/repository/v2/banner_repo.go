package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// BannerRepository handles banner data access
type BannerRepository interface {
	FindActiveByPosition(position string) ([]*v2.Banner, error)
	IncrementClickCount(bannerID uint64) error
	CreateClickLog(log *v2.BannerClickLog) error
}

type bannerRepository struct {
	db *gorm.DB
}

// NewBannerRepository creates a new BannerRepository
func NewBannerRepository(db *gorm.DB) BannerRepository {
	return &bannerRepository{db: db}
}

func (r *bannerRepository) FindActiveByPosition(position string) ([]*v2.Banner, error) {
	var banners []*v2.Banner
	query := r.db.Model(&v2.Banner{}).
		Where("is_active = ?", true).
		Where("(start_date IS NULL OR start_date <= CURDATE())").
		Where("(end_date IS NULL OR end_date >= CURDATE())")

	if position != "" {
		query = query.Where("position = ?", position)
	}

	if err := query.Order("priority DESC, id DESC").Find(&banners).Error; err != nil {
		return nil, err
	}
	return banners, nil
}

func (r *bannerRepository) IncrementClickCount(bannerID uint64) error {
	return r.db.Model(&v2.Banner{}).Where("id = ?", bannerID).
		UpdateColumn("click_count", gorm.Expr("click_count + 1")).Error
}

func (r *bannerRepository) CreateClickLog(log *v2.BannerClickLog) error {
	return r.db.Create(log).Error
}
