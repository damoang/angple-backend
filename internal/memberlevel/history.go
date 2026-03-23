package memberlevel

import (
	"strings"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

const (
	ReasonAutoPromoteLoginAPI = "auto_promote_login_api"
	ReasonAutoPromoteCron     = "auto_promote_cron"
)

func IsMissingHistoryTableError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "g5_member_level_history") &&
		(strings.Contains(msg, "no such table") || strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "error 1146"))
}

func RecordPromotion(tx *gorm.DB, member *gnudomain.G5Member, newLevel int, reason string) error {
	if member == nil || member.MbLevel == newLevel {
		return nil
	}

	entry := gnudomain.MemberLevelHistory{
		MbID:              member.MbID,
		OldMbLevel:        member.MbLevel,
		NewMbLevel:        newLevel,
		Reason:            reason,
		SnapshotAsLevel:   member.AsLevel,
		SnapshotAsExp:     member.AsExp,
		SnapshotLoginDays: member.MbLoginDays,
		SnapshotMbCertify: member.MbCertify,
		CreatedAt:         tx.NowFunc(),
	}
	if !member.MbDatetime.IsZero() {
		entry.MemberCreatedAt = &member.MbDatetime
	}

	return tx.Create(&entry).Error
}
