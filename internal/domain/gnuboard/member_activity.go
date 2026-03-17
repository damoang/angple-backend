package gnuboard

import "time"

// MemberActivityFeed represents a read-side activity entry from member_activity_feed.
// This denormalized model exists to avoid repeated fan-out scans across g5_write_* tables.
type MemberActivityFeed struct {
	ID              uint64     `gorm:"column:id;primaryKey" json:"id"`
	MemberID        string     `gorm:"column:member_id" json:"member_id"`
	BoardID         string     `gorm:"column:board_id" json:"board_id"`
	WriteTable      string     `gorm:"column:write_table" json:"write_table"`
	WriteID         int        `gorm:"column:write_id" json:"write_id"`
	ParentWriteID   *int       `gorm:"column:parent_write_id" json:"parent_write_id,omitempty"`
	ActivityType    int8       `gorm:"column:activity_type" json:"activity_type"`
	IsPublic        bool       `gorm:"column:is_public" json:"is_public"`
	IsDeleted       bool       `gorm:"column:is_deleted" json:"is_deleted"`
	Title           string     `gorm:"column:title" json:"title"`
	ContentPreview  string     `gorm:"column:content_preview" json:"content_preview"`
	ParentTitle     string     `gorm:"column:parent_title" json:"parent_title,omitempty"`
	AuthorName      string     `gorm:"column:author_name" json:"author_name"`
	WrOption        string     `gorm:"column:wr_option" json:"wr_option"`
	ViewCount       int        `gorm:"column:view_count" json:"view_count"`
	LikeCount       int        `gorm:"column:like_count" json:"like_count"`
	DislikeCount    int        `gorm:"column:dislike_count" json:"dislike_count"`
	CommentCount    int        `gorm:"column:comment_count" json:"comment_count"`
	HasFile         bool       `gorm:"column:has_file" json:"has_file"`
	SourceCreatedAt time.Time  `gorm:"column:source_created_at" json:"source_created_at"`
	SourceUpdatedAt *time.Time `gorm:"column:source_updated_at" json:"source_updated_at,omitempty"`
}

func (MemberActivityFeed) TableName() string {
	return "member_activity_feed"
}

type MemberActivityStatsRow struct {
	MemberID           string `gorm:"column:member_id" json:"member_id"`
	BoardID            string `gorm:"column:board_id" json:"board_id"`
	PostCount          int64  `gorm:"column:post_count" json:"post_count"`
	CommentCount       int64  `gorm:"column:comment_count" json:"comment_count"`
	PublicPostCount    int64  `gorm:"column:public_post_count" json:"public_post_count"`
	PublicCommentCount int64  `gorm:"column:public_comment_count" json:"public_comment_count"`
}

func (MemberActivityStatsRow) TableName() string {
	return "member_activity_stats"
}
