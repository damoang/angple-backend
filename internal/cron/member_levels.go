package cron

import (
	"fmt"
	"log"
	"time"

	"github.com/damoang/angple-backend/internal/activitycache"
	"github.com/damoang/angple-backend/internal/common"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/gorm"
)

// MemberLevelsResult contains the result of member levels update
type MemberLevelsResult struct {
	UpdatedCount    int      `json:"updated_count"`
	VisibilityCount int      `json:"visibility_count"`
	Messages        []string `json:"messages"`
	ExecutedAt      string   `json:"executed_at"`
}

type promotionRow struct {
	AdvertiserName string `gorm:"column:advertiser_name"`
	MemberID       string `gorm:"column:member_id"`
	StartDate      string `gorm:"column:start_date"`
	EndDate        string `gorm:"column:end_date"`
}

// runUpdateMemberLevels updates advertiser member levels based on active dates
func runUpdateMemberLevels(db *gorm.DB) (*MemberLevelsResult, error) {
	now := time.Now()
	today := now.Format("2006-01-02")

	// promotions 테이블에서 광고주 목록 조회 (같은 DB)
	var promotions []promotionRow
	if err := db.Table("promotions").
		Select("advertiser_name, member_id, start_date, end_date").
		Where("is_active = ?", true).
		Find(&promotions).Error; err != nil {
		return nil, fmt.Errorf("promotions 조회 실패: %w", err)
	}

	result := &MemberLevelsResult{
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}

	if len(promotions) == 0 {
		result.Messages = append(result.Messages, "처리할 광고주 데이터가 없습니다")
		return result, nil
	}

	for _, promo := range promotions {
		if promo.MemberID == "" {
			continue
		}

		// 현재 회원 레벨 조회
		var currentLevel int
		err := db.Table("g5_member").
			Select("mb_level").
			Where("mb_id = ?", promo.MemberID).
			Scan(&currentLevel).Error
		if err != nil {
			result.Messages = append(result.Messages, fmt.Sprintf("회원 ID '%s'를 찾을 수 없습니다", promo.MemberID))
			continue
		}

		// 기간에 따른 권한 결정
		newLevel := determineMemberLevel(promo.StartDate, promo.EndDate, today)

		// 권한 변경이 필요한 경우만 업데이트
		if currentLevel != newLevel {
			if err := db.Table("g5_member").
				Where("mb_id = ?", promo.MemberID).
				Update("mb_level", newLevel).Error; err != nil {
				result.Messages = append(result.Messages, fmt.Sprintf("%s(%s): 권한 업데이트 실패", promo.AdvertiserName, promo.MemberID))
				continue
			}
			result.UpdatedCount++
			status := getStatusText(promo.StartDate, promo.EndDate, today)
			result.Messages = append(result.Messages, fmt.Sprintf("%s(%s): 권한 %d → %d (%s)", promo.AdvertiserName, promo.MemberID, currentLevel, newLevel, status))
		}

		// 게시글 비공개/공개 처리
		isExpired := promo.EndDate != "" && today > promo.EndDate
		isActive := !isExpired &&
			(promo.StartDate == "" || today >= promo.StartDate) &&
			(promo.EndDate == "" || today <= promo.EndDate)

		if isExpired {
			affected := updatePostVisibility(db, promo.MemberID, true)
			if affected > 0 {
				result.VisibilityCount += common.SafeInt64ToInt(affected)
				result.Messages = append(result.Messages, fmt.Sprintf("%s(%s): %d개 게시글 비공개 처리", promo.AdvertiserName, promo.MemberID, affected))
			}
		} else if isActive {
			affected := updatePostVisibility(db, promo.MemberID, false)
			if affected > 0 {
				result.VisibilityCount += common.SafeInt64ToInt(affected)
				result.Messages = append(result.Messages, fmt.Sprintf("%s(%s): %d개 게시글 공개 전환", promo.AdvertiserName, promo.MemberID, affected))
			}
		}
	}

	return result, nil
}

// determineMemberLevel determines member level based on start/end dates
func determineMemberLevel(startDate, endDate, today string) int {
	// 기간이 설정되지 않은 경우 기본 권한 유지
	if startDate == "" && endDate == "" {
		return 2
	}

	// 시작일과 종료일이 모두 설정된 경우
	if startDate != "" && endDate != "" {
		if today >= startDate && today <= endDate {
			return 5
		}
		return 2
	}

	// 시작일만 설정된 경우
	if startDate != "" && endDate == "" {
		if today >= startDate {
			return 5
		}
		return 2
	}

	// 종료일만 설정된 경우
	if startDate == "" && endDate != "" {
		if today <= endDate {
			return 5
		}
		return 2
	}

	return 2
}

// getStatusText returns human-readable status text
func getStatusText(startDate, endDate, today string) string {
	if startDate == "" && endDate == "" {
		return "기간 미설정"
	}
	if startDate != "" && endDate != "" {
		if today < startDate {
			return "대기중"
		} else if today > endDate {
			return "만료됨"
		}
		return "활성중"
	}
	if startDate != "" {
		if today >= startDate {
			return "활성중"
		}
		return "대기중"
	}
	if endDate != "" {
		if today <= endDate {
			return "활성중"
		}
		return "만료됨"
	}
	return "기간 불완전"
}

// updatePostVisibility adds or removes 'secret' from wr_option in g5_write_promotion
func updatePostVisibility(db *gorm.DB, memberID string, makeSecret bool) int64 {
	// 바뀔 행을 UPDATE 와 같은 조건으로 먼저 뽑아 둔다 — 뒤에서 후처리 이벤트를 남긴다.
	visibilityCond := "mb_id = ? AND wr_option LIKE '%secret%'"
	if makeSecret {
		visibilityCond = "mb_id = ? AND wr_option NOT LIKE '%secret%'"
	}
	var changed []promotionVisibilityRow
	if err := db.Table("g5_write_promotion").
		Select("wr_id, wr_parent, wr_is_comment, COALESCE(wr_name, '') AS wr_name, COALESCE(wr_subject, '') AS wr_subject").
		Where(visibilityCond, memberID).
		Scan(&changed).Error; err != nil {
		changed = nil
	}

	var result *gorm.DB

	if makeSecret {
		// 비공개 처리: secret 추가
		result = db.Exec(`
			UPDATE g5_write_promotion
			SET wr_option = CASE
				WHEN wr_option = '' THEN 'secret'
				WHEN wr_option NOT LIKE '%secret%' THEN CONCAT(wr_option, ',secret')
				ELSE wr_option
			END
			WHERE mb_id = ? AND wr_option NOT LIKE '%secret%'
		`, memberID)
	} else {
		// 공개 처리: secret 제거
		result = db.Exec(`
			UPDATE g5_write_promotion
			SET wr_option = TRIM(BOTH ',' FROM
				REPLACE(REPLACE(REPLACE(wr_option, ',secret', ''), 'secret,', ''), 'secret', '')
			)
			WHERE mb_id = ? AND wr_option LIKE '%secret%'
		`, memberID)
	}

	if result.Error != nil {
		return 0
	}
	if result.RowsAffected > 0 {
		// 비공개 전환은 활동 패널·프로필 최근 글에서도 바로 빠져야 한다.
		// 작성자 캐시는 바로 지우고, 행마다 후처리 이벤트를 남겨 워커가 피드 공개 여부를
		// 다시 맞춘 뒤 작성자와 그 글에 댓글을 단 회원의 캐시까지 지우게 한다.
		activitycache.InvalidateMembers(memberID)
		enqueuePromotionVisibilityEvents(db, memberID, changed)
	}
	return result.RowsAffected
}

// promotionVisibilityRow 는 홍보 게시판 공개 여부가 바뀌는 행이다.
type promotionVisibilityRow struct {
	WrID        int    `gorm:"column:wr_id"`
	WrParent    int    `gorm:"column:wr_parent"`
	WrIsComment int    `gorm:"column:wr_is_comment"`
	WrName      string `gorm:"column:wr_name"`
	WrSubject   string `gorm:"column:wr_subject"`
}

// enqueuePromotionVisibilityEvents 는 공개 여부가 바뀐 홍보 글·댓글마다 후처리 이벤트를 남긴다.
//
// 이벤트를 쓰는 이유: 캐시만 지우면 피드의 is_public 이 그대로라 피드를 읽는 다른
// 화면이 어긋나고, 글의 댓글 작성자 목록도 워커 동기화가 이미 계산한다.
// 이벤트 기록이 실패한 행은 그 자리에서 캐시만이라도 지운다(최선 노력).
func enqueuePromotionVisibilityEvents(db *gorm.DB, memberID string, rows []promotionVisibilityRow) {
	repo := gnurepo.NewWriteAfterEventRepository(db)
	for _, row := range rows {
		event := &gnudomain.WriteAfterEvent{
			EventType: gnudomain.WriteAfterEventTypePostUpdated,
			BoardSlug: promotionBoardSlug,
			WriteID:   row.WrID,
			MemberID:  memberID,
			Author:    row.WrName,
			Subject:   row.WrSubject,
		}
		if row.WrIsComment != 0 {
			postID := row.WrParent
			event.EventType = gnudomain.WriteAfterEventTypeCommentUpdated
			event.PostID = &postID
			event.Subject = ""
		}
		if err := repo.Create(nil, event); err != nil {
			log.Printf("[Cron:update-member-levels] 후처리 이벤트 기록 실패 (%s/%d): %v", promotionBoardSlug, row.WrID, err)
			activitycache.InvalidateWrite(db, promotionBoardSlug, row.WrID)
		}
	}
}

// promotionBoardSlug 는 광고주 글 공개 여부를 관리하는 홍보 게시판이다.
const promotionBoardSlug = "promotion"
