package service

import (
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// MemoService handles member memo business logic
type MemoService struct {
	memoRepo   *repository.MemoRepository
	memberRepo repository.MemberRepository
}

// NewMemoService creates a new MemoService
func NewMemoService(memoRepo *repository.MemoRepository, memberRepo repository.MemberRepository) *MemoService {
	return &MemoService{
		memoRepo:   memoRepo,
		memberRepo: memberRepo,
	}
}

// GetMemo retrieves a memo by member ID and target member ID
func (s *MemoService) GetMemo(memberID, targetMemberID string) (*domain.MemberMemo, error) {
	return s.memoRepo.GetMemo(memberID, targetMemberID)
}

// GetMemoList retrieves all memos by member ID
func (s *MemoService) GetMemoList(memberID string, page, limit int) ([]domain.MemberMemo, int64, error) {
	offset := (page - 1) * limit
	return s.memoRepo.GetMemoList(memberID, offset, limit)
}

// CreateOrUpdateMemo creates or updates a memo
func (s *MemoService) CreateOrUpdateMemo(memberID string, targetMemberID string, req *domain.MemoRequest) (*domain.MemberMemo, error) {
	// Get member info
	member, err := s.memberRepo.FindByUserID(memberID)
	if err != nil {
		return nil, err
	}

	// Get target member info
	targetMember, err := s.memberRepo.FindByUserID(targetMemberID)
	if err != nil {
		return nil, err
	}

	// Set default color
	color := req.Color
	if color == "" {
		color = "yellow"
	}

	memo := &domain.MemberMemo{
		MemberUID:       member.ID,
		MemberID:        memberID,
		TargetMemberUID: targetMember.ID,
		TargetMemberID:  targetMemberID,
		Memo:            req.Content,
		MemoDetail:      req.MemoDetail,
		Color:           color,
	}

	if err := s.memoRepo.UpsertMemo(memo); err != nil {
		return nil, err
	}

	return memo, nil
}

// DeleteMemo deletes a memo
func (s *MemoService) DeleteMemo(memberID, targetMemberID string) error {
	return s.memoRepo.DeleteMemo(memberID, targetMemberID)
}
