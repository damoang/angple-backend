package domain

import "time"

// BoardGood represents the g5_board_good table
type BoardGood struct {
	BgID       int       `gorm:"column:bg_id;primaryKey;autoIncrement" json:"bg_id"`
	BoTable    string    `gorm:"column:bo_table" json:"bo_table"`
	WrID       int       `gorm:"column:wr_id" json:"wr_id"`
	MbID       string    `gorm:"column:mb_id" json:"mb_id"`
	BgFlag     string    `gorm:"column:bg_flag" json:"bg_flag"` // "good" or "nogood"
	BgDatetime time.Time `gorm:"column:bg_datetime" json:"bg_datetime"`
	BgIP       string    `gorm:"column:bg_ip" json:"bg_ip"`
}

// TableName returns the table name for GORM
func (BoardGood) TableName() string {
	return "g5_board_good"
}

// LikeResponse is the frontend-compatible response DTO for like/dislike toggle actions
type LikeResponse struct {
	Likes        int  `json:"likes"`
	Dislikes     int  `json:"dislikes"`
	UserLiked    bool `json:"user_liked"`
	UserDisliked bool `json:"user_disliked"`
}

// RecommendResponse is the response DTO for recommend actions
type RecommendResponse struct {
	RecommendCount  int  `json:"recommend_count"`
	UserRecommended bool `json:"user_recommended"`
}

// DownvoteResponse is the response DTO for downvote actions
type DownvoteResponse struct {
	DownvoteCount int  `json:"downvote_count"`
	UserDownvoted bool `json:"user_downvoted"`
}

// LikerInfo represents a user who liked a post
type LikerInfo struct {
	MbID    string `json:"mb_id"`
	MbName  string `json:"mb_name"`
	MbNick  string `json:"mb_nick"`            // 닉네임
	BgIP    string `json:"bg_ip,omitempty"`    // 마스킹된 IP (로그인 사용자만)
	LikedAt string `json:"liked_at"`
}

// LikersResponse is the response DTO for likers list
type LikersResponse struct {
	Likers []LikerInfo `json:"likers"`
	Total  int         `json:"total"`
}
