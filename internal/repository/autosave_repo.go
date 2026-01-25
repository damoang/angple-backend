package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AutosaveRepository handles autosave data operations
type AutosaveRepository interface {
	// Save creates or updates an autosave entry
	Save(autosave *domain.Autosave) error
	// FindByMemberID returns all autosaves for a member
	FindByMemberID(memberID string) ([]domain.Autosave, error)
	// FindByID returns a specific autosave
	FindByID(id int, memberID string) (*domain.Autosave, error)
	// Delete removes an autosave entry
	Delete(id int, memberID string) error
	// Count returns the number of autosaves for a member
	Count(memberID string) (int64, error)
	// ExistsSameContent checks if same content already exists
	ExistsSameContent(memberID, subject, content string) (bool, error)
}

type autosaveRepository struct {
	db *gorm.DB
}

// NewAutosaveRepository creates a new AutosaveRepository
func NewAutosaveRepository(db *gorm.DB) AutosaveRepository {
	return &autosaveRepository{db: db}
}

// Save creates or updates an autosave entry using ON DUPLICATE KEY UPDATE
func (r *autosaveRepository) Save(autosave *domain.Autosave) error {
	autosave.CreatedAt = time.Now()

	// Use GORM's Clauses for upsert
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mb_id"}, {Name: "as_uid"}},
		DoUpdates: clause.AssignmentColumns([]string{"as_subject", "as_content", "as_datetime"}),
	}).Create(autosave).Error
}

// FindByMemberID returns all autosaves for a member ordered by ID desc
func (r *autosaveRepository) FindByMemberID(memberID string) ([]domain.Autosave, error) {
	var autosaves []domain.Autosave
	err := r.db.Where("mb_id = ?", memberID).
		Order("as_id DESC").
		Find(&autosaves).Error
	return autosaves, err
}

// FindByID returns a specific autosave for a member
func (r *autosaveRepository) FindByID(id int, memberID string) (*domain.Autosave, error) {
	var autosave domain.Autosave
	err := r.db.Where("as_id = ? AND mb_id = ?", id, memberID).
		First(&autosave).Error
	if err != nil {
		return nil, err
	}
	return &autosave, nil
}

// Delete removes an autosave entry
func (r *autosaveRepository) Delete(id int, memberID string) error {
	result := r.db.Where("as_id = ? AND mb_id = ?", id, memberID).
		Delete(&domain.Autosave{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// Count returns the number of autosaves for a member
func (r *autosaveRepository) Count(memberID string) (int64, error) {
	var count int64
	err := r.db.Model(&domain.Autosave{}).
		Where("mb_id = ?", memberID).
		Count(&count).Error
	return count, err
}

// ExistsSameContent checks if same content already exists
func (r *autosaveRepository) ExistsSameContent(memberID, subject, content string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Autosave{}).
		Where("mb_id = ? AND as_subject = ? AND as_content = ?", memberID, subject, content).
		Count(&count).Error
	return count > 0, err
}
