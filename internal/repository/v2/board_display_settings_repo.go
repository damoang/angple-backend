package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// BoardDisplaySettingsRepository handles display settings data access
type BoardDisplaySettingsRepository interface {
	FindByBoardSlug(slug string) (*v2.V2BoardDisplaySettings, error)
	Upsert(settings *v2.V2BoardDisplaySettings) error
}

type boardDisplaySettingsRepository struct {
	db *gorm.DB
}

// NewBoardDisplaySettingsRepository creates a new BoardDisplaySettingsRepository
func NewBoardDisplaySettingsRepository(db *gorm.DB) BoardDisplaySettingsRepository {
	return &boardDisplaySettingsRepository{db: db}
}

// FindByBoardSlug returns display settings by board slug (board_id is the slug)
func (r *boardDisplaySettingsRepository) FindByBoardSlug(slug string) (*v2.V2BoardDisplaySettings, error) {
	var settings v2.V2BoardDisplaySettings
	err := r.db.Where("board_id = ?", slug).First(&settings).Error
	if err == gorm.ErrRecordNotFound {
		// Return default settings if not found
		return &v2.V2BoardDisplaySettings{
			BoardID:       slug,
			ListLayout:    "compact",
			ViewLayout:    "basic",
			ShowPreview:   false,
			PreviewLength: 150,
			ShowThumbnail: false,
		}, nil
	}
	return &settings, err
}

// Upsert creates or updates display settings for a board (uses board_id as primary key)
func (r *boardDisplaySettingsRepository) Upsert(settings *v2.V2BoardDisplaySettings) error {
	var existing v2.V2BoardDisplaySettings
	err := r.db.Where("board_id = ?", settings.BoardID).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Create new
		return r.db.Create(settings).Error
	} else if err != nil {
		return err
	}

	// Update existing - board_id is primary key so we can use Save directly
	return r.db.Save(settings).Error
}
