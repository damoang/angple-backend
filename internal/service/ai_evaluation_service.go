package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

var (
	ErrAIEvaluationNotFound = errors.New("AI 평가 결과를 찾을 수 없습니다")
)

// AIEvaluationService handles AI evaluation business logic
type AIEvaluationService struct {
	repo *repository.AIEvaluationRepository
}

// NewAIEvaluationService creates a new AIEvaluationService
func NewAIEvaluationService(repo *repository.AIEvaluationRepository) *AIEvaluationService {
	return &AIEvaluationService{repo: repo}
}

// Save stores an AI evaluation result
func (s *AIEvaluationService) Save(adminID string, req *domain.SaveAIEvaluationRequest) (*domain.AIEvaluation, error) {
	// Marshal arrays to JSON strings
	penaltyTypeJSON, err := json.Marshal(req.PenaltyType)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal penalty type: %w", err)
	}
	penaltyReasonsJSON, err := json.Marshal(req.PenaltyReasons)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal penalty reasons: %w", err)
	}
	flagsJSON, err := json.Marshal(req.Flags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal flags: %w", err)
	}

	evaluatedAt := time.Now()
	if req.EvaluatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, req.EvaluatedAt); err == nil {
			evaluatedAt = parsed
		}
	}

	eval := &domain.AIEvaluation{
		Table:             req.Table,
		Parent:            req.Parent,
		Score:             req.Score,
		Confidence:        req.Confidence,
		RecommendedAction: req.RecommendedAction,
		PenaltyDays:       req.PenaltyDays,
		PenaltyType:       string(penaltyTypeJSON),
		PenaltyReasons:    string(penaltyReasonsJSON),
		Reasoning:         req.Reasoning,
		Flags:             string(flagsJSON),
		RawResponse:       req.RawResponse,
		Model:             req.Model,
		EvaluatedAt:       evaluatedAt,
		EvaluatedBy:       adminID,
		CreatedAt:         time.Now(),
	}

	if err := s.repo.Create(eval); err != nil {
		return nil, err
	}

	return eval, nil
}

// GetByReport retrieves the latest AI evaluation for a report
func (s *AIEvaluationService) GetByReport(table string, parent int) (*domain.AIEvaluation, error) {
	eval, err := s.repo.GetByReport(table, parent)
	if err != nil {
		return nil, ErrAIEvaluationNotFound
	}
	return eval, nil
}

// ListByReport retrieves all AI evaluations for a report
func (s *AIEvaluationService) ListByReport(table string, parent int) ([]domain.AIEvaluation, error) {
	return s.repo.ListByReport(table, parent)
}
