package domain

import (
	"time"
)

// Site represents a tenant site in multi-tenant architecture
type Site struct {
	ID           string     `gorm:"column:id;primaryKey" json:"id"`
	Subdomain    string     `gorm:"column:subdomain;uniqueIndex" json:"subdomain"`
	SiteName     string     `gorm:"column:site_name" json:"site_name"`
	OwnerEmail   string     `gorm:"column:owner_email" json:"owner_email"`
	Plan         string     `gorm:"column:plan" json:"plan"`               // free, pro, business, enterprise
	DBStrategy   string     `gorm:"column:db_strategy" json:"db_strategy"` // shared, schema, dedicated
	DBSchemaName *string    `gorm:"column:db_schema_name" json:"db_schema_name,omitempty"`
	DBHost       *string    `gorm:"column:db_host" json:"db_host,omitempty"`
	DBPort       int        `gorm:"column:db_port;default:3306" json:"db_port,omitempty"`
	Active       bool       `gorm:"column:active;default:true" json:"active"`
	Suspended    bool       `gorm:"column:suspended;default:false" json:"suspended"`
	TrialEndsAt  *time.Time `gorm:"column:trial_ends_at" json:"trial_ends_at,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Site) TableName() string {
	return "sites"
}

// SiteSettings represents site-specific configuration
type SiteSettings struct {
	SiteID            string    `gorm:"column:site_id;primaryKey" json:"site_id"`
	ActiveTheme       string    `gorm:"column:active_theme;default:damoang-official" json:"active_theme"`
	LogoURL           *string   `gorm:"column:logo_url" json:"logo_url,omitempty"`
	FaviconURL        *string   `gorm:"column:favicon_url" json:"favicon_url,omitempty"`
	PrimaryColor      string    `gorm:"column:primary_color;default:#3b82f6" json:"primary_color"`
	SecondaryColor    string    `gorm:"column:secondary_color;default:#8b5cf6" json:"secondary_color"`
	SiteDescription   *string   `gorm:"column:site_description" json:"site_description,omitempty"`
	SiteKeywords      *string   `gorm:"column:site_keywords" json:"site_keywords,omitempty"`
	GoogleAnalyticsID *string   `gorm:"column:google_analytics_id" json:"google_analytics_id,omitempty"`
	CustomDomain      *string   `gorm:"column:custom_domain" json:"custom_domain,omitempty"`
	SSLEnabled        bool      `gorm:"column:ssl_enabled;default:true" json:"ssl_enabled"`
	SettingsJSON      *string   `gorm:"column:settings_json;type:json" json:"settings_json,omitempty"`
	CreatedAt         time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (SiteSettings) TableName() string {
	return "site_settings"
}

// SiteUser represents user permissions for a site
type SiteUser struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SiteID    string    `gorm:"column:site_id;uniqueIndex:idx_site_user" json:"site_id"`
	UserID    string    `gorm:"column:user_id;uniqueIndex:idx_site_user" json:"user_id"`
	Role      string    `gorm:"column:role;default:viewer" json:"role"` // owner, admin, editor, viewer
	InvitedBy *string   `gorm:"column:invited_by" json:"invited_by,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (SiteUser) TableName() string {
	return "site_users"
}

// SiteUsage represents daily resource usage tracking
type SiteUsage struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SiteID          string    `gorm:"column:site_id;uniqueIndex:idx_site_date" json:"site_id"`
	Date            time.Time `gorm:"column:date;type:date;uniqueIndex:idx_site_date" json:"date"`
	UniqueVisitors  int       `gorm:"column:unique_visitors;default:0" json:"unique_visitors"`
	PageViews       int       `gorm:"column:page_views;default:0" json:"page_views"`
	PostsCreated    int       `gorm:"column:posts_created;default:0" json:"posts_created"`
	CommentsCreated int       `gorm:"column:comments_created;default:0" json:"comments_created"`
	StorageUsedMB   float64   `gorm:"column:storage_used_mb;type:decimal(10,2);default:0" json:"storage_used_mb"`
	BandwidthUsedMB float64   `gorm:"column:bandwidth_used_mb;type:decimal(10,2);default:0" json:"bandwidth_used_mb"`
	APICalls        int       `gorm:"column:api_calls;default:0" json:"api_calls"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (SiteUsage) TableName() string {
	return "site_usage"
}

// ========================================
// Response DTOs
// ========================================

// SiteResponse is the API response for a site with its settings
type SiteResponse struct {
	ID              string  `json:"id"`
	Subdomain       string  `json:"subdomain"`
	SiteName        string  `json:"site_name"`
	OwnerEmail      string  `json:"owner_email"`
	Plan            string  `json:"plan"`
	DBStrategy      string  `json:"db_strategy"`
	Active          bool    `json:"active"`
	Suspended       bool    `json:"suspended"`
	ActiveTheme     string  `json:"active_theme"`
	LogoURL         *string `json:"logo_url,omitempty"`
	FaviconURL      *string `json:"favicon_url,omitempty"`
	PrimaryColor    string  `json:"primary_color"`
	SecondaryColor  string  `json:"secondary_color"`
	SiteDescription *string `json:"site_description,omitempty"`
	CustomDomain    *string `json:"custom_domain,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

// ToResponse converts Site + SiteSettings to SiteResponse
func (s *Site) ToResponse(settings *SiteSettings) *SiteResponse {
	resp := &SiteResponse{
		ID:         s.ID,
		Subdomain:  s.Subdomain,
		SiteName:   s.SiteName,
		OwnerEmail: s.OwnerEmail,
		Plan:       s.Plan,
		DBStrategy: s.DBStrategy,
		Active:     s.Active,
		Suspended:  s.Suspended,
		CreatedAt:  s.CreatedAt.Format(time.RFC3339),
	}

	if settings != nil {
		resp.ActiveTheme = settings.ActiveTheme
		resp.LogoURL = settings.LogoURL
		resp.FaviconURL = settings.FaviconURL
		resp.PrimaryColor = settings.PrimaryColor
		resp.SecondaryColor = settings.SecondaryColor
		resp.SiteDescription = settings.SiteDescription
		resp.CustomDomain = settings.CustomDomain
	}

	return resp
}

// ========================================
// Request DTOs
// ========================================

// CreateSiteRequest is the request body for creating a new site
type CreateSiteRequest struct {
	Subdomain      string  `json:"subdomain" validate:"required,min=3,max=50,alphanum"`
	SiteName       string  `json:"site_name" validate:"required,min=1,max=100"`
	OwnerEmail     string  `json:"owner_email" validate:"required,email"`
	Plan           string  `json:"plan" validate:"required,oneof=free pro business enterprise"`
	ActiveTheme    *string `json:"active_theme,omitempty"`
	PrimaryColor   *string `json:"primary_color,omitempty"`
	SecondaryColor *string `json:"secondary_color,omitempty"`
}

// UpdateSiteSettingsRequest is the request body for updating site settings
type UpdateSiteSettingsRequest struct {
	ActiveTheme       *string `json:"active_theme,omitempty"`
	LogoURL           *string `json:"logo_url,omitempty"`
	FaviconURL        *string `json:"favicon_url,omitempty"`
	PrimaryColor      *string `json:"primary_color,omitempty"`
	SecondaryColor    *string `json:"secondary_color,omitempty"`
	SiteDescription   *string `json:"site_description,omitempty"`
	SiteKeywords      *string `json:"site_keywords,omitempty"`
	GoogleAnalyticsID *string `json:"google_analytics_id,omitempty"`
	CustomDomain      *string `json:"custom_domain,omitempty"`
}
