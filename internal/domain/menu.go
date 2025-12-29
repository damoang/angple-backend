package domain

import (
	"time"
)

// Menu domain model for angple menu system
// Table: menus
type Menu struct {
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	ParentID      *int64    `gorm:"column:parent_id" json:"parent_id"`
	Target        string    `gorm:"column:target" json:"target"`
	Title         string    `gorm:"column:title" json:"title"`
	URL           string    `gorm:"column:url" json:"url"`
	Icon          string    `gorm:"column:icon" json:"icon,omitempty"`
	Shortcut      string    `gorm:"column:shortcut" json:"shortcut,omitempty"`
	Description   string    `gorm:"column:description" json:"description,omitempty"`
	Children      []*Menu   `gorm:"-" json:"children,omitempty"`
	OrderNum      int       `gorm:"column:order_num" json:"order_num"`
	ViewLevel     int       `gorm:"column:view_level" json:"view_level"`
	ID            int64     `gorm:"column:id;primaryKey" json:"id"`
	Depth         int       `gorm:"column:depth" json:"depth"`
	ShowInHeader  bool      `gorm:"column:show_in_header" json:"show_in_header"`
	ShowInSidebar bool      `gorm:"column:show_in_sidebar" json:"show_in_sidebar"`
	IsActive      bool      `gorm:"column:is_active" json:"is_active"`
}

// TableName specifies the table name for Menu model
func (Menu) TableName() string {
	return "menus"
}

// MenuResponse is the API response format
type MenuResponse struct {
	ParentID      *int64         `json:"parent_id"`
	Description   string         `json:"description,omitempty"`
	Title         string         `json:"title"`
	URL           string         `json:"url"`
	Icon          string         `json:"icon,omitempty"`
	Shortcut      string         `json:"shortcut,omitempty"`
	Target        string         `json:"target"`
	Children      []MenuResponse `json:"children,omitempty"`
	ID            int64          `json:"id"`
	Depth         int            `json:"depth"`
	OrderNum      int            `json:"order_num"`
	ShowInHeader  bool           `json:"show_in_header"`
	ShowInSidebar bool           `json:"show_in_sidebar"`
}

// ToResponse converts Menu to MenuResponse
func (m *Menu) ToResponse() MenuResponse {
	resp := MenuResponse{
		ID:            m.ID,
		ParentID:      m.ParentID,
		Title:         m.Title,
		URL:           m.URL,
		Icon:          m.Icon,
		Shortcut:      m.Shortcut,
		Description:   m.Description,
		Depth:         m.Depth,
		OrderNum:      m.OrderNum,
		Target:        m.Target,
		ShowInHeader:  m.ShowInHeader,
		ShowInSidebar: m.ShowInSidebar,
		Children:      make([]MenuResponse, 0),
	}

	// Convert children recursively
	if len(m.Children) > 0 {
		for _, child := range m.Children {
			resp.Children = append(resp.Children, child.ToResponse())
		}
	}

	return resp
}

// MenuListResponse is the response for list of menus
type MenuListResponse struct {
	Sidebar []MenuResponse `json:"sidebar"`
	Header  []MenuResponse `json:"header"`
}
