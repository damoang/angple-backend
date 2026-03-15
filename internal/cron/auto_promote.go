package cron

import (
	"fmt"
	"log"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// AutoPromoteResult contains the result of auto-promotion cron
type AutoPromoteResult struct {
	PromotedCount int      `json:"promoted_count"`
	PromotedIDs   []string `json:"promoted_ids"`
	ExecutedAt    string   `json:"executed_at"`
}

// runAutoPromote promotes members from mb_level 2 to 3
// Conditions: mb_login_days >= 7 AND as_exp >= 3000
func runAutoPromote(db *gorm.DB, notiRepo gnurepo.NotiRepository) (*AutoPromoteResult, error) {
	now := time.Now()

	// 조건 충족 회원 조회: mb_level=2, 로그인 7일 이상, 경험치 3000 이상
	type candidate struct {
		MbID string `gorm:"column:mb_id"`
	}
	var candidates []candidate
	if err := db.Table("g5_member").
		Select("mb_id").
		Where("mb_level = 2 AND mb_login_days >= 7 AND as_exp >= 3000").
		Where("mb_leave_date = '' AND mb_intercept_date = ''").
		Find(&candidates).Error; err != nil {
		return nil, fmt.Errorf("후보 조회 실패: %w", err)
	}

	result := &AutoPromoteResult{
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}

	if len(candidates) == 0 {
		return result, nil
	}

	// 일괄 업데이트
	mbIDs := make([]string, len(candidates))
	for i, c := range candidates {
		mbIDs[i] = c.MbID
	}

	if err := db.Table("g5_member").
		Where("mb_id IN ?", mbIDs).
		Update("mb_level", 3).Error; err != nil {
		return nil, fmt.Errorf("등급 업데이트 실패: %w", err)
	}

	result.PromotedCount = len(mbIDs)
	result.PromotedIDs = mbIDs

	// 알림 발송 (best-effort)
	if notiRepo != nil {
		for _, mbID := range mbIDs {
			noti := &gnurepo.Notification{
				MbID:          mbID,
				PhFromCase:    "promote",
				PhToCase:      "me",
				BoTable:       "@system",
				WrID:          0,
				RelMbID:       "system",
				RelMbNick:     "다모앙",
				RelMsg:        "💛 앙님(💛)으로 되었습니다. 앞으로도 다모앙에서 즐거운 시간 보내세요!",
				RelURL:        "/my",
				PhReaded:      "N",
				ParentSubject: "축하합니다.",
			}
			if err := notiRepo.Create(noti); err != nil {
				log.Printf("[Cron:auto-promote] notification failed for %s: %v", mbID, err)
			}
		}
	}

	return result, nil
}
