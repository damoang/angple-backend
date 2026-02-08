package domain

import "time"

// BoardDisplaySettingsModel represents the v2_board_display_settings table
// g5_board를 수정하지 않고 별도 테이블에서 표시 설정을 관리한다.
type BoardDisplaySettingsModel struct {
	BoardID       string    `gorm:"column:board_id;primaryKey;size:20" json:"board_id"`
	ListLayout    string    `gorm:"column:list_layout;size:30;default:compact" json:"list_layout"`
	ViewLayout    string    `gorm:"column:view_layout;size:30;default:basic" json:"view_layout"`
	ShowPreview   bool      `gorm:"column:show_preview;default:false" json:"show_preview"`
	PreviewLength int       `gorm:"column:preview_length;default:150" json:"preview_length"`
	ShowThumbnail bool      `gorm:"column:show_thumbnail;default:false" json:"show_thumbnail"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (BoardDisplaySettingsModel) TableName() string {
	return "v2_board_display_settings"
}

// UpdateDisplaySettingsRequest - 게시판 표시 설정 수정 요청 DTO
type UpdateDisplaySettingsRequest struct {
	ListLayout    *string `json:"list_layout,omitempty"`
	ViewLayout    *string `json:"view_layout,omitempty"`
	ShowPreview   *bool   `json:"show_preview,omitempty"`
	PreviewLength *int    `json:"preview_length,omitempty"`
	ShowThumbnail *bool   `json:"show_thumbnail,omitempty"`
}

// ToDisplaySettings converts to the DTO used in API responses
func (m *BoardDisplaySettingsModel) ToDisplaySettings() BoardDisplaySettings {
	return BoardDisplaySettings{
		ListLayout:    m.ListLayout,
		ViewLayout:    m.ViewLayout,
		ListStyle:     m.ListLayout, // 하위호환: list_style = list_layout
		ShowPreview:   m.ShowPreview,
		PreviewLength: m.PreviewLength,
		ShowThumbnail: m.ShowThumbnail,
	}
}
