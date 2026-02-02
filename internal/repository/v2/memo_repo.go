package v2

import (
	"errors"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// MemoRepository v2 memo data access
type MemoRepository interface {
	FindByUserAndTarget(userID, targetUserID uint64) (*v2.V2Memo, error)
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

func (r *memoRepository) Delete(userID, targetUserID uint64) error {
	return r.db.Where("user_id = ? AND target_user_id = ?", userID, targetUserID).Delete(&v2.V2Memo{}).Error
}
