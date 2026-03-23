package gnuboard

import "time"

// MemberLevelHistory records mb_level changes for audit and analysis.
type MemberLevelHistory struct {
	ID                uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	MbID              string     `gorm:"column:mb_id;size:255;not null;index:idx_member_created,priority:1"`
	OldMbLevel        int        `gorm:"column:old_mb_level;not null"`
	NewMbLevel        int        `gorm:"column:new_mb_level;not null"`
	Reason            string     `gorm:"column:reason;size:50;not null"`
	SnapshotAsLevel   int        `gorm:"column:snapshot_as_level;not null;default:0"`
	SnapshotAsExp     int        `gorm:"column:snapshot_as_exp;not null;default:0"`
	SnapshotLoginDays int        `gorm:"column:snapshot_login_days;not null;default:0"`
	SnapshotMbCertify string     `gorm:"column:snapshot_mb_certify;size:20;not null;default:''"`
	MemberCreatedAt   *time.Time `gorm:"column:member_created_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_created_at;index:idx_member_created,priority:2"`
}

func (MemberLevelHistory) TableName() string {
	return "g5_member_level_history"
}
