package repository

import (
	"context"
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// SiteContentRepository persists Angple Sites builder content (issue #1288 PoC).
type SiteContentRepository struct {
	db *gorm.DB
}

// NewSiteContentRepository constructs a SiteContentRepository.
func NewSiteContentRepository(db *gorm.DB) *SiteContentRepository {
	return &SiteContentRepository{db: db}
}

// FindBySiteAndKey returns the content row for (site_id, content_key) or nil if not found.
func (r *SiteContentRepository) FindBySiteAndKey(
	ctx context.Context, siteID int64, contentKey string,
) (*domain.AngpleSiteContent, error) {
	var c domain.AngpleSiteContent
	err := r.db.WithContext(ctx).
		Where("site_id = ? AND content_key = ?", siteID, contentKey).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Upsert creates or updates the content row for (site_id, content_key).
func (r *SiteContentRepository) Upsert(ctx context.Context, c *domain.AngpleSiteContent) error {
	existing, err := r.FindBySiteAndKey(ctx, c.SiteID, c.ContentKey)
	if err != nil {
		return err
	}
	if existing == nil {
		return r.db.WithContext(ctx).Create(c).Error
	}
	c.ID = existing.ID
	c.CreatedAt = existing.CreatedAt
	return r.db.WithContext(ctx).Save(c).Error
}

// Delete removes the content row for (site_id, content_key).
func (r *SiteContentRepository) Delete(
	ctx context.Context, siteID int64, contentKey string,
) error {
	return r.db.WithContext(ctx).
		Where("site_id = ? AND content_key = ?", siteID, contentKey).
		Delete(&domain.AngpleSiteContent{}).Error
}

// ListBySite returns all content rows for a site, optionally limited.
func (r *SiteContentRepository) ListBySite(
	ctx context.Context, siteID int64, limit, offset int,
) ([]domain.AngpleSiteContent, error) {
	var rows []domain.AngpleSiteContent
	q := r.db.WithContext(ctx).Where("site_id = ?", siteID).Order("content_key ASC")
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Find(&rows).Error
	return rows, err
}
