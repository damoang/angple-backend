package gnuboard

import (
	"errors"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

type AnniversaryDrawRepository interface {
	GetByMember(eventCode, mbID string) (*gnudomain.AnniversaryDrawEntry, error)
	Create(entry *gnudomain.AnniversaryDrawEntry) error
	ListPending(eventCode string, limit int) ([]gnudomain.AnniversaryDrawEntry, error)
	MarkGranted(id uint64, pointPoID int, grantedAt time.Time) error
}

type anniversaryDrawRepository struct {
	db *gorm.DB
}

func NewAnniversaryDrawRepository(db *gorm.DB) AnniversaryDrawRepository {
	return &anniversaryDrawRepository{db: db}
}

func (r *anniversaryDrawRepository) GetByMember(eventCode, mbID string) (*gnudomain.AnniversaryDrawEntry, error) {
	var entry gnudomain.AnniversaryDrawEntry
	err := r.db.Where("event_code = ? AND mb_id = ?", eventCode, mbID).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *anniversaryDrawRepository) Create(entry *gnudomain.AnniversaryDrawEntry) error {
	return r.db.Create(entry).Error
}

func (r *anniversaryDrawRepository) ListPending(eventCode string, limit int) ([]gnudomain.AnniversaryDrawEntry, error) {
	if limit <= 0 {
		limit = 200
	}

	var entries []gnudomain.AnniversaryDrawEntry
	err := r.db.
		Where("event_code = ? AND granted_at IS NULL", eventCode).
		Order("id ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

func (r *anniversaryDrawRepository) MarkGranted(id uint64, pointPoID int, grantedAt time.Time) error {
	return r.db.Model(&gnudomain.AnniversaryDrawEntry{}).
		Where("id = ? AND granted_at IS NULL", id).
		Updates(map[string]interface{}{
			"granted_at":  grantedAt,
			"point_po_id": pointPoID,
		}).Error
}
