package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/plugins/advertising/domain"
	"gorm.io/gorm"
)

// AdRepository 광고 저장소 인터페이스
type AdRepository interface {
	// AdUnit CRUD
	CreateAdUnit(unit *domain.AdUnit) error
	UpdateAdUnit(id uint64, unit *domain.AdUnit) error
	DeleteAdUnit(id uint64) error
	FindAdUnitByID(id uint64) (*domain.AdUnit, error)
	FindAdUnitByPosition(position string) (*domain.AdUnit, error)
	ListAdUnits(activeOnly bool) ([]*domain.AdUnit, error)
	ListAdUnitsByType(adType domain.AdType, activeOnly bool) ([]*domain.AdUnit, error)

	// AdRotationConfig CRUD
	CreateRotationConfig(config *domain.AdRotationConfig) error
	UpdateRotationConfig(id uint64, config *domain.AdRotationConfig) error
	DeleteRotationConfig(id uint64) error
	FindRotationConfigByPosition(position string) (*domain.AdRotationConfig, error)
	ListRotationConfigs() ([]*domain.AdRotationConfig, error)

	// CelebrationBanner CRUD
	CreateBanner(banner *domain.CelebrationBanner) error
	UpdateBanner(id uint64, banner *domain.CelebrationBanner) error
	DeleteBanner(id uint64) error
	FindBannerByID(id uint64) (*domain.CelebrationBanner, error)
	ListBanners(activeOnly bool) ([]*domain.CelebrationBanner, error)
	ListBannersByDate(date time.Time) ([]*domain.CelebrationBanner, error)
}

// adRepository GORM 구현체
type adRepository struct {
	db *gorm.DB
}

// NewAdRepository 생성자
func NewAdRepository(db *gorm.DB) AdRepository {
	return &adRepository{db: db}
}

// ============ AdUnit Methods ============

// CreateAdUnit 광고 단위 생성
func (r *adRepository) CreateAdUnit(unit *domain.AdUnit) error {
	now := time.Now()
	unit.CreatedAt = now
	unit.UpdatedAt = now
	return r.db.Create(unit).Error
}

// UpdateAdUnit 광고 단위 수정
func (r *adRepository) UpdateAdUnit(id uint64, unit *domain.AdUnit) error {
	unit.UpdatedAt = time.Now()
	return r.db.Model(&domain.AdUnit{}).Where("id = ?", id).Updates(unit).Error
}

// DeleteAdUnit 광고 단위 삭제
func (r *adRepository) DeleteAdUnit(id uint64) error {
	return r.db.Delete(&domain.AdUnit{}, id).Error
}

// FindAdUnitByID ID로 광고 단위 조회
func (r *adRepository) FindAdUnitByID(id uint64) (*domain.AdUnit, error) {
	var unit domain.AdUnit
	err := r.db.First(&unit, id).Error
	if err != nil {
		return nil, err
	}
	return &unit, nil
}

// FindAdUnitByPosition 위치로 광고 단위 조회
func (r *adRepository) FindAdUnitByPosition(position string) (*domain.AdUnit, error) {
	var unit domain.AdUnit
	err := r.db.Where("position = ? AND is_active = ?", position, true).
		Order("priority DESC").
		First(&unit).Error
	if err != nil {
		return nil, err
	}
	return &unit, nil
}

// ListAdUnits 모든 광고 단위 조회
func (r *adRepository) ListAdUnits(activeOnly bool) ([]*domain.AdUnit, error) {
	var units []*domain.AdUnit
	query := r.db.Order("position ASC, priority DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&units).Error
	return units, err
}

// ListAdUnitsByType 광고 유형별 단위 조회
func (r *adRepository) ListAdUnitsByType(adType domain.AdType, activeOnly bool) ([]*domain.AdUnit, error) {
	var units []*domain.AdUnit
	query := r.db.Where("ad_type = ?", adType).Order("position ASC, priority DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&units).Error
	return units, err
}

// ============ AdRotationConfig Methods ============

// CreateRotationConfig 로테이션 설정 생성
func (r *adRepository) CreateRotationConfig(config *domain.AdRotationConfig) error {
	config.CreatedAt = time.Now()
	return r.db.Create(config).Error
}

// UpdateRotationConfig 로테이션 설정 수정
func (r *adRepository) UpdateRotationConfig(id uint64, config *domain.AdRotationConfig) error {
	return r.db.Model(&domain.AdRotationConfig{}).Where("id = ?", id).Updates(config).Error
}

// DeleteRotationConfig 로테이션 설정 삭제
func (r *adRepository) DeleteRotationConfig(id uint64) error {
	return r.db.Delete(&domain.AdRotationConfig{}, id).Error
}

// FindRotationConfigByPosition 위치별 로테이션 설정 조회
func (r *adRepository) FindRotationConfigByPosition(position string) (*domain.AdRotationConfig, error) {
	var config domain.AdRotationConfig
	err := r.db.Where("position = ?", position).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// ListRotationConfigs 모든 로테이션 설정 조회
func (r *adRepository) ListRotationConfigs() ([]*domain.AdRotationConfig, error) {
	var configs []*domain.AdRotationConfig
	err := r.db.Order("position ASC").Find(&configs).Error
	return configs, err
}

// ============ CelebrationBanner Methods ============

// CreateBanner 배너 생성
func (r *adRepository) CreateBanner(banner *domain.CelebrationBanner) error {
	banner.CreatedAt = time.Now()
	return r.db.Create(banner).Error
}

// UpdateBanner 배너 수정
func (r *adRepository) UpdateBanner(id uint64, banner *domain.CelebrationBanner) error {
	return r.db.Model(&domain.CelebrationBanner{}).Where("id = ?", id).Updates(banner).Error
}

// DeleteBanner 배너 삭제
func (r *adRepository) DeleteBanner(id uint64) error {
	return r.db.Delete(&domain.CelebrationBanner{}, id).Error
}

// FindBannerByID ID로 배너 조회
func (r *adRepository) FindBannerByID(id uint64) (*domain.CelebrationBanner, error) {
	var banner domain.CelebrationBanner
	err := r.db.First(&banner, id).Error
	if err != nil {
		return nil, err
	}
	return &banner, nil
}

// ListBanners 모든 배너 조회
func (r *adRepository) ListBanners(activeOnly bool) ([]*domain.CelebrationBanner, error) {
	var banners []*domain.CelebrationBanner
	query := r.db.Order("display_date DESC, id DESC")
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Find(&banners).Error
	return banners, err
}

// ListBannersByDate 특정 날짜 배너 조회
func (r *adRepository) ListBannersByDate(date time.Time) ([]*domain.CelebrationBanner, error) {
	var banners []*domain.CelebrationBanner
	// 날짜만 비교 (시간 제외)
	dateStr := date.Format("2006-01-02")
	err := r.db.Where("DATE(display_date) = ? AND is_active = ?", dateStr, true).
		Order("id DESC").
		Find(&banners).Error
	return banners, err
}
