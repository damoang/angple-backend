package domain

import (
	"time"
)

// Menu domain model for angple menu system
// Table: menus
type Menu struct {
	ID             int64     `gorm:"column:id;primaryKey" json:"id"`
	ParentID       *int64    `gorm:"column:parent_id" json:"parent_id"`
	Title          string    `gorm:"column:title" json:"title"`
	URL            string    `gorm:"column:url" json:"url"`
	Icon           string    `gorm:"column:icon" json:"icon,omitempty"`
	Shortcut       string    `gorm:"column:shortcut" json:"shortcut,omitempty"`
	Description    string    `gorm:"column:description" json:"description,omitempty"`
	Depth          int       `gorm:"column:depth" json:"depth"`
	OrderNum       int       `gorm:"column:order_num" json:"order_num"`
	IsActive       bool      `gorm:"column:is_active" json:"is_active"`
	Target         string    `gorm:"column:target" json:"target"`
	ViewLevel      int       `gorm:"column:view_level" json:"view_level"`
	ShowInHeader   bool      `gorm:"column:show_in_header" json:"show_in_header"`
	ShowInSidebar  bool      `gorm:"column:show_in_sidebar" json:"show_in_sidebar"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
	Children       []*Menu   `gorm:"-" json:"children,omitempty"`
}

// TableName specifies the table name for Menu model
func (Menu) TableName() string {
	return "menus"
}

// MenuResponse is the API response format
type MenuResponse struct {
	ID            int64           `json:"id"`
	ParentID      *int64          `json:"parent_id"`
	Title         string          `json:"title"`
	URL           string          `json:"url"`
	Icon          string          `json:"icon,omitempty"`
	Shortcut      string          `json:"shortcut,omitempty"`
	Description   string          `json:"description,omitempty"`
	Depth         int             `json:"depth"`
	OrderNum      int             `json:"order_num"`
	Target        string          `json:"target"`
	ShowInHeader  bool            `json:"show_in_header"`
	ShowInSidebar bool            `json:"show_in_sidebar"`
	Children      []MenuResponse  `json:"children,omitempty"`
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
