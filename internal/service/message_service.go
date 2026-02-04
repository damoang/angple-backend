package service

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// MessageService business logic for private messages
type MessageService interface {
	SendMessage(senderID string, req *domain.SendMessageRequest, senderIP string) (*domain.MessageResponse, error)
	GetInbox(mbID string, page, limit int) ([]*domain.MessageResponse, *common.Meta, error)
	GetSent(mbID string, page, limit int) ([]*domain.MessageResponse, *common.Meta, error)
	GetMessage(id int, mbID string) (*domain.MessageResponse, error)
	DeleteMessage(id int, mbID string) error
}

type messageService struct {
	repo       repository.MessageRepository
	memberRepo repository.MemberRepository
	blockRepo  repository.BlockRepository
}

// NewMessageService creates a new MessageService
func NewMessageService(repo repository.MessageRepository, memberRepo repository.MemberRepository, blockRepo repository.BlockRepository) MessageService {
	return &messageService{
		repo:       repo,
		memberRepo: memberRepo,
		blockRepo:  blockRepo,
	}
}

// SendMessage sends a private message
func (s *messageService) SendMessage(senderID string, req *domain.SendMessageRequest, senderIP string) (*domain.MessageResponse, error) {
	if senderID == req.ToUserID {
		return nil, fmt.Errorf("자기 자신에게 쪽지를 보낼 수 없습니다")
	}

	// 수신자 존재 확인
	_, err := s.memberRepo.FindByUserID(req.ToUserID)
	if err != nil {
		return nil, fmt.Errorf("수신자를 찾을 수 없습니다")
	}

	// 차단 여부 확인 (수신자가 발신자를 차단한 경우)
	blocked, err := s.blockRepo.Exists(req.ToUserID, senderID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, fmt.Errorf("쪽지를 보낼 수 없는 회원입니다")
	}

	msg := &domain.Message{
		RecvMbID:     req.ToUserID,
		SendMbID:     senderID,
		Memo:         req.Content,
		SendDatetime: time.Now(),
		SendIP:       senderIP,
	}

	if err := s.repo.Create(msg); err != nil {
		return nil, err
	}

	return msg.ToResponse(), nil
}

// GetInbox returns received messages
func (s *messageService) GetInbox(mbID string, page, limit int) ([]*domain.MessageResponse, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	messages, total, err := s.repo.FindInbox(mbID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.MessageResponse, len(messages))
	for i, m := range messages {
		responses[i] = m.ToResponse()
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	return responses, meta, nil
}

// GetSent returns sent messages
func (s *messageService) GetSent(mbID string, page, limit int) ([]*domain.MessageResponse, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	messages, total, err := s.repo.FindSent(mbID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.MessageResponse, len(messages))
	for i, m := range messages {
		responses[i] = m.ToResponse()
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	return responses, meta, nil
}

// GetMessage returns a single message and marks as read
func (s *messageService) GetMessage(id int, mbID string) (*domain.MessageResponse, error) {
	msg, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("쪽지를 찾을 수 없습니다")
	}

	// 본인의 쪽지인지 확인
	if msg.RecvMbID != mbID && msg.SendMbID != mbID {
		return nil, fmt.Errorf("권한이 없습니다")
	}

	// 수신 쪽지이고 읽지 않은 경우 읽음 처리
	if msg.RecvMbID == mbID && msg.ReadDatetime == nil {
		s.repo.MarkAsRead(id) //nolint:errcheck
		now := time.Now()
		msg.ReadDatetime = &now
	}

	return msg.ToResponse(), nil
}

// DeleteMessage deletes a message
func (s *messageService) DeleteMessage(id int, mbID string) error {
	return s.repo.Delete(id, mbID)
}
