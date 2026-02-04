package service

import (
	"fmt"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// BlockService business logic for member blocking
type BlockService interface {
	BlockMember(mbID string, targetMbID string) (*domain.BlockResponse, error)
	UnblockMember(mbID string, targetMbID string) error
	ListBlocks(mbID string) ([]*domain.BlockResponse, error)
	GetBlockedUserIDs(mbID string) ([]string, error)
}

type blockService struct {
	blockRepo  repository.BlockRepository
	memberRepo repository.MemberRepository
}

// NewBlockService creates a new BlockService
func NewBlockService(blockRepo repository.BlockRepository, memberRepo repository.MemberRepository) BlockService {
	return &blockService{
		blockRepo:  blockRepo,
		memberRepo: memberRepo,
	}
}

// BlockMember blocks a member
func (s *blockService) BlockMember(mbID string, targetMbID string) (*domain.BlockResponse, error) {
	if mbID == targetMbID {
		return nil, fmt.Errorf("자기 자신을 차단할 수 없습니다")
	}

	// 대상 회원 존재 확인
	target, err := s.memberRepo.FindByUserID(targetMbID)
	if err != nil {
		return nil, fmt.Errorf("회원을 찾을 수 없습니다")
	}

	exists, err := s.blockRepo.Exists(mbID, targetMbID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("이미 차단한 회원입니다")
	}

	block, err := s.blockRepo.Create(mbID, targetMbID)
	if err != nil {
		return nil, err
	}

	return &domain.BlockResponse{
		BlockID:   block.ID,
		UserID:    targetMbID,
		Nickname:  target.Nickname,
		BlockedAt: block.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// UnblockMember unblocks a member
func (s *blockService) UnblockMember(mbID string, targetMbID string) error {
	return s.blockRepo.Delete(mbID, targetMbID)
}

// ListBlocks returns all blocked members
func (s *blockService) ListBlocks(mbID string) ([]*domain.BlockResponse, error) {
	blocks, err := s.blockRepo.FindByMember(mbID)
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.BlockResponse, len(blocks))
	for i, b := range blocks {
		nickname := ""
		if member, err := s.memberRepo.FindByUserID(b.BlockedMbID); err == nil {
			nickname = member.Nickname
		}
		responses[i] = &domain.BlockResponse{
			BlockID:   b.ID,
			UserID:    b.BlockedMbID,
			Nickname:  nickname,
			BlockedAt: b.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return responses, nil
}

// GetBlockedUserIDs returns all blocked user IDs
func (s *blockService) GetBlockedUserIDs(mbID string) ([]string, error) {
	return s.blockRepo.GetBlockedUserIDs(mbID)
}
