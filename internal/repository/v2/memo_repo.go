package v2

import (
	"errors"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// MemoWithWriter represents a memo with writer member info
type MemoWithWriter struct {
	v2.V2Memo
	MbID   string `gorm:"column:mb_id" json:"mb_id"`
	MbNick string `gorm:"column:mb_nick" json:"mb_nick"`
}

// MemoRepository v2 memo data access
type MemoRepository interface {
	FindByUserAndTarget(userID, targetUserID uint64) (*v2.V2Memo, error)
	FindAllByTarget(targetUserID uint64) ([]MemoWithWriter, error)
	Upsert(memo *v2.V2Memo) error
	Delete(userID, targetUserID uint64) error
}

type memoRepository struct {
	db *gorm.DB
}

// NewMemoRepository creates a new v2 MemoRepository
func NewMemoRepository(db *gorm.DB) MemoRepository {
	return &memoRepository{db: db}
}

func (r *memoRepository) FindByUserAndTarget(userID, targetUserID uint64) (*v2.V2Memo, error) {
	var memo v2.V2Memo
	err := r.db.Where("user_id = ? AND target_user_id = ?", userID, targetUserID).First(&memo).Error
	return &memo, err
}

func (r *memoRepository) Upsert(memo *v2.V2Memo) error {
	existing := &v2.V2Memo{}
	result := r.db.Where("user_id = ? AND target_user_id = ?", memo.UserID, memo.TargetUserID).First(existing)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return r.db.Create(memo).Error
	}
	if result.Error != nil {
		return result.Error
	}
	existing.Content = memo.Content
	existing.Color = memo.Color
	return r.db.Save(existing).Error
}

// FindAllByTarget returns all memos about a target member with writer info (admin only)
func (r *memoRepository) FindAllByTarget(targetUserID uint64) ([]MemoWithWriter, error) {
	var memos []MemoWithWriter
	err := r.db.Table("v2_memos").
		Select("v2_memos.*, g5_member.mb_id, g5_member.mb_nick").
		Joins("LEFT JOIN g5_member ON g5_member.mb_no = v2_memos.user_id").
		Where("v2_memos.target_user_id = ?", targetUserID).
		Order("v2_memos.created_at DESC").
		Find(&memos).Error
	return memos, err
}

func (r *memoRepository) Delete(userID, targetUserID uint64) error {
	return r.db.Where("user_id = ? AND target_user_id = ?", userID, targetUserID).Delete(&v2.V2Memo{}).Error
}
