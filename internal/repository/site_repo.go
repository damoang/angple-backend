package repository

import (
	"context"
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

type SiteRepository struct {
	db *gorm.DB
}

func NewSiteRepository(db *gorm.DB) *SiteRepository {
	return &SiteRepository{db: db}
}

// ========================================
// Site CRUD
// ========================================

// FindBySubdomain retrieves a site by subdomain
func (r *SiteRepository) FindBySubdomain(ctx context.Context, subdomain string) (*domain.Site, error) {
	var site domain.Site
	err := r.db.WithContext(ctx).
		Where("subdomain = ?", subdomain).
		First(&site).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Not found is not an error
		}
		return nil, err
	}

	return &site, nil
}

// FindByID retrieves a site by ID
func (r *SiteRepository) FindByID(ctx context.Context, siteID string) (*domain.Site, error) {
	var site domain.Site
	err := r.db.WithContext(ctx).
		Where("id = ?", siteID).
		First(&site).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &site, nil
}

// Create creates a new site
func (r *SiteRepository) Create(ctx context.Context, site *domain.Site) error {
	return r.db.WithContext(ctx).Create(site).Error
}

// Update updates an existing site
func (r *SiteRepository) Update(ctx context.Context, site *domain.Site) error {
	return r.db.WithContext(ctx).Save(site).Error
}

// Delete soft-deletes a site (actually just set active=false)
func (r *SiteRepository) Delete(ctx context.Context, siteID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.Site{}).
		Where("id = ?", siteID).
		Update("active", false).Error
}

// ListActive retrieves all active sites
func (r *SiteRepository) ListActive(ctx context.Context, limit, offset int) ([]domain.Site, error) {
	var sites []domain.Site
	err := r.db.WithContext(ctx).
		Where("active = ? AND suspended = ?", true, false).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&sites).Error

	return sites, err
}

// ========================================
// SiteSettings CRUD
// ========================================

// FindSettingsBySiteID retrieves site settings
func (r *SiteRepository) FindSettingsBySiteID(ctx context.Context, siteID string) (*domain.SiteSettings, error) {
	var settings domain.SiteSettings
	err := r.db.WithContext(ctx).
		Where("site_id = ?", siteID).
		First(&settings).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &settings, nil
}

// CreateSettings creates initial site settings
func (r *SiteRepository) CreateSettings(ctx context.Context, settings *domain.SiteSettings) error {
	return r.db.WithContext(ctx).Create(settings).Error
}

// UpdateSettings updates site settings
func (r *SiteRepository) UpdateSettings(ctx context.Context, settings *domain.SiteSettings) error {
	return r.db.WithContext(ctx).Save(settings).Error
}

// ========================================
// SiteUser CRUD
// ========================================

// AddUserPermission adds a user permission to a site
func (r *SiteRepository) AddUserPermission(ctx context.Context, siteUser *domain.SiteUser) error {
	return r.db.WithContext(ctx).Create(siteUser).Error
}

// FindUserPermission retrieves user permission for a site
func (r *SiteRepository) FindUserPermission(ctx context.Context, siteID, userID string) (*domain.SiteUser, error) {
	var siteUser domain.SiteUser
	err := r.db.WithContext(ctx).
		Where("site_id = ? AND user_id = ?", siteID, userID).
		First(&siteUser).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &siteUser, nil
}

// ListSiteUsers retrieves all users with permissions for a site
func (r *SiteRepository) ListSiteUsers(ctx context.Context, siteID string) ([]domain.SiteUser, error) {
	var users []domain.SiteUser
	err := r.db.WithContext(ctx).
		Where("site_id = ?", siteID).
		Order("created_at ASC").
		Find(&users).Error

	return users, err
}

// RemoveUserPermission removes a user's permission from a site
func (r *SiteRepository) RemoveUserPermission(ctx context.Context, siteID, userID string) error {
	return r.db.WithContext(ctx).
		Where("site_id = ? AND user_id = ?", siteID, userID).
		Delete(&domain.SiteUser{}).Error
}

// ========================================
// Helper methods
// ========================================

// FindByCustomDomain retrieves a site by custom domain (from site_settings table)
func (r *SiteRepository) FindByCustomDomain(ctx context.Context, customDomain string) (interface{}, error) {
	// First find site_id from settings table
	var settings domain.SiteSettings
	err := r.db.WithContext(ctx).
		Where("custom_domain = ?", customDomain).
		First(&settings).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("site not found")
		}
		return nil, err
	}

	// Then get the site
	var site domain.Site
	err = r.db.WithContext(ctx).
		Where("id = ?", settings.SiteID).
		First(&site).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("site not found")
		}
		return nil, err
	}

	// Return as map for compatibility with tenant middleware
	return map[string]interface{}{
		"id":          site.ID,
		"subdomain":   site.Subdomain,
		"db_strategy": site.DBStrategy,
		"plan":        site.Plan,
	}, nil
}

// CheckSubdomainAvailability checks if subdomain is available
func (r *SiteRepository) CheckSubdomainAvailability(ctx context.Context, subdomain string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.Site{}).
		Where("subdomain = ?", subdomain).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count == 0, nil
}

// CountSitesByOwner counts how many sites an owner has
func (r *SiteRepository) CountSitesByOwner(ctx context.Context, ownerEmail string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.Site{}).
		Where("owner_email = ? AND active = ?", ownerEmail, true).
		Count(&count).Error

	return count, err
}
