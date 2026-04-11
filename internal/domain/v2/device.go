package v2

import "time"

// V2Device represents a registered push notification device for a user.
type V2Device struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID     uint64    `gorm:"column:user_id;not null;index" json:"user_id"`
	Token      string    `gorm:"column:token;type:varchar(255);not null;uniqueIndex" json:"token"`
	Platform   string    `gorm:"column:platform;type:varchar(16);not null" json:"platform"`
	AppVersion string    `gorm:"column:app_version;type:varchar(32)" json:"app_version,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2Device) TableName() string { return "v2_devices" }
