package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// AdType 광고 유형
type AdType string

const (
	AdTypeGAM     AdType = "gam"
	AdTypeAdsense AdType = "adsense"
)

// RotationStrategy 로테이션 전략
type RotationStrategy string

const (
	RotationSequential RotationStrategy = "sequential"
	RotationRandom     RotationStrategy = "random"
	RotationWeighted   RotationStrategy = "weighted"
)

// JSON 타입 헬퍼
type JSONSlice []interface{}

func (j *JSONSlice) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan JSONSlice")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// StringSlice JSON 배열 타입
type StringSlice []string

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan StringSlice")
	}
	return json.Unmarshal(bytes, s)
}

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// IntSliceSlice 2D int 배열 (광고 사이즈용)
type IntSliceSlice [][]int

func (s *IntSliceSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan IntSliceSlice")
	}
	return json.Unmarshal(bytes, s)
}

func (s IntSliceSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// ResponsiveConfig 반응형 설정 타입
type ResponsiveConfig [][]interface{}

func (r *ResponsiveConfig) Scan(value interface{}) error {
	if value == nil {
		*r = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan ResponsiveConfig")
	}
	return json.Unmarshal(bytes, r)
}

func (r ResponsiveConfig) Value() (driver.Value, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r)
}

// AdUnit 광고 단위 (GAM/AdSense)
type AdUnit struct {
	ID                    uint64           `gorm:"primaryKey" json:"id"`
	Name                  string           `gorm:"size:50;not null" json:"name"`
	AdType                AdType           `gorm:"column:ad_type;size:20;not null" json:"ad_type"`
	GAMUnitPath           string           `gorm:"column:gam_unit_path;size:255" json:"gam_unit_path,omitempty"`
	AdsenseSlot           string           `gorm:"column:adsense_slot;size:50" json:"adsense_slot,omitempty"`
	AdsenseClient         string           `gorm:"column:adsense_client;size:50" json:"adsense_client,omitempty"`
	Sizes                 IntSliceSlice    `gorm:"column:sizes;type:json" json:"sizes,omitempty"`
	ResponsiveBreakpoints ResponsiveConfig `gorm:"column:responsive_breakpoints;type:json" json:"responsive_breakpoints,omitempty"`
	Position              string           `gorm:"size:50" json:"position"`
	Priority              int              `gorm:"default:0" json:"priority"`
	IsActive              bool             `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt             time.Time        `gorm:"column:created_at" json:"created_at"`
	UpdatedAt             time.Time        `gorm:"column:updated_at" json:"updated_at"`
}

func (AdUnit) TableName() string {
	return "ad_units"
}

// AdRotationConfig AdSense 로테이션 설정
type AdRotationConfig struct {
	ID               uint64           `gorm:"primaryKey" json:"id"`
	Position         string           `gorm:"size:50;not null" json:"position"`
	SlotPool         StringSlice      `gorm:"column:slot_pool;type:json;not null" json:"slot_pool"`
	RotationStrategy RotationStrategy `gorm:"column:rotation_strategy;size:20;default:'sequential'" json:"rotation_strategy"`
	CreatedAt        time.Time        `gorm:"column:created_at" json:"created_at"`
}

func (AdRotationConfig) TableName() string {
	return "ad_rotation_config"
}

// CelebrationBanner 축하 배너
type CelebrationBanner struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	Title          string    `gorm:"size:255;not null" json:"title"`
	Content        string    `gorm:"type:text" json:"content,omitempty"`
	ImageURL       string    `gorm:"column:image_url;size:500" json:"image_url,omitempty"`
	LinkURL        string    `gorm:"column:link_url;size:500" json:"link_url,omitempty"`
	ExternalURL    string    `gorm:"column:external_url;size:500" json:"external_url,omitempty"`
	DisplayDate    time.Time `gorm:"column:display_date;type:date;not null" json:"display_date"`
	YearlyRepeat   bool      `gorm:"column:yearly_repeat;default:false" json:"yearly_repeat"`
	LinkTarget     string    `gorm:"column:link_target;size:20;default:_blank" json:"link_target"`
	SortOrder      int       `gorm:"column:sort_order;default:0" json:"sort_order"`
	TargetMemberID string    `gorm:"column:target_member_id;size:20" json:"target_member_id,omitempty"`
	IsActive       bool      `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (CelebrationBanner) TableName() string {
	return "celebration_banners"
}

// AdUnitResponse 광고 단위 응답 DTO
type AdUnitResponse struct {
	ID                    uint64          `json:"id"`
	Name                  string          `json:"name"`
	AdType                AdType          `json:"ad_type"`
	GAMUnitPath           string          `json:"gam_unit_path,omitempty"`
	AdsenseSlot           string          `json:"adsense_slot,omitempty"`
	AdsenseClient         string          `json:"adsense_client,omitempty"`
	Sizes                 [][]int         `json:"sizes,omitempty"`
	ResponsiveBreakpoints [][]interface{} `json:"responsive_breakpoints,omitempty"`
	Position              string          `json:"position"`
	Priority              int             `json:"priority"`
	IsActive              bool            `json:"is_active"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// ToResponse AdUnit을 AdUnitResponse로 변환
func (u *AdUnit) ToResponse() *AdUnitResponse {
	return &AdUnitResponse{
		ID:                    u.ID,
		Name:                  u.Name,
		AdType:                u.AdType,
		GAMUnitPath:           u.GAMUnitPath,
		AdsenseSlot:           u.AdsenseSlot,
		AdsenseClient:         u.AdsenseClient,
		Sizes:                 u.Sizes,
		ResponsiveBreakpoints: u.ResponsiveBreakpoints,
		Position:              u.Position,
		Priority:              u.Priority,
		IsActive:              u.IsActive,
		CreatedAt:             u.CreatedAt,
		UpdatedAt:             u.UpdatedAt,
	}
}

// CelebrationBannerResponse 축하 배너 응답 DTO
type CelebrationBannerResponse struct {
	ID             uint64 `json:"id"`
	Title          string `json:"title"`
	Content        string `json:"content,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	LinkURL        string `json:"link_url,omitempty"`
	ExternalLink   string `json:"external_link,omitempty"`
	DisplayDate    string `json:"display_date"`
	YearlyRepeat   bool   `json:"yearly_repeat"`
	LinkTarget     string `json:"link_target"`
	SortOrder      int    `json:"sort_order"`
	TargetMemberID string `json:"target_member_id,omitempty"`
	IsActive       bool   `json:"is_active"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ToResponse CelebrationBanner를 응답 DTO로 변환
func (b *CelebrationBanner) ToResponse() *CelebrationBannerResponse {
	return &CelebrationBannerResponse{
		ID:             b.ID,
		Title:          b.Title,
		Content:        b.Content,
		ImageURL:       b.ImageURL,
		LinkURL:        b.LinkURL,
		ExternalLink:   b.ExternalURL,
		DisplayDate:    b.DisplayDate.Format("2006-01-02"),
		YearlyRepeat:   b.YearlyRepeat,
		LinkTarget:     b.LinkTarget,
		SortOrder:      b.SortOrder,
		TargetMemberID: b.TargetMemberID,
		IsActive:       b.IsActive,
		CreatedAt:      b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      b.UpdatedAt.Format(time.RFC3339),
	}
}

// CreateAdUnitRequest 광고 단위 생성 요청
type CreateAdUnitRequest struct {
	Name                  string          `json:"name" binding:"required,max=50"`
	AdType                AdType          `json:"ad_type" binding:"required,oneof=gam adsense"`
	GAMUnitPath           string          `json:"gam_unit_path,omitempty"`
	AdsenseSlot           string          `json:"adsense_slot,omitempty"`
	AdsenseClient         string          `json:"adsense_client,omitempty"`
	Sizes                 [][]int         `json:"sizes,omitempty"`
	ResponsiveBreakpoints [][]interface{} `json:"responsive_breakpoints,omitempty"`
	Position              string          `json:"position" binding:"required,max=50"`
	Priority              int             `json:"priority"`
	IsActive              *bool           `json:"is_active"`
}

// UpdateAdUnitRequest 광고 단위 수정 요청
type UpdateAdUnitRequest struct {
	Name                  *string         `json:"name,omitempty"`
	AdType                *AdType         `json:"ad_type,omitempty"`
	GAMUnitPath           *string         `json:"gam_unit_path,omitempty"`
	AdsenseSlot           *string         `json:"adsense_slot,omitempty"`
	AdsenseClient         *string         `json:"adsense_client,omitempty"`
	Sizes                 [][]int         `json:"sizes,omitempty"`
	ResponsiveBreakpoints [][]interface{} `json:"responsive_breakpoints,omitempty"`
	Position              *string         `json:"position,omitempty"`
	Priority              *int            `json:"priority,omitempty"`
	IsActive              *bool           `json:"is_active,omitempty"`
}

// CreateBannerRequest 축하 배너 생성 요청
type CreateBannerRequest struct {
	Title          string `json:"title" binding:"required,max=255"`
	Content        string `json:"content,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	LinkURL        string `json:"link_url,omitempty"`
	DisplayDate    string `json:"display_date" binding:"required"` // YYYY-MM-DD
	YearlyRepeat   bool   `json:"yearly_repeat"`
	LinkTarget     string `json:"link_target,omitempty"`
	SortOrder      int    `json:"sort_order"`
	TargetMemberID string `json:"target_member_id,omitempty"`
	IsActive       *bool  `json:"is_active"`
}

// UpdateBannerRequest 축하 배너 수정 요청
type UpdateBannerRequest struct {
	Title          *string `json:"title,omitempty"`
	Content        *string `json:"content,omitempty"`
	ImageURL       *string `json:"image_url,omitempty"`
	LinkURL        *string `json:"link_url,omitempty"`
	DisplayDate    *string `json:"display_date,omitempty"`
	YearlyRepeat   *bool   `json:"yearly_repeat,omitempty"`
	LinkTarget     *string `json:"link_target,omitempty"`
	SortOrder      *int    `json:"sort_order,omitempty"`
	TargetMemberID *string `json:"target_member_id,omitempty"`
	IsActive       *bool   `json:"is_active,omitempty"`
}

// GAMConfigResponse GAM 전역 설정 응답
type GAMConfigResponse struct {
	NetworkCode    string                   `json:"network_code"`
	EnableGAM      bool                     `json:"enable_gam"`
	EnableFallback bool                     `json:"enable_fallback"`
	AdUnits        map[string]*AdUnitConfig `json:"ad_units"`
	PositionMap    map[string]string        `json:"position_map"`
}

// AdUnitConfig 단일 광고 단위 설정
type AdUnitConfig struct {
	Unit       string          `json:"unit"`
	Sizes      [][]int         `json:"sizes"`
	Responsive [][]interface{} `json:"responsive,omitempty"`
}

// AdsenseConfigResponse AdSense 전역 설정 응답
type AdsenseConfigResponse struct {
	ClientID string                       `json:"client_id"`
	Slots    map[string]*AdsenseSlotGroup `json:"slots"`
}

// AdsenseSlotGroup AdSense 슬롯 그룹
type AdsenseSlotGroup struct {
	Slots []string `json:"slots"`
}

// AdPositionResponse 특정 위치 광고 응답
type AdPositionResponse struct {
	Position      string           `json:"position"`
	GAM           *AdUnitConfig    `json:"gam,omitempty"`
	Adsense       *AdsenseSlotInfo `json:"adsense,omitempty"`
	RotationIndex int              `json:"rotation_index"`
}

// AdsenseSlotInfo AdSense 슬롯 정보
type AdsenseSlotInfo struct {
	ClientID string `json:"client_id"`
	Slot     string `json:"slot"`
	Style    string `json:"style,omitempty"`
}

// InfeedConfigResponse 인피드 광고 설정 응답
type InfeedConfigResponse struct {
	Enabled  bool   `json:"enabled"`
	Interval int    `json:"interval"`
	Position string `json:"position"`
}
