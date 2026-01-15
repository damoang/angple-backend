package domain

import (
	"time"
)

// Comment domain model (uses same table as Post)
// Differentiated by wr_is_comment = 1
//
//nolint:dupl // Comment와 Post는 같은 테이블을 사용하므로 구조가 유사함
type Comment struct {
	CreatedAt    time.Time `gorm:"column:wr_datetime" json:"created_at"`
	Extra2       string    `gorm:"column:wr_2" json:"-"`
	FacebookUser string    `gorm:"column:wr_facebook_user" json:"-"`
	CommentReply string    `gorm:"column:wr_comment_reply" json:"-"`
	Content      string    `gorm:"column:wr_content" json:"content"`
	Author       string    `gorm:"column:wr_name" json:"author"`
	AuthorID     string    `gorm:"column:mb_id" json:"author_id"`
	IP           string    `gorm:"column:wr_ip" json:"-"`
	SEOTitle     string    `gorm:"column:wr_seo_title" json:"-"`
	Reply        string    `gorm:"column:wr_reply" json:"-"`
	Option       string    `gorm:"column:wr_option" json:"-"`
	Link1        string    `gorm:"column:wr_link1" json:"-"`
	Link2        string    `gorm:"column:wr_link2" json:"-"`
	Email        string    `gorm:"column:wr_email" json:"-"`
	Password     string    `gorm:"column:wr_password" json:"-"`
	Homepage     string    `gorm:"column:wr_homepage" json:"-"`
	LastUpdated  string    `gorm:"column:wr_last" json:"-"`
	Category     string    `gorm:"column:ca_name" json:"-"`
	TwitterUser  string    `gorm:"column:wr_twitter_user" json:"-"`
	Title        string    `gorm:"column:wr_subject" json:"-"`
	Extra1       string    `gorm:"column:wr_1" json:"-"`
	Extra6       string    `gorm:"column:wr_6" json:"-"`
	Extra4       string    `gorm:"column:wr_4" json:"-"`
	Extra5       string    `gorm:"column:wr_5" json:"-"`
	Extra3       string    `gorm:"column:wr_3" json:"-"`
	Extra7       string    `gorm:"column:wr_7" json:"-"`
	Extra8       string    `gorm:"column:wr_8" json:"-"`
	Extra9       string    `gorm:"column:wr_9" json:"-"`
	Extra10      string    `gorm:"column:wr_10" json:"-"`
	IsComment    int       `gorm:"column:wr_is_comment" json:"-"`
	ID           int       `gorm:"column:wr_id;primaryKey" json:"id"`
	Views        int       `gorm:"column:wr_hit" json:"-"`
	Likes        int       `gorm:"column:wr_good" json:"-"`
	Dislikes     int       `gorm:"column:wr_nogood" json:"-"`
	CommentCount int       `gorm:"column:wr_comment" json:"-"`
	Num          int       `gorm:"column:wr_num" json:"-"`
	HasFile      int       `gorm:"column:wr_file" json:"-"`
	Link1Hit     int       `gorm:"column:wr_link1_hit" json:"-"`
	Link2Hit     int       `gorm:"column:wr_link2_hit" json:"-"`
	ParentID     int       `gorm:"column:wr_parent" json:"parent_id"`
}

type CommentResponse struct {
	CreatedAt time.Time `json:"created_at"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	ID        int       `json:"id"`
	ParentID  int       `json:"parent_id"`
	Depth     int       `json:"depth"` // 댓글 depth (1=일반 댓글, 2=대댓글, ...)
}

func (c *Comment) ToResponse() *CommentResponse {
	return &CommentResponse{
		ID:        c.ID,
		ParentID:  c.ParentID,
		Content:   c.Content,
		Author:    c.Author,
		AuthorID:  c.AuthorID,
		CreatedAt: c.CreatedAt,
		Depth:     c.CommentCount, // wr_comment 컬럼이 depth를 나타냄
	}
}

type CreateCommentRequest struct {
	Content string `json:"content" validate:"required,min=1"`
	Author  string `json:"author" validate:"required,min=1,max=50"`
}

type UpdateCommentRequest struct {
	Content string `json:"content" validate:"required,min=1"`
}
