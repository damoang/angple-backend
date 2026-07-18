package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/damoang/angple-backend/internal/repository"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// 별점 투표 최소 등급 — 공감(좋아요)과 동일 게이트 (mb_level 3 = 앙님 이상).
const postRatingMinLevel = 3

// Post rating errors — handler 가 HTTP 상태코드로 매핑한다.
var (
	// ErrRatingOutOfRange 별점 범위(1~5) 밖 → 400
	ErrRatingOutOfRange = errors.New("별점은 1~5 사이여야 합니다")
	// ErrRatingDisabled features.rating 미설정 보드 → 403
	ErrRatingDisabled = errors.New("이 게시판에서는 별점 기능을 사용할 수 없습니다")
	// ErrRatingLevelTooLow 등급 미달 → 403
	ErrRatingLevelTooLow = errors.New("앙님 등급부터 별점을 남길 수 있습니다")
)

// RatingSummary is the aggregate response payload: {"avg", "count", "my"}.
type RatingSummary struct {
	Avg   float64 `json:"avg"`
	Count int64   `json:"count"`
	My    int     `json:"my"`
}

// ratingExtendedSettings mirrors the features section of
// v2_board_extended_settings.settings JSON (admin 확장 설정과 동일 소스,
// NariyaFeatures.Rating 과 같은 키: $.features.rating).
type ratingExtendedSettings struct {
	Features struct {
		Rating bool `json:"rating"`
	} `json:"features"`
}

// PostRatingService implements the post star rating feature (★1~5, 회원당 1표).
// features.rating 토글이 켜진 게시판에서만 투표를 허용한다.
type PostRatingService struct {
	repo                 repository.PostRatingRepository
	extendedSettingsRepo v2repo.BoardExtendedSettingsRepository
}

// NewPostRatingService creates a new PostRatingService.
func NewPostRatingService(repo repository.PostRatingRepository, extendedSettingsRepo v2repo.BoardExtendedSettingsRepository) *PostRatingService {
	return &PostRatingService{repo: repo, extendedSettingsRepo: extendedSettingsRepo}
}

// Enabled reports whether the board has features.rating turned on
// (v2_board_extended_settings.settings → $.features.rating == true).
// 설정 누락·파싱 오류 시 비활성으로 간주한다(fail-closed).
func (s *PostRatingService) Enabled(boardSlug string) bool {
	settings, err := s.extendedSettingsRepo.FindByBoardSlug(boardSlug)
	if err != nil || settings == nil || settings.Settings == "" {
		return false
	}
	var parsed ratingExtendedSettings
	if err := json.Unmarshal([]byte(settings.Settings), &parsed); err != nil {
		return false
	}
	return parsed.Features.Rating
}

// Rate records (or updates) the member's star rating and returns the new aggregate.
func (s *PostRatingService) Rate(boardSlug string, wrID int, mbID string, mbLevel int, rating int) (*RatingSummary, error) {
	if rating < 1 || rating > 5 {
		return nil, ErrRatingOutOfRange
	}
	if !s.Enabled(boardSlug) {
		return nil, ErrRatingDisabled
	}
	if mbLevel < postRatingMinLevel {
		return nil, ErrRatingLevelTooLow
	}
	if err := s.repo.Upsert(boardSlug, wrID, mbID, rating); err != nil {
		return nil, fmt.Errorf("failed to upsert rating for %s/%d: %w", boardSlug, wrID, err)
	}
	return s.Summary(boardSlug, wrID, mbID)
}

// Summary returns the aggregate rating for a post (my=0 for guests / non-voters).
// avg 는 소수 1자리 반올림.
func (s *PostRatingService) Summary(boardSlug string, wrID int, mbID string) (*RatingSummary, error) {
	avg, count, err := s.repo.Aggregate(boardSlug, wrID)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate ratings for %s/%d: %w", boardSlug, wrID, err)
	}
	my := 0
	if mbID != "" {
		if my, err = s.repo.FindMyRating(boardSlug, wrID, mbID); err != nil {
			return nil, fmt.Errorf("failed to load my rating for %s/%d: %w", boardSlug, wrID, err)
		}
	}
	return &RatingSummary{
		Avg:   math.Round(avg*10) / 10,
		Count: count,
		My:    my,
	}, nil
}
