package domain

import (
	"time"
)

// Category 중고거래 카테고리
type Category struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ParentID  *uint64   `gorm:"column:parent_id;index" json:"parent_id,omitempty"`
	Name      string    `gorm:"column:name;size:50;not null" json:"name"`
	Slug      string    `gorm:"column:slug;size:50;uniqueIndex" json:"slug"`
	Icon      string    `gorm:"column:icon;size:50" json:"icon"`
	OrderNum  int       `gorm:"column:order_num;default:0" json:"order_num"`
	IsActive  bool      `gorm:"column:is_active;default:true" json:"is_active"`
	ItemCount uint      `gorm:"column:item_count;default:0" json:"item_count"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Relations
	Children []Category `gorm:"foreignKey:ParentID" json:"children,omitempty"`
}

func (Category) TableName() string {
	return "marketplace_categories"
}

// CreateCategoryRequest 카테고리 생성 요청
type CreateCategoryRequest struct {
	ParentID *uint64 `json:"parent_id"`
	Name     string  `json:"name" binding:"required,max=50"`
	Slug     string  `json:"slug" binding:"required,max=50"`
	Icon     string  `json:"icon" binding:"max=50"`
	OrderNum int     `json:"order_num"`
}

// UpdateCategoryRequest 카테고리 수정 요청
type UpdateCategoryRequest struct {
	ParentID *uint64 `json:"parent_id"`
	Name     *string `json:"name" binding:"omitempty,max=50"`
	Slug     *string `json:"slug" binding:"omitempty,max=50"`
	Icon     *string `json:"icon" binding:"omitempty,max=50"`
	OrderNum *int    `json:"order_num"`
	IsActive *bool   `json:"is_active"`
}

// CategoryResponse 카테고리 응답
type CategoryResponse struct {
	ID        uint64              `json:"id"`
	ParentID  *uint64             `json:"parent_id,omitempty"`
	Name      string              `json:"name"`
	Slug      string              `json:"slug"`
	Icon      string              `json:"icon"`
	OrderNum  int                 `json:"order_num"`
	IsActive  bool                `json:"is_active"`
	ItemCount uint                `json:"item_count"`
	Children  []*CategoryResponse `json:"children,omitempty"`
}

// ToResponse Category를 CategoryResponse로 변환
func (c *Category) ToResponse() *CategoryResponse {
	resp := &CategoryResponse{
		ID:        c.ID,
		ParentID:  c.ParentID,
		Name:      c.Name,
		Slug:      c.Slug,
		Icon:      c.Icon,
		OrderNum:  c.OrderNum,
		IsActive:  c.IsActive,
		ItemCount: c.ItemCount,
	}

	if len(c.Children) > 0 {
		resp.Children = make([]*CategoryResponse, len(c.Children))
		for i, child := range c.Children {
			resp.Children[i] = child.ToResponse()
		}
	}

	return resp
}
