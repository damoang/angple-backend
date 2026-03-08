package cron

import (
	"time"

	"gorm.io/gorm"
)

// MemberLockResult contains the result of member lock release
type MemberLockResult struct {
	ReleasedCount int      `json:"released_count"`
	ReleasedIDs   []string `json:"released_ids"`
	ExecutedAt    string   `json:"executed_at"`
}

// runMemberLockRelease releases locked members (mb_4 = 'lock' → '')
func runMemberLockRelease(db *gorm.DB) (*MemberLockResult, error) {
	now := time.Now()

	// 1. lock이 걸린 회원 ID 조회
	var lockedIDs []string
	if err := db.Table("g5_member").
		Where("mb_4 = ?", "lock").
		Pluck("mb_id", &lockedIDs).Error; err != nil {
		return nil, err
	}

	// 2. lock 해제
	if len(lockedIDs) > 0 {
		if err := db.Table("g5_member").
			Where("mb_4 = ?", "lock").
			Update("mb_4", "").Error; err != nil {
			return nil, err
		}
	}

	return &MemberLockResult{
		ReleasedCount: len(lockedIDs),
		ReleasedIDs:   lockedIDs,
		ExecutedAt:    now.Format("2006-01-02 15:04:05"),
	}, nil
}
