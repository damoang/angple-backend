package domain

import "time"

// AnglePostRating represents a member's star rating (1~5) for a post.
// 회원당 1표(재투표=UPDATE)를 복합 PK(bo_table, wr_id, mb_id)로 보장한다.
// features.rating 토글이 켜진 게시판(앙티티 등)에서만 사용된다.
type AnglePostRating struct {
	BoTable   string    `gorm:"column:bo_table;type:varchar(20);primaryKey" json:"bo_table"`
	WrID      int       `gorm:"column:wr_id;primaryKey" json:"wr_id"`
	MbID      string    `gorm:"column:mb_id;type:varchar(20);primaryKey" json:"mb_id"`
	Rating    int       `gorm:"column:rating;type:tinyint;not null" json:"rating"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for AnglePostRating.
func (AnglePostRating) TableName() string { return "angple_post_ratings" }
