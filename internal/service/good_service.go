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

// addVote adds a vote, optionally removing the opposite vote first
func (s *goodService) addVote(boardID string, wrID int, userID, voteType, oppositeType string) error {
	has, err := s.goodRepo.HasGood(boardID, wrID, userID, voteType)
	if err != nil {
		return err
	}
	if has {
		return common.ErrAlreadyRecommended
	}

	// Remove opposite vote if present
	hasOpposite, err := s.goodRepo.HasGood(boardID, wrID, userID, oppositeType)
	if err != nil {
		return err
	}
	if hasOpposite {
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, oppositeType); err != nil {
			return err
		}
	}

	return s.goodRepo.AddGood(boardID, wrID, userID, voteType)
}

// cancelVote removes a vote and returns the updated count
func (s *goodService) cancelVote(boardID string, wrID int, userID, voteType string) (int, int, error) {
	has, err := s.goodRepo.HasGood(boardID, wrID, userID, voteType)
	if err != nil {
		return 0, 0, err
	}
	if !has {
		return 0, 0, common.ErrNotRecommended
	}

	if err := s.goodRepo.RemoveGood(boardID, wrID, userID, voteType); err != nil {
		return 0, 0, err
	}

	good, nogood, err := s.goodRepo.GetGoodCount(boardID, wrID)
	if err != nil {
		return 0, 0, err
	}
	return good, nogood, nil
}

// toggleVote toggles a vote type, removing opposite if needed
func (s *goodService) toggleVote(boardID string, wrID int, userID, voteType, oppositeType string) (*domain.LikeResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	hasVote, err := s.goodRepo.HasGood(boardID, wrID, userID, voteType)
	if err != nil {
		return nil, err
	}

	if hasVote {
		if err := s.goodRepo.RemoveGood(boardID, wrID, userID, voteType); err != nil {
			return nil, err
		}
	} else {
		hasOpposite, err := s.goodRepo.HasGood(boardID, wrID, userID, oppositeType)
		if err != nil {
			return nil, err
		}
		if hasOpposite {
			if err := s.goodRepo.RemoveGood(boardID, wrID, userID, oppositeType); err != nil {
				return nil, err
			}
		}
		if err := s.goodRepo.AddGood(boardID, wrID, userID, voteType); err != nil {
			return nil, err
		}
	}

	return s.buildLikeResponse(boardID, wrID, userID)
}

func (s *goodService) RecommendPost(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	if err := s.checkAuthorAndReject(boardID, wrID, userID); err != nil {
		return nil, err
	}

	if err := s.addVote(boardID, wrID, userID, "good", "nogood"); err != nil {
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
	good, _, err := s.cancelVote(boardID, wrID, userID, "good")
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

	if err := s.addVote(boardID, wrID, userID, "nogood", "good"); err != nil {
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
	_, nogood, err := s.cancelVote(boardID, wrID, userID, "nogood")
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

	if err := s.addVote(boardID, wrID, userID, "good", "nogood"); err != nil {
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

func (s *goodService) CancelRecommendComment(boardID string, wrID int, userID string) (*domain.RecommendResponse, error) {
	good, _, err := s.cancelVote(boardID, wrID, userID, "good")
	if err != nil {
		return nil, err
	}
	return &domain.RecommendResponse{
		RecommendCount:  good,
		UserRecommended: false,
	}, nil
}

// ToggleLike toggles like status for a post (frontend-compatible)
func (s *goodService) ToggleLike(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	return s.toggleVote(boardID, wrID, userID, "good", "nogood")
}

// ToggleDislike toggles dislike status for a post (frontend-compatible)
func (s *goodService) ToggleDislike(boardID string, wrID int, userID string) (*domain.LikeResponse, error) {
	return s.toggleVote(boardID, wrID, userID, "nogood", "good")
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
		userLiked, err = s.goodRepo.HasGood(boardID, wrID, userID, "good")
		if err != nil {
			userLiked = false
		}
		userDisliked, err = s.goodRepo.HasGood(boardID, wrID, userID, "nogood")
		if err != nil {
			userDisliked = false
		}
	}

	return &domain.LikeResponse{
		Likes:        good,
		Dislikes:     nogood,
		UserLiked:    userLiked,
		UserDisliked: userDisliked,
	}, nil
}
