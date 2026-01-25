package service

import (
	"errors"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

const (
	maxReactionsPerTarget = 20
)

var (
	ErrReactionLimitExceeded = errors.New("리액션은 최대 20개까지 가능합니다")
	ErrLoginRequired         = errors.New("로그인이 필요합니다")
)

// ReactionService handles reaction business logic
type ReactionService struct {
	repo *repository.ReactionRepository
}

// NewReactionService creates a new ReactionService
func NewReactionService(repo *repository.ReactionRepository) *ReactionService {
	return &ReactionService{repo: repo}
}

// GetReactions retrieves reactions for target IDs
func (s *ReactionService) GetReactions(targetIDs []string, memberID string) (map[string][]domain.ReactionItem, error) {
	return s.repo.GetReactions(targetIDs, memberID)
}

// GetReactionsByParent retrieves reactions by parent ID
func (s *ReactionService) GetReactionsByParent(parentID string, memberID string) (map[string][]domain.ReactionItem, error) {
	return s.repo.GetReactionsByParent(parentID, memberID)
}

// React adds or removes a reaction
func (s *ReactionService) React(memberID string, req *domain.ReactionRequest, ip string) (map[string][]domain.ReactionItem, error) {
	if memberID == "" {
		return nil, ErrLoginRequired
	}

	// Check reaction limit for add mode
	if req.ReactionMode == "add" {
		count, err := s.repo.GetReactionCount(req.TargetID)
		if err != nil {
			return nil, err
		}

		if count >= maxReactionsPerTarget {
			// Check if member already has this reaction (allow toggle)
			hasReaction, err := s.repo.HasReaction(memberID, req.TargetID, req.Reaction)
			if err != nil {
				return nil, err
			}
			if !hasReaction {
				return nil, ErrReactionLimitExceeded
			}
		}
	}

	// Check if reaction exists
	hasReaction, err := s.repo.HasReaction(memberID, req.TargetID, req.Reaction)
	if err != nil {
		return nil, err
	}

	if !hasReaction {
		// Add reaction
		if err := s.repo.AddReaction(memberID, req.Reaction, req.TargetID, req.ParentID, ip); err != nil {
			return nil, err
		}
	} else if req.ReactionMode != "add" {
		// Remove reaction (toggle off)
		if err := s.repo.RemoveReaction(memberID, req.Reaction, req.TargetID); err != nil {
			return nil, err
		}
	}

	// Return updated reactions
	return s.repo.GetReactions([]string{req.TargetID}, memberID)
}
