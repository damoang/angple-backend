package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// AIEvaluationRepository handles AI evaluation data operations
type AIEvaluationRepository struct {
	db *gorm.DB
}

// NewAIEvaluationRepository creates a new AIEvaluationRepository
func NewAIEvaluationRepository(db *gorm.DB) *AIEvaluationRepository {
	return &AIEvaluationRepository{db: db}
}

// Create saves a new AI evaluation
func (r *AIEvaluationRepository) Create(eval *domain.AIEvaluation) error {
	return r.db.Create(eval).Error
}

// GetByReport retrieves the latest AI evaluation for a report
func (r *AIEvaluationRepository) GetByReport(table string, parent int) (*domain.AIEvaluation, error) {
	var eval domain.AIEvaluation
	if err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("evaluated_at DESC").
		First(&eval).Error; err != nil {
		return nil, err
	}
	return &eval, nil
}

// ListByReport retrieves all AI evaluations for a report
func (r *AIEvaluationRepository) ListByReport(table string, parent int) ([]domain.AIEvaluation, error) {
	var evals []domain.AIEvaluation
	if err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("evaluated_at DESC").
		Find(&evals).Error; err != nil {
		return nil, err
	}
	return evals, nil
}

// DeleteByReport deletes all AI evaluations for a report (재평가용)
func (r *AIEvaluationRepository) DeleteByReport(table string, parent int) error {
	return r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Delete(&domain.AIEvaluation{}).Error
}
