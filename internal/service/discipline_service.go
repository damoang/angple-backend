package service

import (
	"encoding/json"
	"fmt"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// DisciplineService business logic for discipline/ban management
type DisciplineService interface {
	GetMyDisciplines(memberID string, page, limit int) ([]*domain.DisciplineResponse, *common.Meta, error)
	GetDiscipline(id int, memberID string) (*domain.DisciplineResponse, error)
	SubmitAppeal(disciplineID int, memberID, memberName, content, ip string) (int, error)
	ListBoard(page, limit int) ([]*domain.DisciplineResponse, *common.Meta, error)
}

type disciplineService struct {
	repo *repository.DisciplineRepository
}

// NewDisciplineService creates a new DisciplineService
func NewDisciplineService(repo *repository.DisciplineRepository) DisciplineService {
	return &disciplineService{repo: repo}
}

// GetMyDisciplines returns discipline logs for the given member
func (s *disciplineService) GetMyDisciplines(memberID string, page, limit int) ([]*domain.DisciplineResponse, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	logs, total, err := s.repo.FindByTargetMember(memberID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.DisciplineResponse, len(logs))
	for i, log := range logs {
		responses[i] = toDisciplineResponse(&log)
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}
	return responses, meta, nil
}

// GetDiscipline returns a single discipline log (only if it belongs to the member)
func (s *disciplineService) GetDiscipline(id int, memberID string) (*domain.DisciplineResponse, error) {
	log, err := s.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("이용제한 내역을 찾을 수 없습니다")
	}

	// Parse content to verify target_id matches
	var content domain.DisciplineLogContent
	if err := json.Unmarshal([]byte(log.Content), &content); err == nil {
		if content.TargetID != memberID {
			return nil, fmt.Errorf("권한이 없습니다")
		}
	}

	return toDisciplineResponse(log), nil
}

// SubmitAppeal creates an appeal comment under a discipline log
func (s *disciplineService) SubmitAppeal(disciplineID int, memberID, memberName, content, ip string) (int, error) {
	// Verify the discipline log belongs to this member
	log, err := s.repo.GetByID(disciplineID)
	if err != nil {
		return 0, fmt.Errorf("이용제한 내역을 찾을 수 없습니다")
	}

	var logContent domain.DisciplineLogContent
	if err := json.Unmarshal([]byte(log.Content), &logContent); err == nil {
		if logContent.TargetID != memberID {
			return 0, fmt.Errorf("권한이 없습니다")
		}
	}

	return s.repo.CreateAppeal(disciplineID, memberID, memberName, content, ip)
}

// ListBoard returns all discipline logs (이용제한 게시판)
func (s *disciplineService) ListBoard(page, limit int) ([]*domain.DisciplineResponse, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	logs, total, err := s.repo.ListAll(page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.DisciplineResponse, len(logs))
	for i, log := range logs {
		responses[i] = toDisciplineResponse(&log)
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}
	return responses, meta, nil
}

func toDisciplineResponse(log *domain.DisciplineLog) *domain.DisciplineResponse {
	resp := &domain.DisciplineResponse{
		ID:           log.ID,
		Subject:      log.Subject,
		Status:       log.Wr4,
		ProcessType:  log.Wr7,
		CreatedAt:    log.DateTime.Format("2006-01-02 15:04:05"),
		CommentCount: log.Comment,
	}

	var content domain.DisciplineLogContent
	if err := json.Unmarshal([]byte(log.Content), &content); err == nil {
		resp.Content = &content
	}

	return resp
}
