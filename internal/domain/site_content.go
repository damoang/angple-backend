package domain

import (
	"time"
)

// AngpleSiteContent stores block-based page content for the Angple Sites builder (M1 A1).
// PoC scope (issue #1288): site_id FK omitted; will be added in Phase 2 once `sites` table is in prod.
//
// schema_version=1: blocks JSON shape is `{schema_version, blocks: [{id, type, data, meta}, ...]}`.
type AngpleSiteContent struct {
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"          json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"          json:"updated_at"`
	Meta          *string   `gorm:"column:meta;type:json"                     json:"meta,omitempty"`
	Blocks        string    `gorm:"column:blocks;type:json;not null"          json:"blocks"`
	ContentKey    string    `gorm:"column:content_key;not null"               json:"content_key"`
	ID            int64     `gorm:"column:id;primaryKey;autoIncrement"        json:"id"`
	SiteID        int64     `gorm:"column:site_id;not null"                   json:"site_id"`
	SchemaVersion uint16    `gorm:"column:schema_version;not null;default:1"  json:"schema_version"`
}

// TableName returns the GORM table name. `angple_*` prefix per issue #1224 agreement.
func (AngpleSiteContent) TableName() string {
	return "angple_site_content"
}

// AngpleSiteContentUpsertRequest is the payload for PUT /content/:key.
type AngpleSiteContentUpsertRequest struct {
	Meta          *string `json:"meta,omitempty"`
	Blocks        string  `json:"blocks"         binding:"required"`
	SchemaVersion uint16  `json:"schema_version" binding:"required,oneof=1"`
}

// AngpleSiteContentResponse is the response shape returned to clients.
type AngpleSiteContentResponse struct {
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Meta          *string   `json:"meta,omitempty"`
	ContentKey    string    `json:"content_key"`
	Blocks        string    `json:"blocks"`
	ID            int64     `json:"id"`
	SiteID        int64     `json:"site_id"`
	SchemaVersion uint16    `json:"schema_version"`
}

// ToResponse converts an AngpleSiteContent to its response DTO.
func (c *AngpleSiteContent) ToResponse() *AngpleSiteContentResponse {
	return &AngpleSiteContentResponse{
		ID:            c.ID,
		SiteID:        c.SiteID,
		ContentKey:    c.ContentKey,
		SchemaVersion: c.SchemaVersion,
		Blocks:        c.Blocks,
		Meta:          c.Meta,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}
