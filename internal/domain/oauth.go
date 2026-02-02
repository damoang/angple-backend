package domain

import "time"

// OAuthProvider represents supported OAuth providers
type OAuthProvider string

const (
	OAuthProviderNaver  OAuthProvider = "naver"
	OAuthProviderKakao  OAuthProvider = "kakao"
	OAuthProviderGoogle OAuthProvider = "google"
)

// OAuthAccount links an external OAuth account to a local member
type OAuthAccount struct {
	ID           int64         `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID       string        `gorm:"column:user_id;index" json:"user_id"`
	Provider     OAuthProvider `gorm:"column:provider;index" json:"provider"`
	ProviderUID  string        `gorm:"column:provider_uid" json:"provider_uid"`
	Email        string        `gorm:"column:email" json:"email"`
	Name         string        `gorm:"column:name" json:"name"`
	ProfileImage string        `gorm:"column:profile_image" json:"profile_image"`
	AccessToken  string        `gorm:"column:access_token" json:"-"`
	RefreshToken string        `gorm:"column:refresh_token" json:"-"`
	ExpiresAt    *time.Time    `gorm:"column:expires_at" json:"expires_at"`
	CreatedAt    time.Time     `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time     `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (OAuthAccount) TableName() string {
	return "oauth_accounts"
}

// OAuthConfig holds configuration for an OAuth provider
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// OAuthUserInfo represents user info retrieved from an OAuth provider
type OAuthUserInfo struct {
	Provider     OAuthProvider `json:"provider"`
	ProviderUID  string        `json:"provider_uid"`
	Email        string        `json:"email"`
	Name         string        `json:"name"`
	Nickname     string        `json:"nickname"`
	ProfileImage string        `json:"profile_image"`
}

// OAuthLoginResponse is returned after successful OAuth login
type OAuthLoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IsNewUser    bool   `json:"is_new_user"`
	UserID       string `json:"user_id"`
}

// APIKey represents an API key for external integrations
type APIKey struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Key       string    `gorm:"column:api_key;uniqueIndex;size:64" json:"key"`
	Name      string    `gorm:"column:name" json:"name"`
	UserID    string    `gorm:"column:user_id;index" json:"user_id"`
	Scopes    string    `gorm:"column:scopes" json:"scopes"` // comma-separated: read,write,admin
	Active    bool      `gorm:"column:active;default:true" json:"active"`
	ExpiresAt *time.Time `gorm:"column:expires_at" json:"expires_at"`
	LastUsed  *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (APIKey) TableName() string {
	return "api_keys"
}
