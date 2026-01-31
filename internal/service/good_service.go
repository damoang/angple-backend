package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// GoodService defines the interface for recommend/downvote business logic
type GoodService interface {
	RecommendPost(boardID string, wrID int, userID string) (*domain.RecommendResponse, error)
	CancelRecommendPost(boardID string, wrID int, userID string) (*domain.RecommendResponse, error)
	DownvotePost(boardID string, wrID int, userID string) (*domain.DownvoteResponse, error)
	CancelDownvotePost(boardID string, wrID int, userID string) (*domain.DownvoteResponse, error)
	RecommendComment(boardID string, wrID int, userID string) (*domain.RecommendResponse, error)
	CancelRecommendComment(boardID string, wrID int, userID string) (*domain.RecommendResponse, error)
	// Frontend-compatible toggle methods
	ToggleLike(boardID string, wrID int, userID string) (*domain.LikeResponse, error)
	ToggleDislike(boardID string, wrID int, userID string) (*domain.LikeResponse, error)
	GetLikeStatus(boardID string, wrID int, userID string) (*domain.LikeResponse, error)
}

type goodService struct {
	goodRepo repository.GoodRepository
}

// NewGoodService creates a new GoodService
func NewGoodService(goodRepo repository.GoodRepository) GoodService {
	return &goodService{
		goodRepo: goodRepo,
	}
}

// checkAuthorAndReject checks if the user is the author and returns error if so
func (s *goodService) checkAuthorAndReject(boardID string, wrID int, userID string) error {
	authorID, err := s.goodRepo.GetWriteAuthorID(boardID, wrID)
	if err != nil {
		return common.ErrPostNotFound
	}
	if authorID == userID {
		return common.ErrSelfRecommend
	}
	return nil
}

func (s *goodService) RecommendPost(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	if has {
		return nil, common.ErrAlreadyRecommended
	}

	// If user has downvoted, remove downvote first
	hasNogood, err := s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
	if err != nil {
		return nil, err
	}
	if hasNogood {
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "nogood"); err != nil {
			return nil, err
		}
	}

	if err := s.goodRepo.AddGood(boardID, wrID, userID, "good"); err != nil {
		return nil, err
	}

	good, _, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.RecommendResponse{
		RecommendCount:  good,
		UserRecommended: true,
	}, nil
}

func (s *goodService) CancelRecommendPost(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, common.ErrNotRecommended
	}

	if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "good"); err != nil {
		return nil, err
	}

	good, _, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.RecommendResponse{
		RecommendCount:  good,
		UserRecommended: false,
	}, nil
}

func (s *goodService) DownvotePost(boardID string, wrID int, userID string) (*domain.DownvoteResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
	if err != nil {
		return nil, err
	}
	if has {
		return nil, common.ErrAlreadyRecommended
	}

	// If user has recommended, remove recommend first
	hasGood, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	if hasGood {
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "good"); err != nil {
			return nil, err
		}
	}

	if err := s.goodRepo.AddGood(boardID, wrID, userID, "nogood"); err != nil {
		return nil, err
	}

	_, nogood, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.DownvoteResponse{
		DownvoteCount: nogood,
		UserDownvoted: true,
	}, nil
}

func (s *goodService) CancelDownvotePost(boardID string, wrID int, userID string) (*domain.DownvoteResponse, error) {
	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, common.ErrNotRecommended
	}

	if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "nogood"); err != nil {
		return nil, err
	}

	_, nogood, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.DownvoteResponse{
		DownvoteCount: nogood,
		UserDownvoted: false,
	}, nil
}

func (s *goodService) RecommendComment(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	authorID, err := s.goodRepo.GetWriteAuthorID(boardID, wrID)
	if err != nil {
		return nil, common.ErrCommentNotFound
	}
	if authorID == userID {
		return nil, common.ErrSelfRecommend
	}

	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	if has {
		return nil, common.ErrAlreadyRecommended
	}

	if err := s.goodRepo.AddGood(boardID, wrID, userID, "good"); err != nil {
		return nil, err
	}

	good, _, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.RecommendResponse{
		RecommendCount:  good,
		UserRecommended: true,
	}, nil
}

// ToggleLike toggles like status for a post (frontend-compatible)
func (s *goodService) ToggleLike(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	hasGood, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}

	if hasGood {
		// Cancel like
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "good"); err != nil {
			return nil, err
		}
	} else {
		// Remove dislike if present
		hasNogood, err := s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
		if err != nil {
			return nil, err
		}
		if hasNogood {
			if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "nogood"); err != nil {
				return nil, err
			}
		}
		// Add like
		if err := s.goodRepo.AddGood(boardID, wrID, userID, "good"); err != nil {
			return nil, err
		}
	}

	return s.buildLikeResponse(boardID, wrID, userID)
}

// ToggleDislike toggles dislike status for a post (frontend-compatible)
func (s *goodService) ToggleDislike(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	hasNogood, err := s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
	if err != nil {
		return nil, err
	}

	if hasNogood {
		// Cancel dislike
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "nogood"); err != nil {
			return nil, err
		}
	} else {
		// Remove like if present
		hasGood, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
		if err != nil {
			return nil, err
		}
		if hasGood {
			if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "good"); err != nil {
				return nil, err
			}
		}
		// Add dislike
		if err := s.goodRepo.AddGood(boardID, wrID, userID, "nogood"); err != nil {
			return nil, err
		}
	}

	return s.buildLikeResponse(boardID, wrID, userID)
}

// GetLikeStatus returns the current like/dislike status for a post
func (s *goodService) GetLikeStatus(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	return s.buildLikeResponse(boardID, wrID, userID)
}

// buildLikeResponse builds a frontend-compatible LikeResponse
func (s *goodService) buildLikeResponse(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	good, nogood, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	userLiked := false
	userDisliked := false
	if userID != "" {
		userLiked, _ = s.goodRepo.HasGood(boardID, wrID, userID, "good")
		userDisliked, _ = s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
	}

	return &domain.LikeResponse{
		Likes:        good,
		Dislikes:     nogood,
		UserLiked:    userLiked,
		UserDisliked: userDisliked,
	}, nil
}

func (s *goodService) CancelRecommendComment(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	has, err := s.goodRepo.HasGood(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, common.ErrNotRecommended
	}

	if err := s.goodRepo.RemoveGood(boardID, wrID, userID, "good"); err != nil {
		return nil, err
	}

	good, _, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return nil, err
	}

	return &domain.RecommendResponse{
		RecommendCount:  good,
		UserRecommended: false,
	}, nil
}
