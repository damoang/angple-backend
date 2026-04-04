package v2

import "time"

// WikiBacklink 위키 문서 간 [[링크]] 역참조 관계
type WikiBacklink struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	SourcePostID uint64    `gorm:"column:source_post_id;index:idx_wiki_bl_source;uniqueIndex:idx_wiki_bl_unique" json:"source_post_id"`
	TargetPostID uint64    `gorm:"column:target_post_id;index:idx_wiki_bl_target;uniqueIndex:idx_wiki_bl_unique" json:"target_post_id"`
	LinkText     string    `gorm:"column:link_text;type:varchar(255)" json:"link_text"`
	IsBroken     bool      `gorm:"column:is_broken;default:false" json:"is_broken"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (WikiBacklink) TableName() string { return "wiki_backlinks" }
