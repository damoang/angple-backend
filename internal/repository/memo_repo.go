package repository

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemoRepository handles member memo data operations
type MemoRepository struct {
	db *gorm.DB
}

// NewMemoRepository creates a new MemoRepository
func NewMemoRepository(db *gorm.DB) *MemoRepository {
	return &MemoRepository{db: db}
}

// GetMemo retrieves a memo by member ID and target member ID
func (r *MemoRepository) GetMemo(memberID, targetMemberID string) (*domain.MemberMemo, error) {
	var memo domain.MemberMemo
	err := r.db.Where("member_id = ? AND target_member_id = ?", memberID, targetMemberID).First(&memo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &memo, nil
}

// GetMemoList retrieves all memos by member ID
func (r *MemoRepository) GetMemoList(memberID string, offset, limit int) ([]domain.MemberMemo, int64, error) {
	var memos []domain.MemberMemo
	var total int64

	// Count total
	if err := r.db.Model(&domain.MemberMemo{}).Where("member_id = ?", memberID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get list
	if err := r.db.Where("member_id = ?", memberID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&memos).Error; err != nil {
		return nil, 0, err
	}

	return memos, total, nil
}

// UpsertMemo creates or updates a memo
func (r *MemoRepository) UpsertMemo(memo *domain.MemberMemo) error {
	now := time.Now()
	memo.UpdatedAt = &now

	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "member_id"}, {Name: "target_member_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"memo", "memo_detail", "color", "updated_at"}),
	}).Create(memo).Error
}

// DeleteMemo deletes a memo by member ID and target member ID
func (r *MemoRepository) DeleteMemo(memberID, targetMemberID string) error {
	return r.db.Where("member_id = ? AND target_member_id = ?", memberID, targetMemberID).
		Delete(&domain.MemberMemo{}).Error
}
