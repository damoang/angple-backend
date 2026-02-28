package v2

import "time"

// V2BoardDisplaySettings represents display settings for a board
// This is stored separately from V2Board for flexibility and
// to support multiple display configurations per board.
// NOTE: board_id is varchar(20) PRIMARY KEY (board slug), matching existing table schema
type V2BoardDisplaySettings struct {
	BoardID       string    `gorm:"column:board_id;type:varchar(20);primaryKey" json:"board_id"`
	ListLayout    string    `gorm:"column:list_layout;type:varchar(30);default:'compact'" json:"list_layout"`
	ViewLayout    string    `gorm:"column:view_layout;type:varchar(30);default:'basic'" json:"view_layout"`
	ShowPreview   bool      `gorm:"column:show_preview;default:false" json:"show_preview"`
	PreviewLength int       `gorm:"column:preview_length;default:150" json:"preview_length"`
	ShowThumbnail bool      `gorm:"column:show_thumbnail;default:false" json:"show_thumbnail"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for V2BoardDisplaySettings
func (V2BoardDisplaySettings) TableName() string { return "v2_board_display_settings" }

// BoardDisplaySettingsResponse is the API response format
type BoardDisplaySettingsResponse struct {
	ListLayout    string `json:"list_layout"`
	ViewLayout    string `json:"view_layout"`
	ListStyle     string `json:"list_style,omitempty"` // Legacy compatibility
	ShowPreview   bool   `json:"show_preview"`
	PreviewLength int    `json:"preview_length"`
	ShowThumbnail bool   `json:"show_thumbnail"`
}

// UpdateDisplaySettingsRequest is the API request format for updating settings
type UpdateDisplaySettingsRequest struct {
	ListLayout    *string `json:"list_layout,omitempty"`
	ViewLayout    *string `json:"view_layout,omitempty"`
	ShowPreview   *bool   `json:"show_preview,omitempty"`
	PreviewLength *int    `json:"preview_length,omitempty"`
	ShowThumbnail *bool   `json:"show_thumbnail,omitempty"`
}

// ToResponse converts V2BoardDisplaySettings to API response format
func (s *V2BoardDisplaySettings) ToResponse() *BoardDisplaySettingsResponse {
	return &BoardDisplaySettingsResponse{
		ListLayout:    s.ListLayout,
		ViewLayout:    s.ViewLayout,
		ListStyle:     s.ListLayout, // Legacy compatibility
		ShowPreview:   s.ShowPreview,
		PreviewLength: s.PreviewLength,
		ShowThumbnail: s.ShowThumbnail,
	}
}
