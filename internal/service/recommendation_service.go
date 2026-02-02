package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"

	"gorm.io/gorm"
)

// RecommendationService handles AI-powered content recommendation
type RecommendationService struct {
	recRepo *repository.RecommendationRepository
	db      *gorm.DB
}

// NewRecommendationService creates a new RecommendationService
func NewRecommendationService(
	recRepo *repository.RecommendationRepository,
	db *gorm.DB,
) *RecommendationService {
	return &RecommendationService{
		recRepo: recRepo,
		db:      db,
	}
}

// actionScores defines interest weight per action type
var actionScores = map[string]float64{
	"view":    1.0,
	"like":    3.0,
	"comment": 2.0,
	"scrap":   4.0,
	"search":  2.5,
}

// TrackActivity records user behavior and updates interests
func (s *RecommendationService) TrackActivity(ctx context.Context, userID, actionType, targetType, targetID, boardID, metadata string) error {
	activity := &domain.UserActivity{
		UserID:     userID,
		ActionType: actionType,
		TargetType: targetType,
		TargetID:   targetID,
		BoardID:    boardID,
		Metadata:   metadata,
	}

	if err := s.recRepo.TrackActivity(ctx, activity); err != nil {
		return err
	}

	// Update user interests based on post topics
	if targetType == "post" {
		scoreDelta := actionScores[actionType]
		if scoreDelta == 0 {
			scoreDelta = 1.0
		}

		topics, _ := s.recRepo.GetPostTopics(ctx, targetID)
		for _, t := range topics {
			_ = s.recRepo.UpsertUserInterest(ctx, userID, t.Topic, scoreDelta*t.Score)
		}
	}

	return nil
}

// ExtractAndSaveTopics extracts keywords from post content and saves as topics
func (s *RecommendationService) ExtractAndSaveTopics(ctx context.Context, boardID, postID, title, content string) error {
	keywords := extractKeywords(title, content)
	if len(keywords) == 0 {
		return nil
	}

	topics := make([]domain.PostTopic, 0, len(keywords))
	for keyword, score := range keywords {
		topics = append(topics, domain.PostTopic{
			BoardID: boardID,
			PostID:  postID,
			Topic:   keyword,
			Score:   score,
		})
	}

	return s.recRepo.UpsertPostTopics(ctx, topics)
}

// GetPersonalizedFeed returns personalized post recommendations for a user
func (s *RecommendationService) GetPersonalizedFeed(ctx context.Context, userID string, limit int) (*domain.PersonalizedFeedResponse, error) {
	// 1. Get user interests
	interests, _ := s.recRepo.GetUserInterests(ctx, userID, 20)

	// 2. Get recently viewed posts to exclude
	viewedIDs, _ := s.recRepo.GetUserViewedPostIDs(ctx, userID, time.Now().AddDate(0, 0, -7), 200)

	var recommended []domain.RecommendedPost

	// 3. Topic-based recommendations
	if len(interests) > 0 {
		topicNames := make([]string, len(interests))
		for i, interest := range interests {
			topicNames[i] = interest.Topic
		}

		matches, _ := s.recRepo.FindPostsByTopics(ctx, topicNames, viewedIDs, limit)
		for _, m := range matches {
			recommended = append(recommended, domain.RecommendedPost{
				PostID:  m.PostID,
				BoardID: m.BoardID,
				Score:   m.Score,
				Reason:  "topic_match",
			})
		}
	}

	// 4. Fill with trending/popular if not enough
	if len(recommended) < limit {
		remaining := limit - len(recommended)
		popular, _ := s.recRepo.GetPopularPostIDs(ctx, time.Now().Add(-24*time.Hour), remaining*2)
		for _, p := range popular {
			if len(recommended) >= limit {
				break
			}
			// Skip already recommended or viewed
			if containsPostID(recommended, p.PostID) || containsStr(viewedIDs, p.PostID) {
				continue
			}
			recommended = append(recommended, domain.RecommendedPost{
				PostID:  p.PostID,
				BoardID: p.BoardID,
				Score:   float64(p.Score),
				Reason:  "popular",
			})
		}
	}

	// 5. Enrich with post details
	s.enrichPosts(ctx, recommended)

	// 6. Sort by score descending
	sort.Slice(recommended, func(i, j int) bool {
		return recommended[i].Score > recommended[j].Score
	})

	if len(recommended) > limit {
		recommended = recommended[:limit]
	}

	// Build interest topic names for response
	userTopics := make([]string, 0, len(interests))
	for _, interest := range interests {
		userTopics = append(userTopics, interest.Topic)
	}

	return &domain.PersonalizedFeedResponse{
		Posts:  recommended,
		Topics: userTopics,
	}, nil
}

// GetTrendingTopics returns trending topics for a given period
func (s *RecommendationService) GetTrendingTopics(ctx context.Context, period string, limit int) ([]domain.TrendingTopic, error) {
	return s.recRepo.GetTrendingTopics(ctx, period, limit)
}

// RefreshTrending recalculates trending topics
func (s *RecommendationService) RefreshTrending(ctx context.Context) error {
	periods := map[string]time.Duration{
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}

	for period, dur := range periods {
		since := time.Now().Add(-dur)
		if err := s.recRepo.RefreshTrendingTopics(ctx, period, since); err != nil {
			return fmt.Errorf("트렌딩 갱신 실패 (%s): %w", period, err)
		}
	}
	return nil
}

// GetUserInterests returns a user's interest topics
func (s *RecommendationService) GetUserInterests(ctx context.Context, userID string, limit int) ([]domain.UserInterest, error) {
	return s.recRepo.GetUserInterests(ctx, userID, limit)
}

// enrichPosts fills in post details (title, author, views, etc.) from the write tables
func (s *RecommendationService) enrichPosts(ctx context.Context, posts []domain.RecommendedPost) {
	for i := range posts {
		if posts[i].BoardID == "" || posts[i].PostID == "" {
			continue
		}
		tableName := fmt.Sprintf("g5_write_%s", posts[i].BoardID)
		var result struct {
			Title     string    `gorm:"column:wr_subject"`
			Author    string    `gorm:"column:wr_name"`
			Views     int       `gorm:"column:wr_hit"`
			Likes     int       `gorm:"column:wr_good"`
			Comments  int       `gorm:"column:wr_comment"`
			CreatedAt time.Time `gorm:"column:wr_datetime"`
		}
		err := s.db.WithContext(ctx).
			Table(tableName).
			Select("wr_subject, wr_name, wr_hit, wr_good, wr_comment, wr_datetime").
			Where("wr_id = ? AND wr_is_comment = 0", posts[i].PostID).
			First(&result).Error
		if err == nil {
			posts[i].Title = result.Title
			posts[i].Author = result.Author
			posts[i].Views = result.Views
			posts[i].Likes = result.Likes
			posts[i].Comments = result.Comments
			posts[i].CreatedAt = result.CreatedAt.Format(time.RFC3339)
		}

		// Attach topics
		topics, _ := s.recRepo.GetPostTopics(ctx, posts[i].PostID)
		for _, t := range topics {
			posts[i].Topics = append(posts[i].Topics, t.Topic)
		}
	}
}

// extractKeywords extracts significant keywords from title and content
// Simple TF-based extraction; can be replaced with external NLP service
func extractKeywords(title, content string) map[string]float64 {
	// Strip HTML tags
	htmlRe := regexp.MustCompile(`<[^>]*>`)
	content = htmlRe.ReplaceAllString(content, " ")

	// Combine title (weighted higher) and content
	text := strings.ToLower(title + " " + title + " " + title + " " + content)

	// Split into words (Korean + English)
	wordRe := regexp.MustCompile(`[\p{Hangul}]{2,}|[a-zA-Z]{3,}`)
	words := wordRe.FindAllString(text, -1)

	// Count frequencies
	freq := make(map[string]int)
	for _, w := range words {
		if isStopWord(w) {
			continue
		}
		freq[w]++
	}

	// Normalize to 0-1 scores, keep top 10
	if len(freq) == 0 {
		return nil
	}

	maxFreq := 0
	for _, c := range freq {
		if c > maxFreq {
			maxFreq = c
		}
	}

	type kv struct {
		key   string
		score float64
	}
	var sorted []kv
	for k, c := range freq {
		sorted = append(sorted, kv{k, float64(c) / float64(maxFreq)})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	result := make(map[string]float64)
	for i, s := range sorted {
		if i >= 10 {
			break
		}
		result[s.key] = s.score
	}
	return result
}

// isStopWord filters common Korean/English stop words
func isStopWord(w string) bool {
	stopWords := map[string]bool{
		// Korean
		"그리고": true, "하지만": true, "그래서": true, "때문에": true,
		"그런데": true, "이것": true, "저것": true, "그것": true,
		"있는": true, "없는": true, "하는": true, "되는": true,
		"있다": true, "없다": true, "하다": true, "되다": true,
		"것이": true, "수가": true, "것을": true, "것은": true,
		"입니다": true, "합니다": true, "습니다": true, "니다": true,
		// English
		"the": true, "and": true, "for": true, "are": true,
		"but": true, "not": true, "you": true, "all": true,
		"can": true, "had": true, "her": true, "was": true,
		"one": true, "our": true, "out": true, "has": true,
		"have": true, "this": true, "that": true, "with": true,
		"from": true, "they": true, "been": true, "said": true,
		"each": true, "which": true, "their": true, "will": true,
	}
	return stopWords[w]
}

func containsPostID(posts []domain.RecommendedPost, id string) bool {
	for _, p := range posts {
		if p.PostID == id {
			return true
		}
	}
	return false
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
