package v2

import "time"

// V2ContentRevision stores post revision history
type V2ContentRevision struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PostID       uint64    `gorm:"column:post_id;index" json:"post_id"`
	Version      uint      `gorm:"column:version" json:"version"`
	ChangeType   string    `gorm:"column:change_type;type:varchar(20)" json:"change_type"` // create, update, soft_delete, restore
	Title        string    `gorm:"column:title;type:varchar(255)" json:"title"`
	Content      string    `gorm:"column:content;type:mediumtext" json:"content"`
	EditedBy     uint64    `gorm:"column:edited_by" json:"edited_by"`
	EditedByName string    `gorm:"column:edited_by_name;type:varchar(100)" json:"edited_by_name"`
	EditedAt     time.Time `gorm:"column:edited_at;autoCreateTime" json:"edited_at"`
}

func (V2ContentRevision) TableName() string { return "v2_content_revisions" }
