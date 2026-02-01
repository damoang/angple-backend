package domain

import "time"

// PluginInstallation 플러그인 설치 상태 엔티티
type PluginInstallation struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	PluginName   string     `gorm:"uniqueIndex;size:100" json:"plugin_name"`
	Version      string     `gorm:"size:50" json:"version"`
	Status       string     `gorm:"size:20;default:disabled" json:"status"` // enabled, disabled, error
	InstalledAt  time.Time  `gorm:"autoCreateTime" json:"installed_at"`
	EnabledAt    *time.Time `json:"enabled_at"`
	DisabledAt   *time.Time `json:"disabled_at"`
	Config       *string    `gorm:"type:json" json:"config"`
	ErrorMessage *string    `gorm:"type:text" json:"error_message"`
	InstalledBy  *string    `gorm:"size:100" json:"installed_by"`
}

// TableName GORM 테이블명
func (PluginInstallation) TableName() string {
	return "plugin_installations"
}

// PluginSetting 플러그인 개별 설정 (key-value)
type PluginSetting struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	PluginName   string    `gorm:"size:100;uniqueIndex:uk_plugin_setting" json:"plugin_name"`
	SettingKey   string    `gorm:"size:200;uniqueIndex:uk_plugin_setting" json:"setting_key"`
	SettingValue *string   `gorm:"type:text" json:"setting_value"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName GORM 테이블명
func (PluginSetting) TableName() string {
	return "plugin_settings"
}

// PluginEvent 플러그인 이벤트 감사 로그
type PluginEvent struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	PluginName string    `gorm:"size:100;index:idx_plugin_event" json:"plugin_name"`
	EventType  string    `gorm:"size:30" json:"event_type"` // installed, enabled, disabled, uninstalled, config_changed, error
	Details    *string   `gorm:"type:json" json:"details"`
	ActorID    *string   `gorm:"size:100" json:"actor_id"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index:idx_plugin_event" json:"created_at"`
}

// TableName GORM 테이블명
func (PluginEvent) TableName() string {
	return "plugin_events"
}

// PluginPermission 플러그인 권한 매핑 (권한 → 최소 회원 레벨)
type PluginPermission struct {
	ID           int64  `gorm:"primaryKey" json:"id"`
	PluginName   string `gorm:"size:100;uniqueIndex:uk_plugin_perm" json:"plugin_name"`
	PermissionID string `gorm:"size:200;uniqueIndex:uk_plugin_perm" json:"permission_id"`
	Label        string `gorm:"size:200" json:"label"`
	MinLevel     int    `gorm:"default:1" json:"min_level"` // 최소 회원 레벨 (1=일반, 10=관리자)
}

// TableName GORM 테이블명
func (PluginPermission) TableName() string {
	return "plugin_permissions"
}

// 이벤트 타입 상수
const (
	EventInstalled     = "installed"
	EventEnabled       = "enabled"
	EventDisabled      = "disabled"
	EventUninstalled   = "uninstalled"
	EventConfigChanged = "config_changed"
	EventError         = "error"
)

// 플러그인 상태 상수
const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusError    = "error"
)
