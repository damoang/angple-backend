package domain

import (
	"time"
)

// Post domain model
// Uses Gnuboard DB structure (g5_write_*) but with standard Go naming
type Post struct {
	ID           int       `gorm:"column:wr_id;primaryKey" json:"id"`
	Title        string    `gorm:"column:wr_subject" json:"title"`
	Content      string    `gorm:"column:wr_content" json:"content"`
	Category     string    `gorm:"column:ca_name" json:"category"`
	Author       string    `gorm:"column:wr_name" json:"author"`
	AuthorID     string    `gorm:"column:mb_id" json:"author_id"`
	Email        string    `gorm:"column:wr_email" json:"-"`
	Password     string    `gorm:"column:wr_password" json:"-"`
	Homepage     string    `gorm:"column:wr_homepage" json:"-"`
	IP           string    `gorm:"column:wr_ip" json:"-"`
	Views        int       `gorm:"column:wr_hit" json:"views"`
	Likes        int       `gorm:"column:wr_good" json:"likes"`
	Dislikes     int       `gorm:"column:wr_nogood" json:"dislikes"`
	CommentCount int       `gorm:"column:wr_comment" json:"comments_count"`
	ParentID     int       `gorm:"column:wr_parent" json:"parent_id"`
	IsComment    int       `gorm:"column:wr_is_comment" json:"is_comment"`
	Num          int       `gorm:"column:wr_num" json:"-"`
	Reply        string    `gorm:"column:wr_reply" json:"-"`
	CommentReply string    `gorm:"column:wr_comment_reply" json:"-"`
	HasFile      int       `gorm:"column:wr_file" json:"has_file"`
	Link1        string    `gorm:"column:wr_link1" json:"link1,omitempty"`
	Link2        string    `gorm:"column:wr_link2" json:"link2,omitempty"`
	Link1Hit     int       `gorm:"column:wr_link1_hit" json:"-"`
	Link2Hit     int       `gorm:"column:wr_link2_hit" json:"-"`
	SEOTitle     string    `gorm:"column:wr_seo_title" json:"seo_title"`
	Option       string    `gorm:"column:wr_option" json:"-"`
	CreatedAt    time.Time `gorm:"column:wr_datetime" json:"created_at"`
	LastUpdated  string    `gorm:"column:wr_last" json:"last_updated"`
	FacebookUser string    `gorm:"column:wr_facebook_user" json:"-"`
	TwitterUser  string    `gorm:"column:wr_twitter_user" json:"-"`
	Extra1       string    `gorm:"column:wr_1" json:"-"`
	Extra2       string    `gorm:"column:wr_2" json:"-"`
	Extra3       string    `gorm:"column:wr_3" json:"-"`
	Extra4       string    `gorm:"column:wr_4" json:"-"`
	Extra5       string    `gorm:"column:wr_5" json:"-"`
	Extra6       string    `gorm:"column:wr_6" json:"-"`
	Extra7       string    `gorm:"column:wr_7" json:"-"`
	Extra8       string    `gorm:"column:wr_8" json:"-"`
	Extra9       string    `gorm:"column:wr_9" json:"-"`
	Extra10      string    `gorm:"column:wr_10" json:"-"`
}

func (Post) TableName() string {
	return "g5_write_free"
}

type PostResponse struct {
	ID            int       `json:"id"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Category      string    `json:"category,omitempty"`
	Author        string    `json:"author"`
	AuthorID      string    `json:"author_id"`
	Views         int       `json:"views"`
	Likes         int       `json:"likes"`
	CommentsCount int       `json:"comments_count"`
	CreatedAt     time.Time `json:"created_at"`
	HasFile       bool      `json:"has_file"`
}

func (p *Post) ToResponse() *PostResponse {
	return &PostResponse{
		ID:            p.ID,
		Title:         p.Title,
		Content:       p.Content,
		Category:      p.Category,
		Author:        p.Author,
		AuthorID:      p.AuthorID,
		Views:         p.Views,
		Likes:         p.Likes,
		CommentsCount: p.CommentCount,
		CreatedAt:     p.CreatedAt,
		HasFile:       p.HasFile > 0,
	}
}

type CreatePostRequest struct {
	Title    string `json:"title" validate:"required,min=1,max=200"`
	Content  string `json:"content" validate:"required,min=1"`
	Category string `json:"category,omitempty"`
	Author   string `json:"author" validate:"required,min=1,max=50"`
	Password string `json:"password,omitempty"`
}

type UpdatePostRequest struct {
	Title    string `json:"title" validate:"omitempty,min=1,max=200"`
	Content  string `json:"content" validate:"omitempty,min=1"`
	Category string `json:"category,omitempty"`
}
