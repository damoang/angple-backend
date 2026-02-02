package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// BlockRepository block data access interface
type BlockRepository interface {
	Create(mbID string, blockedMbID string) (*domain.MemberBlock, error)
	Delete(mbID string, blockedMbID string) error
	FindByMember(mbID string) ([]*domain.MemberBlock, error)
	Exists(mbID string, blockedMbID string) (bool, error)
	GetBlockedUserIDs(mbID string) ([]string, error)
}

type blockRepository struct {
	db *gorm.DB
}

// NewBlockRepository creates a new BlockRepository
func NewBlockRepository(db *gorm.DB) BlockRepository {
	return &blockRepository{db: db}
}

// Create adds a block
func (r *blockRepository) Create(mbID string, blockedMbID string) (*domain.MemberBlock, error) {
	block := &domain.MemberBlock{
		MbID:        mbID,
		BlockedMbID: blockedMbID,
		CreatedAt:   time.Now(),
	}
	if err := r.db.Create(block).Error; err != nil {
		return nil, err
	}
	return block, nil
}

// Delete removes a block
func (r *blockRepository) Delete(mbID string, blockedMbID string) error {
	result := r.db.Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).
		Delete(&domain.MemberBlock{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("차단 기록을 찾을 수 없습니다")
	}
	return nil
}

// FindByMember returns all blocks for a member
func (r *blockRepository) FindByMember(mbID string) ([]*domain.MemberBlock, error) {
	var blocks []*domain.MemberBlock
	err := r.db.Where("mb_id = ?", mbID).Order("id DESC").Find(&blocks).Error
	return blocks, err
}

// Exists checks if a block exists
func (r *blockRepository) Exists(mbID string, blockedMbID string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ? AND blocked_mb_id = ?", mbID, blockedMbID).
		Count(&count).Error
	return count > 0, err
}

// GetBlockedUserIDs returns all blocked user IDs for a member
func (r *blockRepository) GetBlockedUserIDs(mbID string) ([]string, error) {
	var ids []string
	err := r.db.Model(&domain.MemberBlock{}).
		Where("mb_id = ?", mbID).
		Pluck("blocked_mb_id", &ids).Error
	return ids, err
}
