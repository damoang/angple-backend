package v2

import "time"

// SiteLogo represents a site logo with optional scheduling
type SiteLogo struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"column:name;type:varchar(100);not null" json:"name"`
	LogoURL       string    `gorm:"column:logo_url;type:varchar(500);not null" json:"logo_url"`
	ScheduleType  string    `gorm:"column:schedule_type;type:enum('recurring','date_range','default');not null" json:"schedule_type"`
	RecurringDate *string   `gorm:"column:recurring_date;type:varchar(5)" json:"recurring_date,omitempty"`
	StartDate     *string   `gorm:"column:start_date;type:date" json:"start_date,omitempty"`
	EndDate       *string   `gorm:"column:end_date;type:date" json:"end_date,omitempty"`
	Priority      int       `gorm:"column:priority;default:0" json:"priority"`
	IsActive      bool      `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for SiteLogo
func (SiteLogo) TableName() string { return "site_logos" }

// CreateSiteLogoRequest represents a request to create a site logo
type CreateSiteLogoRequest struct {
	Name          string  `json:"name" binding:"required"`
	LogoURL       string  `json:"logo_url" binding:"required"`
	ScheduleType  string  `json:"schedule_type" binding:"required,oneof=recurring date_range default"`
	RecurringDate *string `json:"recurring_date,omitempty"`
	StartDate     *string `json:"start_date,omitempty"`
	EndDate       *string `json:"end_date,omitempty"`
	Priority      int     `json:"priority"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

// UpdateSiteLogoRequest represents a request to update a site logo
type UpdateSiteLogoRequest struct {
	Name          *string `json:"name,omitempty"`
	LogoURL       *string `json:"logo_url,omitempty"`
	ScheduleType  *string `json:"schedule_type,omitempty" binding:"omitempty,oneof=recurring date_range default"`
	RecurringDate *string `json:"recurring_date,omitempty"`
	StartDate     *string `json:"start_date,omitempty"`
	EndDate       *string `json:"end_date,omitempty"`
	Priority      *int    `json:"priority,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

// PresetLogoItemRequest represents a single seasonal preset logo entry.
type PresetLogoItemRequest struct {
	Name          string `json:"name" binding:"required"`
	RecurringDate string `json:"recurring_date" binding:"required"`
}

// CreatePresetLogosRequest represents a batch request to create seasonal preset logos.
type CreatePresetLogosRequest struct {
	LogoURL  string                  `json:"logo_url" binding:"required"`
	Priority int                     `json:"priority"`
	IsActive *bool                   `json:"is_active,omitempty"`
	Items    []PresetLogoItemRequest `json:"items" binding:"required,min=1,dive"`
}

// CreatePresetLogosResult represents the result of seasonal preset logo creation.
type CreatePresetLogosResult struct {
	Created []*SiteLogo               `json:"created"`
	Skipped []CreatePresetLogoSkipped `json:"skipped"`
}

// CreatePresetLogoSkipped represents a preset item skipped during batch creation.
type CreatePresetLogoSkipped struct {
	Name          string `json:"name"`
	RecurringDate string `json:"recurring_date"`
	Reason        string `json:"reason"`
}
