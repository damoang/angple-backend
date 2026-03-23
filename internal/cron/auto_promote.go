package cron

import (
	"fmt"
	"log"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/memberlevel"
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
// Conditions: mb_certify <> ” AND mb_login_days >= 7 AND as_exp >= 3000
func runAutoPromote(db *gorm.DB, notiRepo gnurepo.NotiRepository) (*AutoPromoteResult, error) {
	now := time.Now()

	type candidate struct {
		MbID        string    `gorm:"column:mb_id"`
		MbLevel     int       `gorm:"column:mb_level"`
		MbLoginDays int       `gorm:"column:mb_login_days"`
		AsExp       int       `gorm:"column:as_exp"`
		AsLevel     int       `gorm:"column:as_level"`
		MbCertify   string    `gorm:"column:mb_certify"`
		MbDatetime  time.Time `gorm:"column:mb_datetime"`
	}

	var candidates []candidate
	if err := db.Table("g5_member").
		Select("mb_id, mb_level, mb_login_days, as_exp, COALESCE(as_level, 0) as as_level, mb_certify, mb_datetime").
		Where("mb_level = 2 AND mb_login_days >= 7 AND as_exp >= 3000").
		Where("COALESCE(mb_certify, '') <> ''").
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

	for _, candidate := range candidates {
		member := gnudomain.G5Member{
			MbID:        candidate.MbID,
			MbLevel:     candidate.MbLevel,
			MbLoginDays: candidate.MbLoginDays,
			AsExp:       candidate.AsExp,
			AsLevel:     candidate.AsLevel,
			MbCertify:   candidate.MbCertify,
			MbDatetime:  candidate.MbDatetime,
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			update := tx.Table("g5_member").
				Where("mb_id = ? AND mb_level = ?", candidate.MbID, candidate.MbLevel).
				Update("mb_level", 3)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return nil
			}

			if err := memberlevel.RecordPromotion(tx, &member, 3, memberlevel.ReasonAutoPromoteCron); err != nil {
				if memberlevel.IsMissingHistoryTableError(err) {
					log.Printf("[Cron:auto-promote] member level history table missing; promotion log skipped for %s: %v", candidate.MbID, err)
					return nil
				}
				return err
			}

			result.PromotedCount++
			result.PromotedIDs = append(result.PromotedIDs, candidate.MbID)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("등급 업데이트 실패(%s): %w", candidate.MbID, err)
		}
	}

	if notiRepo != nil {
		for _, mbID := range result.PromotedIDs {
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
