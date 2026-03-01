package gnuboard

import "time"

// G5Write represents the g5_write_* dynamic tables (posts and comments)
// TableName is set dynamically in repository using Table()
// Note: Only core columns are included. Extended columns (wr_1~wr_10, wr_facebook, etc.)
// are omitted to avoid errors when columns don't exist in some boards.
type G5Write struct {
	WrID           int       `gorm:"column:wr_id;primaryKey" json:"wr_id"`
	WrNum          int       `gorm:"column:wr_num" json:"wr_num"`
	WrReply        string    `gorm:"column:wr_reply" json:"wr_reply"`
	WrParent       int       `gorm:"column:wr_parent" json:"wr_parent"`
	WrIsComment    int       `gorm:"column:wr_is_comment" json:"wr_is_comment"`
	WrComment      int       `gorm:"column:wr_comment" json:"wr_comment"`
	WrCommentReply string    `gorm:"column:wr_comment_reply" json:"wr_comment_reply"`
	CaName         string    `gorm:"column:ca_name" json:"ca_name"`
	WrOption       string    `gorm:"column:wr_option" json:"wr_option"`
	WrSubject      string    `gorm:"column:wr_subject" json:"wr_subject"`
	WrContent      string    `gorm:"column:wr_content" json:"wr_content"`
	WrLink1        string    `gorm:"column:wr_link1" json:"wr_link1"`
	WrLink2        string    `gorm:"column:wr_link2" json:"wr_link2"`
	WrLink1Hit     int       `gorm:"column:wr_link1_hit" json:"wr_link1_hit"`
	WrLink2Hit     int       `gorm:"column:wr_link2_hit" json:"wr_link2_hit"`
	WrHit          int       `gorm:"column:wr_hit" json:"wr_hit"`
	WrGood         int       `gorm:"column:wr_good" json:"wr_good"`
	WrNogood       int       `gorm:"column:wr_nogood" json:"wr_nogood"`
	MbID           string    `gorm:"column:mb_id" json:"mb_id"`
	WrPassword     string    `gorm:"column:wr_password" json:"-"`
	WrName         string    `gorm:"column:wr_name" json:"wr_name"`
	WrEmail        string    `gorm:"column:wr_email" json:"wr_email"`
	WrHomepage     string    `gorm:"column:wr_homepage" json:"wr_homepage"`
	WrDatetime     time.Time `gorm:"column:wr_datetime" json:"wr_datetime"`
	WrFile         int       `gorm:"column:wr_file" json:"wr_file"`
	WrLast         string    `gorm:"column:wr_last" json:"wr_last"`
	WrIP           string    `gorm:"column:wr_ip" json:"-"`
}

// PostResponse is the API response format for posts
type PostResponse struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Content       string     `json:"content,omitempty"`
	Author        string     `json:"author"`
	AuthorID      string     `json:"author_id"`
	Category      string     `json:"category,omitempty"`
	Views         int        `json:"views"`
	Likes         int        `json:"likes"`
	Dislikes      int        `json:"dislikes"`
	CommentsCount int        `json:"comments_count"`
	HasFile       bool       `json:"has_file"`
	IsNotice      bool       `json:"is_notice"`
	Link1         string     `json:"link1,omitempty"`
	Link2         string     `json:"link2,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// parseWrLast converts DB datetime string to time.Time
// Returns nil if parsing fails or if the time equals created time (no updates)
func parseWrLast(wrLast string, createdAt time.Time) *time.Time {
	if wrLast == "" {
		return nil
	}
	// DB stores as "2006-01-02 15:04:05"
	lastTime, err := time.ParseInLocation("2006-01-02 15:04:05", wrLast, time.Local)
	if err != nil {
		return nil
	}
	// If updated_at equals created_at (within 1 second), no actual update occurred
	if lastTime.Equal(createdAt) || lastTime.Sub(createdAt).Abs() < time.Second {
		return nil
	}
	return &lastTime
}

// ToPostResponse converts G5Write to post API response format
func (w *G5Write) ToPostResponse() PostResponse {
	return PostResponse{
		ID:            w.WrID,
		Title:         w.WrSubject,
		Author:        w.WrName,
		AuthorID:      w.MbID,
		Category:      w.CaName,
		Views:         w.WrHit,
		Likes:         w.WrGood,
		Dislikes:      w.WrNogood,
		CommentsCount: w.WrComment,
		HasFile:       w.WrFile > 0,
		IsNotice:      false, // Will be set externally based on board notice list
		Link1:         w.WrLink1,
		Link2:         w.WrLink2,
		CreatedAt:     w.WrDatetime,
		UpdatedAt:     parseWrLast(w.WrLast, w.WrDatetime),
	}
}

// ToPostDetailResponse converts G5Write to detailed post API response
func (w *G5Write) ToPostDetailResponse() PostResponse {
	resp := w.ToPostResponse()
	resp.Content = w.WrContent
	return resp
}

// CommentResponse is the API response format for comments
type CommentResponse struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	ParentID  int       `json:"parent_id,omitempty"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	AuthorID  string    `json:"author_id"`
	Likes     int       `json:"likes"`
	Dislikes  int       `json:"dislikes"`
	Depth     int       `json:"depth"`
	CreatedAt time.Time `json:"created_at"`
}

// ToCommentResponse converts G5Write (comment) to API response format
func (w *G5Write) ToCommentResponse() CommentResponse {
	depth := len(w.WrCommentReply)
	return CommentResponse{
		ID:        w.WrID,
		PostID:    w.WrParent,
		ParentID:  0, // Will be determined by comment_reply parsing if needed
		Content:   w.WrContent,
		Author:    w.WrName,
		AuthorID:  w.MbID,
		Likes:     w.WrGood,
		Dislikes:  w.WrNogood,
		Depth:     depth,
		CreatedAt: w.WrDatetime,
	}
}
