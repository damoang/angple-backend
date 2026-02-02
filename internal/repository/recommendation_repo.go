package repository

import (
	"context"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecommendationRepository handles recommendation-related persistence
type RecommendationRepository struct {
	db *gorm.DB
}

// NewRecommendationRepository creates a new RecommendationRepository
func NewRecommendationRepository(db *gorm.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

// AutoMigrate creates recommendation tables
func (r *RecommendationRepository) AutoMigrate() error {
	return r.db.AutoMigrate(
		&domain.UserActivity{},
		&domain.PostTopic{},
		&domain.UserInterest{},
		&domain.TrendingTopic{},
	)
}

// ========================================
// User Activity
// ========================================

// TrackActivity records a user action
func (r *RecommendationRepository) TrackActivity(ctx context.Context, activity *domain.UserActivity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

// GetRecentActivities returns recent activities for a user
func (r *RecommendationRepository) GetRecentActivities(ctx context.Context, userID string, limit int) ([]domain.UserActivity, error) {
	var activities []domain.UserActivity
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&activities).Error
	return activities, err
}

// GetUserViewedPostIDs returns post IDs recently viewed by a user
func (r *RecommendationRepository) GetUserViewedPostIDs(ctx context.Context, userID string, since time.Time, limit int) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&domain.UserActivity{}).
		Select("DISTINCT target_id").
		Where("user_id = ? AND action_type = 'view' AND target_type = 'post' AND created_at >= ?", userID, since).
		Limit(limit).
		Pluck("target_id", &ids).Error
	return ids, err
}

// ========================================
// Post Topics
// ========================================

// UpsertPostTopics creates or updates topics for a post
func (r *RecommendationRepository) UpsertPostTopics(ctx context.Context, topics []domain.PostTopic) error {
	if len(topics) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "post_id"}, {Name: "topic"}},
			DoUpdates: clause.AssignmentColumns([]string{"score"}),
		}).
		Create(&topics).Error
}

// GetPostTopics returns topics for a post
func (r *RecommendationRepository) GetPostTopics(ctx context.Context, postID string) ([]domain.PostTopic, error) {
	var topics []domain.PostTopic
	err := r.db.WithContext(ctx).
		Where("post_id = ?", postID).
		Order("score DESC").
		Find(&topics).Error
	return topics, err
}

// FindPostsByTopics returns post IDs matching given topics, sorted by relevance
func (r *RecommendationRepository) FindPostsByTopics(ctx context.Context, topics []string, excludeIDs []string, limit int) ([]struct {
	PostID  string
	BoardID string
	Score   float64
}, error) {
	type result struct {
		PostID  string  `gorm:"column:post_id"`
		BoardID string  `gorm:"column:board_id"`
		Score   float64 `gorm:"column:total_score"`
	}
	var results []result

	query := r.db.WithContext(ctx).
		Model(&domain.PostTopic{}).
		Select("post_id, board_id, SUM(score) as total_score").
		Where("topic IN ?", topics).
		Group("post_id, board_id").
		Order("total_score DESC").
		Limit(limit)

	if len(excludeIDs) > 0 {
		query = query.Where("post_id NOT IN ?", excludeIDs)
	}

	err := query.Find(&results).Error

	out := make([]struct {
		PostID  string
		BoardID string
		Score   float64
	}, len(results))
	for i, r := range results {
		out[i].PostID = r.PostID
		out[i].BoardID = r.BoardID
		out[i].Score = r.Score
	}
	return out, err
}

// ========================================
// User Interests
// ========================================

// UpsertUserInterest creates or updates a user interest score
func (r *RecommendationRepository) UpsertUserInterest(ctx context.Context, userID, topic string, scoreDelta float64) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO user_interests (user_id, topic, score, updated_at) VALUES (?, ?, ?, NOW())
		 ON DUPLICATE KEY UPDATE score = score + ?, updated_at = NOW()`,
		userID, topic, scoreDelta, scoreDelta,
	).Error
}

// GetUserInterests returns top interests for a user
func (r *RecommendationRepository) GetUserInterests(ctx context.Context, userID string, limit int) ([]domain.UserInterest, error) {
	var interests []domain.UserInterest
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("score DESC").
		Limit(limit).
		Find(&interests).Error
	return interests, err
}

// ========================================
// Trending Topics
// ========================================

// RefreshTrendingTopics recalculates trending topics from recent post topics
func (r *RecommendationRepository) RefreshTrendingTopics(ctx context.Context, period string, since time.Time) error {
	// Delete old trending for this period
	r.db.WithContext(ctx).Where("period = ?", period).Delete(&domain.TrendingTopic{})

	// Aggregate from post_topics joined with recent posts
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO trending_topics (topic, score, post_count, period, updated_at)
		 SELECT topic, SUM(score) as score, COUNT(DISTINCT post_id) as post_count, ?, NOW()
		 FROM post_topics
		 WHERE created_at >= ?
		 GROUP BY topic
		 ORDER BY score DESC
		 LIMIT 100`,
		period, since,
	).Error
}

// GetTrendingTopics returns trending topics for a period
func (r *RecommendationRepository) GetTrendingTopics(ctx context.Context, period string, limit int) ([]domain.TrendingTopic, error) {
	var topics []domain.TrendingTopic
	err := r.db.WithContext(ctx).
		Where("period = ?", period).
		Order("score DESC").
		Limit(limit).
		Find(&topics).Error
	return topics, err
}

// GetPopularPostIDs returns most-interacted post IDs in a time window
func (r *RecommendationRepository) GetPopularPostIDs(ctx context.Context, since time.Time, limit int) ([]struct {
	PostID  string
	BoardID string
	Score   int64
}, error) {
	type result struct {
		PostID  string `gorm:"column:target_id"`
		BoardID string `gorm:"column:board_id"`
		Score   int64  `gorm:"column:activity_count"`
	}
	var results []result

	err := r.db.WithContext(ctx).
		Model(&domain.UserActivity{}).
		Select("target_id, board_id, COUNT(*) as activity_count").
		Where("target_type = 'post' AND created_at >= ?", since).
		Group("target_id, board_id").
		Order("activity_count DESC").
		Limit(limit).
		Find(&results).Error

	out := make([]struct {
		PostID  string
		BoardID string
		Score   int64
	}, len(results))
	for i, r := range results {
		out[i].PostID = r.PostID
		out[i].BoardID = r.BoardID
		out[i].Score = r.Score
	}
	return out, err
}
