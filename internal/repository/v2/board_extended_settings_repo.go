package v2

import (
	"errors"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// BoardExtendedSettingsRepository handles extended settings data access
type BoardExtendedSettingsRepository interface {
	FindByBoardSlug(slug string) (*v2.V2BoardExtendedSettings, error)
	Upsert(settings *v2.V2BoardExtendedSettings) error
}

type boardExtendedSettingsRepository struct {
	db *gorm.DB
}

// NewBoardExtendedSettingsRepository creates a new BoardExtendedSettingsRepository
func NewBoardExtendedSettingsRepository(db *gorm.DB) BoardExtendedSettingsRepository {
	return &boardExtendedSettingsRepository{db: db}
}

// FindByBoardSlug returns extended settings by board slug
func (r *boardExtendedSettingsRepository) FindByBoardSlug(slug string) (*v2.V2BoardExtendedSettings, error) {
	var settings v2.V2BoardExtendedSettings
	err := r.db.Where("board_id = ?", slug).First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Return empty settings if not found
		return &v2.V2BoardExtendedSettings{
			BoardID:  slug,
			Settings: "{}",
		}, nil
	}
	return &settings, err
}

// Upsert creates or updates extended settings for a board
func (r *boardExtendedSettingsRepository) Upsert(settings *v2.V2BoardExtendedSettings) error {
	var existing v2.V2BoardExtendedSettings
	err := r.db.Where("board_id = ?", settings.BoardID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(settings).Error
	} else if err != nil {
		return err
	}

	return r.db.Save(settings).Error
}
