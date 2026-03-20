package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// SiteLogoRepository handles site logo data access
type SiteLogoRepository interface {
	FindAll() ([]*v2.SiteLogo, error)
	FindByID(id uint64) (*v2.SiteLogo, error)
	Create(logo *v2.SiteLogo) error
	Update(logo *v2.SiteLogo) error
	Delete(id uint64) error
	FindActiveLogo(mmdd, today string) (*v2.SiteLogo, error)
	FindAllActive() ([]*v2.SiteLogo, error)
	CountActiveDefault() (int64, error)
}

type siteLogoRepository struct {
	db *gorm.DB
}

// NewSiteLogoRepository creates a new SiteLogoRepository
func NewSiteLogoRepository(db *gorm.DB) SiteLogoRepository {
	return &siteLogoRepository{db: db}
}

func (r *siteLogoRepository) FindAll() ([]*v2.SiteLogo, error) {
	var logos []*v2.SiteLogo
	if err := r.db.Order("priority DESC, id DESC").Find(&logos).Error; err != nil {
		return nil, err
	}
	return logos, nil
}

func (r *siteLogoRepository) FindByID(id uint64) (*v2.SiteLogo, error) {
	var logo v2.SiteLogo
	if err := r.db.First(&logo, id).Error; err != nil {
		return nil, err
	}
	return &logo, nil
}

func (r *siteLogoRepository) Create(logo *v2.SiteLogo) error {
	return r.db.Create(logo).Error
}

func (r *siteLogoRepository) Update(logo *v2.SiteLogo) error {
	return r.db.Save(logo).Error
}

func (r *siteLogoRepository) Delete(id uint64) error {
	return r.db.Delete(&v2.SiteLogo{}, id).Error
}

// FindActiveLogo returns the highest-priority active logo matching the given date.
// Priority: recurring > date_range > default
func (r *siteLogoRepository) FindActiveLogo(mmdd, today string) (*v2.SiteLogo, error) {
	var logo v2.SiteLogo
	err := r.db.
		Where("is_active = ?", true).
		Where(`(
			(schedule_type = 'recurring' AND recurring_date = ?)
			OR (schedule_type = 'date_range' AND start_date <= ? AND end_date >= ?)
			OR schedule_type = 'default'
		)`, mmdd, today, today).
		Order(`CASE schedule_type WHEN 'recurring' THEN 1 WHEN 'date_range' THEN 2 WHEN 'default' THEN 3 END, priority DESC`).
		First(&logo).Error
	if err != nil {
		return nil, err
	}
	return &logo, nil
}

// FindAllActive returns all active logos for client-side re-matching
func (r *siteLogoRepository) FindAllActive() ([]*v2.SiteLogo, error) {
	var logos []*v2.SiteLogo
	if err := r.db.Where("is_active = ?", true).
		Order(`CASE schedule_type WHEN 'recurring' THEN 1 WHEN 'date_range' THEN 2 WHEN 'default' THEN 3 END, priority DESC`).
		Find(&logos).Error; err != nil {
		return nil, err
	}
	return logos, nil
}

// CountActiveDefault returns the count of active default logos
func (r *siteLogoRepository) CountActiveDefault() (int64, error) {
	var count int64
	if err := r.db.Model(&v2.SiteLogo{}).
		Where("is_active = ? AND schedule_type = ?", true, "default").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
