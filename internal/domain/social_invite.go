package domain

import "time"

// SocialInvite is a one-time token for moving social profiles to a restored member.
type SocialInvite struct {
	ID           int        `gorm:"primaryKey" json:"id"`
	Token        string     `gorm:"column:token;type:varchar(32);uniqueIndex" json:"token"`
	TargetMbID   string     `gorm:"column:target_mb_id;type:varchar(20)" json:"target_mb_id"`
	TargetMbNick string     `gorm:"column:target_mb_nick;type:varchar(50)" json:"target_mb_nick"`
	CreatedBy    string     `gorm:"column:created_by;type:varchar(20)" json:"created_by"`
	ExpiresAt    time.Time  `gorm:"column:expires_at" json:"expires_at"`
	UsedAt       *time.Time `gorm:"column:used_at" json:"used_at"`
	UsedBy       *string    `gorm:"column:used_by;type:varchar(20)" json:"used_by"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (SocialInvite) TableName() string { return "social_invites" }

type CreateSocialInviteRequest struct {
	TargetMbID string `json:"target_mb_id" binding:"required"`
}

type SocialInviteInfoResponse struct {
	TargetMbID        string            `json:"target_mb_id"`
	TargetMbNick      string            `json:"target_mb_nick"`
	ExpiresAt         string            `json:"expires_at"`
	CurrentUserMbID   string            `json:"current_user_mb_id,omitempty"`
	CurrentUserMbNick string            `json:"current_user_mb_nick,omitempty"`
	CurrentSocials    []SocialInviteRef `json:"current_socials,omitempty"`
}

type SocialInviteRef struct {
	Provider   string `json:"provider"`
	SocialName string `json:"social_name"`
}

type SocialInviteCreateResponse struct {
	Token               string `json:"token"`
	URL                 string `json:"url"`
	ExpiresAt           string `json:"expires_at"`
	EmailTemplate       string `json:"email_template"`
	EmailTemplateNoCert string `json:"email_template_no_cert"`
}

type SocialProfile struct {
	MpNo        int    `gorm:"column:mp_no;primaryKey" json:"mp_no"`
	MbID        string `gorm:"column:mb_id" json:"mb_id"`
	Provider    string `gorm:"column:provider" json:"provider"`
	Identifier  string `gorm:"column:identifier" json:"identifier"`
	DisplayName string `gorm:"column:displayname" json:"displayname"`
	RegisterDay string `gorm:"column:mp_register_day" json:"register_day"`
	LatestDay   string `gorm:"column:mp_latest_day" json:"latest_day"`
}

func (SocialProfile) TableName() string { return "g5_member_social_profiles" }

type RecoveryLog struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	MbID      string    `gorm:"column:mb_id" json:"mb_id"`
	AdminID   string    `gorm:"column:admin_id" json:"admin_id"`
	Action    string    `gorm:"column:action" json:"action"`
	Details   string    `gorm:"column:details" json:"details"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (RecoveryLog) TableName() string { return "recovery_logs" }
