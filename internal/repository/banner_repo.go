package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// BannerRepository defines the interface for banner data access
type BannerRepository interface {
	// Banner methods
	GetAllBanners() ([]*domain.Banner, error)
	GetActiveBanners() ([]*domain.Banner, error)
	GetBannersByPosition(position domain.BannerPosition) ([]*domain.Banner, error)
	FindBannerByID(id string) (*domain.Banner, error)
	CreateBanner(banner *domain.Banner) error
	UpdateBanner(banner *domain.Banner) error
	DeleteBanner(id string) error
	IncrementClickCount(id string) error
	IncrementViewCount(id string) error

	// Click log methods
	CreateClickLog(log *domain.BannerClickLog) error
	GetClickLogsByBanner(bannerID string, limit int) ([]*domain.BannerClickLog, error)
}

// bannerRepository implements BannerRepository with GORM
type bannerRepository struct {
	db *gorm.DB
}

// NewBannerRepository creates a new BannerRepository
func NewBannerRepository(db *gorm.DB) BannerRepository {
	return &bannerRepository{db: db}
}

// GetAllBanners retrieves all banners
func (r *bannerRepository) GetAllBanners() ([]*domain.Banner, error) {
	var banners []*domain.Banner

	err := r.db.
		Order("priority DESC, created_at DESC").
		Find(&banners).Error

	if err != nil {
		return nil, err
	}

	return banners, nil
}

// GetActiveBanners retrieves active banners within valid date range
func (r *bannerRepository) GetActiveBanners() ([]*domain.Banner, error) {
	var banners []*domain.Banner

	err := r.db.
		Where("status = ?", "active").
		Where("(start_date IS NULL OR start_date <= CURDATE())").
		Where("(end_date IS NULL OR end_date >= CURDATE())").
		Order("created_at DESC").
		Find(&banners).Error

	if err != nil {
		return nil, err
	}

	return banners, nil
}

// GetBannersByPosition retrieves active banners by position
func (r *bannerRepository) GetBannersByPosition(position domain.BannerPosition) ([]*domain.Banner, error) {
	var banners []*domain.Banner

	err := r.db.
		Where("status = ?", "active").
		Where("position = ?", position).
		Where("(start_date IS NULL OR start_date <= CURDATE())").
		Where("(end_date IS NULL OR end_date >= CURDATE())").
		Order("created_at DESC").
		Find(&banners).Error

	if err != nil {
		return nil, err
	}

	return banners, nil
}

// FindBannerByID finds a banner by ID
func (r *bannerRepository) FindBannerByID(id string) (*domain.Banner, error) {
	var banner domain.Banner

	err := r.db.
		Where("id = ?", id).
		First(&banner).Error

	if err != nil {
		return nil, err
	}

	return &banner, nil
}

// CreateBanner creates a new banner
func (r *bannerRepository) CreateBanner(banner *domain.Banner) error {
	return r.db.Create(banner).Error
}

// UpdateBanner updates a banner
func (r *bannerRepository) UpdateBanner(banner *domain.Banner) error {
	return r.db.Save(banner).Error
}

// DeleteBanner deletes a banner by ID
func (r *bannerRepository) DeleteBanner(id string) error {
	return r.db.Delete(&domain.Banner{}, id).Error
}

// IncrementClickCount increments the click count of a banner
func (r *bannerRepository) IncrementClickCount(id string) error {
	return r.db.Model(&domain.Banner{}).
		Where("id = ?", id).
		UpdateColumn("click_count", gorm.Expr("click_count + 1")).Error
}

// IncrementViewCount increments the view count of a banner
func (r *bannerRepository) IncrementViewCount(id string) error {
	return r.db.Model(&domain.Banner{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

// CreateClickLog creates a new click log entry
func (r *bannerRepository) CreateClickLog(log *domain.BannerClickLog) error {
	return r.db.Create(log).Error
}

// GetClickLogsByBanner retrieves click logs for a specific banner
func (r *bannerRepository) GetClickLogsByBanner(bannerID string, limit int) ([]*domain.BannerClickLog, error) {
	var logs []*domain.BannerClickLog

	query := r.db.
		Where("banner_id = ?", bannerID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	if err != nil {
		return nil, err
	}

	return logs, nil
}
