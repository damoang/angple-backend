package domain

import "time"

// UserActivity tracks user behavior for recommendation
type UserActivity struct {
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index" json:"created_at"`

	UserID     string `gorm:"column:user_id;index:idx_user_activity" json:"user_id"`
	ActionType string `gorm:"column:action_type;index:idx_user_activity" json:"action_type"` // view, like, comment, scrap, search
	TargetType string `gorm:"column:target_type" json:"target_type"`                         // post, board, comment
	TargetID   string `gorm:"column:target_id" json:"target_id"`
	BoardID    string `gorm:"column:board_id" json:"board_id"`
	Metadata   string `gorm:"column:metadata;type:json" json:"metadata,omitempty"` // search keywords, etc.

	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (UserActivity) TableName() string {
	return "user_activities"
}

// PostTopic represents extracted topics/keywords for a post
type PostTopic struct {
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	BoardID string  `gorm:"column:board_id" json:"board_id"`
	PostID  string  `gorm:"column:post_id;index:idx_post_topic" json:"post_id"`
	Topic   string  `gorm:"column:topic;index:idx_topic" json:"topic"`
	Score   float64 `gorm:"column:score;type:decimal(5,3);default:1.0" json:"score"` // relevance score 0-1

	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (PostTopic) TableName() string {
	return "post_topics"
}

// UserInterest represents aggregated user interest per topic
type UserInterest struct {
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	UserID string  `gorm:"column:user_id;uniqueIndex:idx_user_topic" json:"user_id"`
	Topic  string  `gorm:"column:topic;uniqueIndex:idx_user_topic" json:"topic"`
	Score  float64 `gorm:"column:score;type:decimal(8,3);default:0" json:"score"` // accumulated interest score

	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (UserInterest) TableName() string {
	return "user_interests"
}

// TrendingTopic represents a trending topic with time-windowed scores
type TrendingTopic struct {
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	Topic     string  `gorm:"column:topic;uniqueIndex" json:"topic"`
	Score     float64 `gorm:"column:score;type:decimal(10,3)" json:"score"`
	PostCount int     `gorm:"column:post_count" json:"post_count"`
	Period    string  `gorm:"column:period" json:"period"` // 24h, 7d, 30d

	ID int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
}

func (TrendingTopic) TableName() string {
	return "trending_topics"
}

// RecommendedPost is the response item for personalized feed
type RecommendedPost struct {
	PostID    string   `json:"post_id"`
	BoardID   string   `json:"board_id"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Score     float64  `json:"score"`
	Reason    string   `json:"reason"` // topic_match, trending, popular, collaborative
	Topics    []string `json:"topics,omitempty"`
	Views     int      `json:"views"`
	Likes     int      `json:"likes"`
	Comments  int      `json:"comments"`
	CreatedAt string   `json:"created_at"`
}

// PersonalizedFeedResponse wraps the feed with metadata
type PersonalizedFeedResponse struct {
	Posts  []RecommendedPost `json:"posts"`
	Topics []string          `json:"user_interests,omitempty"`
}
