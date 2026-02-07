package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/damoang/angple-backend/pkg/cache"
	"gorm.io/gorm"
)

// MemberProfileService business logic for member profiles
type MemberProfileService interface {
	GetProfile(userID string) (*domain.MemberProfileResponse, error)
	GetRecentPosts(userID string, limit int) ([]*domain.MemberPostSummary, error)
	GetRecentComments(userID string, limit int) ([]*domain.MemberCommentSummary, error)
	GetPointHistory(userID string, limit int) ([]*domain.PointHistory, error)
	InvalidateProfileCache(userID string) error
}

type memberProfileService struct {
	memberRepo repository.MemberRepository
	pointRepo  repository.PointRepository
	db         *gorm.DB
	cache      cache.Service
}

// NewMemberProfileService creates a new MemberProfileService
func NewMemberProfileService(memberRepo repository.MemberRepository, pointRepo repository.PointRepository, db *gorm.DB, cacheService cache.Service) MemberProfileService {
	return &memberProfileService{
		memberRepo: memberRepo,
		pointRepo:  pointRepo,
		db:         db,
		cache:      cacheService,
	}
}

// GetProfile returns a member's public profile
func (s *memberProfileService) GetProfile(userID string) (*domain.MemberProfileResponse, error) {
	ctx := context.Background()

	// Try cache first
	if s.cache != nil && s.cache.IsAvailable() {
		cached, err := s.cache.GetUser(ctx, userID)
		if err == nil {
			var profile domain.MemberProfileResponse
			if json.Unmarshal(cached, &profile) == nil {
				return &profile, nil
			}
		}
	}

	member, err := s.memberRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	if member.LeaveDate != "" {
		return nil, fmt.Errorf("탈퇴한 회원입니다")
	}

	profile := member.ToProfileResponse()

	// Cache the profile
	if s.cache != nil {
		if err := s.cache.SetUser(ctx, userID, profile); err != nil {
			log.Printf("cache warning: failed to set user: %v", err)
		}
	}

	return profile, nil
}

// InvalidateProfileCache invalidates the cached profile for a user
func (s *memberProfileService) InvalidateProfileCache(userID string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.InvalidateUser(context.Background(), userID)
}

// GetRecentPosts returns a member's recent posts across all boards
func (s *memberProfileService) GetRecentPosts(userID string, limit int) ([]*domain.MemberPostSummary, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	boardIDs, err := s.getAllBoardIDs()
	if err != nil {
		return nil, err
	}
	if len(boardIDs) == 0 {
		return []*domain.MemberPostSummary{}, nil
	}

	unions := make([]string, 0, len(boardIDs))
	args := make([]interface{}, 0, len(boardIDs))
	for _, bid := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", bid)
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_subject, wr_datetime, wr_comment, wr_good, wr_hit, '%s' as bo_table FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 ORDER BY wr_id DESC LIMIT %d)",
			bid, tableName, limit,
		))
		args = append(args, userID)
	}

	query := strings.Join(unions, " UNION ALL ") + fmt.Sprintf(" ORDER BY wr_datetime DESC LIMIT %d", limit)

	type rawPost struct {
		WrID       int    `gorm:"column:wr_id"`
		WrSubject  string `gorm:"column:wr_subject"`
		WrDatetime string `gorm:"column:wr_datetime"`
		WrComment  int    `gorm:"column:wr_comment"`
		WrGood     int    `gorm:"column:wr_good"`
		WrHit      int    `gorm:"column:wr_hit"`
		BoTable    string `gorm:"column:bo_table"`
	}

	var results []rawPost
	if err := s.db.Raw(query, args...).Scan(&results).Error; err != nil {
		return nil, err
	}

	posts := make([]*domain.MemberPostSummary, len(results))
	for i, r := range results {
		posts[i] = &domain.MemberPostSummary{
			ID:        r.WrID,
			BoardID:   r.BoTable,
			Title:     r.WrSubject,
			CreatedAt: r.WrDatetime,
			Comments:  r.WrComment,
			Likes:     r.WrGood,
			Views:     r.WrHit,
		}
	}
	return posts, nil
}

// GetRecentComments returns a member's recent comments across all boards
func (s *memberProfileService) GetRecentComments(userID string, limit int) ([]*domain.MemberCommentSummary, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	boardIDs, err := s.getAllBoardIDs()
	if err != nil {
		return nil, err
	}
	if len(boardIDs) == 0 {
		return []*domain.MemberCommentSummary{}, nil
	}

	unions := make([]string, 0, len(boardIDs))
	args := make([]interface{}, 0, len(boardIDs))
	for _, bid := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", bid)
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_content, wr_datetime, wr_parent, '%s' as bo_table FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 ORDER BY wr_id DESC LIMIT %d)",
			bid, tableName, limit,
		))
		args = append(args, userID)
	}

	query := strings.Join(unions, " UNION ALL ") + fmt.Sprintf(" ORDER BY wr_datetime DESC LIMIT %d", limit)

	type rawComment struct {
		WrID       int    `gorm:"column:wr_id"`
		WrContent  string `gorm:"column:wr_content"`
		WrDatetime string `gorm:"column:wr_datetime"`
		WrParent   int    `gorm:"column:wr_parent"`
		BoTable    string `gorm:"column:bo_table"`
	}

	var results []rawComment
	if err := s.db.Raw(query, args...).Scan(&results).Error; err != nil {
		return nil, err
	}

	comments := make([]*domain.MemberCommentSummary, len(results))
	for i, r := range results {
		content := r.WrContent
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		comments[i] = &domain.MemberCommentSummary{
			ID:        r.WrID,
			BoardID:   r.BoTable,
			Content:   content,
			CreatedAt: r.WrDatetime,
			PostID:    r.WrParent,
		}
	}
	return comments, nil
}

// GetPointHistory returns a member's point transaction history
func (s *memberProfileService) GetPointHistory(userID string, limit int) ([]*domain.PointHistory, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	points, err := s.pointRepo.FindByMemberID(userID, limit)
	if err != nil {
		return nil, err
	}

	history := make([]*domain.PointHistory, len(points))
	for i, p := range points {
		relID, _ := strconv.Atoi(p.RelID) //nolint:errcheck // best-effort conversion, 0 on failure is acceptable
		history[i] = &domain.PointHistory{
			ID:        p.ID,
			Point:     p.Point,
			Content:   p.Content,
			CreatedAt: p.Datetime,
			RelTable:  p.RelTable,
			RelID:     relID,
			RelAction: p.RelAction,
		}
	}
	return history, nil
}

// getAllBoardIDs returns all board IDs from g5_board
func (s *memberProfileService) getAllBoardIDs() ([]string, error) {
	var boardIDs []string
	err := s.db.Table("g5_board").Pluck("bo_table", &boardIDs).Error
	if err != nil {
		return nil, err
	}
	return boardIDs, nil
}
