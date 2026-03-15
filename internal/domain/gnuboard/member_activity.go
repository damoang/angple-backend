package gnuboard

import (
	"strings"
	"time"
)

// MemberActivityFeed represents a read-side activity entry from member_activity_feed table
type MemberActivityFeed struct {
	ID              uint64    `gorm:"column:id;primaryKey" json:"id"`
	MemberID        string    `gorm:"column:member_id" json:"member_id"`
	BoardID         string    `gorm:"column:board_id" json:"board_id"`
	WriteTable      string    `gorm:"column:write_table" json:"write_table"`
	WriteID         int       `gorm:"column:write_id" json:"write_id"`
	ParentWriteID   *int      `gorm:"column:parent_write_id" json:"parent_write_id,omitempty"`
	ActivityType    int8      `gorm:"column:activity_type" json:"activity_type"`
	IsPublic        bool      `gorm:"column:is_public" json:"is_public"`
	IsDeleted       bool      `gorm:"column:is_deleted" json:"is_deleted"`
	Title           string    `gorm:"column:title" json:"title"`
	ContentPreview  string    `gorm:"column:content_preview" json:"content_preview"`
	ParentTitle     string    `gorm:"column:parent_title" json:"parent_title,omitempty"`
	AuthorName      string    `gorm:"column:author_name" json:"author_name"`
	WrOption        string    `gorm:"column:wr_option" json:"-"`
	SourceCreatedAt time.Time `gorm:"column:source_created_at" json:"source_created_at"`
}

// TableName returns the table name for GORM
func (MemberActivityFeed) TableName() string {
	return "member_activity_feed"
}

// ToPostResponse converts to the same format as MyPost.ToPostResponse()
func (f *MemberActivityFeed) ToPostResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":             f.WriteID,
		"title":          f.Title,
		"author":         f.AuthorName,
		"author_id":      f.MemberID,
		"board_id":       f.BoardID,
		"views":          0,
		"likes":          0,
		"dislikes":       0,
		"comments_count": 0,
		"has_file":       false,
		"is_secret":      strings.Contains(f.WrOption, "secret"),
		"created_at":     f.SourceCreatedAt.Format(time.RFC3339),
	}
}

// ToCommentResponse converts to the same format as MyCommentRow.ToCommentResponse()
func (f *MemberActivityFeed) ToCommentResponse() map[string]interface{} {
	parentID := 0
	if f.ParentWriteID != nil {
		parentID = *f.ParentWriteID
	}
	return map[string]interface{}{
		"id":         f.WriteID,
		"content":    f.ContentPreview,
		"author":     f.AuthorName,
		"author_id":  f.MemberID,
		"likes":      0,
		"dislikes":   0,
		"parent_id":  parentID,
		"post_id":    parentID,
		"post_title": f.ParentTitle,
		"board_id":   f.BoardID,
		"is_secret":  strings.Contains(f.WrOption, "secret"),
		"created_at": f.SourceCreatedAt.Format(time.RFC3339),
	}
}

// MemberActivityStatsRow represents per-board stats from member_activity_stats
type MemberActivityStatsRow struct {
	MemberID           string `gorm:"column:member_id" json:"member_id"`
	BoardID            string `gorm:"column:board_id" json:"board_id"`
	PostCount          int64  `gorm:"column:post_count" json:"post_count"`
	CommentCount       int64  `gorm:"column:comment_count" json:"comment_count"`
	PublicPostCount    int64  `gorm:"column:public_post_count" json:"public_post_count"`
	PublicCommentCount int64  `gorm:"column:public_comment_count" json:"public_comment_count"`
}

// TableName returns the table name for GORM
func (MemberActivityStatsRow) TableName() string {
	return "member_activity_stats"
}
