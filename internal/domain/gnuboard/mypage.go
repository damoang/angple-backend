package gnuboard

import (
	"strings"
	"time"
)

// MyPost represents a post row from UNION ALL across g5_write_* tables
type MyPost struct {
	WrID       int       `gorm:"column:wr_id" json:"wr_id"`
	WrSubject  string    `gorm:"column:wr_subject" json:"wr_subject"`
	WrContent  string    `gorm:"column:wr_content" json:"wr_content"`
	WrHit      int       `gorm:"column:wr_hit" json:"wr_hit"`
	WrGood     int       `gorm:"column:wr_good" json:"wr_good"`
	WrNogood   int       `gorm:"column:wr_nogood" json:"wr_nogood"`
	WrComment  int       `gorm:"column:wr_comment" json:"wr_comment"`
	WrDatetime time.Time `gorm:"column:wr_datetime" json:"wr_datetime"`
	MbID       string    `gorm:"column:mb_id" json:"mb_id"`
	WrName     string    `gorm:"column:wr_name" json:"wr_name"`
	WrOption   string    `gorm:"column:wr_option" json:"wr_option"`
	WrFile     int       `gorm:"column:wr_file" json:"wr_file"`
	BoardID    string    `gorm:"column:board_id" json:"board_id"`
}

// ToPostResponse converts MyPost to the standard API response format
func (p *MyPost) ToPostResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":             p.WrID,
		"title":          p.WrSubject,
		"author":         p.WrName,
		"author_id":      p.MbID,
		"board_id":       p.BoardID,
		"views":          p.WrHit,
		"likes":          p.WrGood,
		"dislikes":       p.WrNogood,
		"comments_count": p.WrComment,
		"has_file":       p.WrFile > 0,
		"is_secret":      strings.Contains(p.WrOption, "secret"),
		"created_at":     p.WrDatetime.Format(time.RFC3339),
	}
}

// MyCommentRow represents a comment row from UNION ALL with parent post title
type MyCommentRow struct {
	WrID       int       `gorm:"column:wr_id" json:"wr_id"`
	WrContent  string    `gorm:"column:wr_content" json:"wr_content"`
	WrDatetime time.Time `gorm:"column:wr_datetime" json:"wr_datetime"`
	MbID       string    `gorm:"column:mb_id" json:"mb_id"`
	WrName     string    `gorm:"column:wr_name" json:"wr_name"`
	WrParent   int       `gorm:"column:wr_parent" json:"wr_parent"`
	WrGood     int       `gorm:"column:wr_good" json:"wr_good"`
	WrNogood   int       `gorm:"column:wr_nogood" json:"wr_nogood"`
	WrOption   string    `gorm:"column:wr_option" json:"wr_option"`
	PostTitle  string    `gorm:"column:post_title" json:"post_title"`
	BoardID    string    `gorm:"column:board_id" json:"board_id"`
}

// ToCommentResponse converts MyCommentRow to the standard API response format
func (c *MyCommentRow) ToCommentResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":         c.WrID,
		"content":    c.WrContent,
		"author":     c.WrName,
		"author_id":  c.MbID,
		"likes":      c.WrGood,
		"dislikes":   c.WrNogood,
		"parent_id":  c.WrParent,
		"post_id":    c.WrParent,
		"post_title": c.PostTitle,
		"board_id":   c.BoardID,
		"is_secret":  strings.Contains(c.WrOption, "secret"),
		"created_at": c.WrDatetime.Format(time.RFC3339),
	}
}

// BoardStat represents post/comment counts per board for a member
type BoardStat struct {
	BoardID      string `json:"board_id"`
	BoardName    string `json:"board_name"`
	PostCount    int64  `json:"post_count"`
	CommentCount int64  `json:"comment_count"`
}
