package repository

import (
	"fmt"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OpinionRepository handles opinion data operations
type OpinionRepository struct {
	db *gorm.DB
}

// NewOpinionRepository creates a new OpinionRepository
func NewOpinionRepository(db *gorm.DB) *OpinionRepository {
	return &OpinionRepository{db: db}
}

// Save creates or updates an opinion (upsert by unique key: sg_table, sg_id, sg_parent, reviewer_id)
func (r *OpinionRepository) Save(opinion *domain.Opinion) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "sg_table"},
			{Name: "sg_id"},
			{Name: "sg_parent"},
			{Name: "reviewer_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"opinion_type",
			"discipline_reasons",
			"discipline_days",
			"discipline_type",
			"discipline_detail",
			"updated_at",
		}),
	}).Create(opinion).Error
}

// GetByReport retrieves all opinions for a specific report
func (r *OpinionRepository) GetByReport(table string, sgID, parent int) ([]domain.Opinion, error) {
	var opinions []domain.Opinion
	err := r.db.Where("sg_table = ? AND sg_id = ? AND sg_parent = ?", table, sgID, parent).
		Order("created_at ASC").
		Find(&opinions).Error
	return opinions, err
}

// GetByReportGrouped retrieves all opinions for a report grouped by table+parent (ignoring sg_id)
func (r *OpinionRepository) GetByReportGrouped(table string, parent int) ([]domain.Opinion, error) {
	var opinions []domain.Opinion
	err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("created_at ASC").
		Find(&opinions).Error
	return opinions, err
}

// CountByReport counts action and dismiss opinions for a report
func (r *OpinionRepository) CountByReport(table string, sgID, parent int) (actionCount, dismissCount int64, err error) {
	var results []struct {
		OpinionType string
		Count       int64
	}

	err = r.db.Model(&domain.Opinion{}).
		Select("opinion_type, COUNT(*) as count").
		Where("sg_table = ? AND sg_id = ? AND sg_parent = ?", table, sgID, parent).
		Group("opinion_type").
		Find(&results).Error

	if err != nil {
		return 0, 0, err
	}

	for _, r := range results {
		switch r.OpinionType {
		case "action":
			actionCount = r.Count
		case "dismiss":
			dismissCount = r.Count
		}
	}
	return
}

// CountByReportGrouped counts opinions grouped by table+parent
func (r *OpinionRepository) CountByReportGrouped(table string, parent int) (actionCount, dismissCount int64, err error) {
	var results []struct {
		OpinionType string
		Count       int64
	}

	err = r.db.Model(&domain.Opinion{}).
		Select("opinion_type, COUNT(*) as count").
		Where("sg_table = ? AND sg_parent = ?", table, parent).
		Group("opinion_type").
		Find(&results).Error

	if err != nil {
		return 0, 0, err
	}

	for _, r := range results {
		switch r.OpinionType {
		case "action":
			actionCount = r.Count
		case "dismiss":
			dismissCount = r.Count
		}
	}
	return
}

// Delete deletes a specific opinion
func (r *OpinionRepository) Delete(table string, sgID, parent int, reviewerID string) error {
	return r.db.Where("sg_table = ? AND sg_id = ? AND sg_parent = ? AND reviewer_id = ?",
		table, sgID, parent, reviewerID).
		Delete(&domain.Opinion{}).Error
}

// DeleteByReportGrouped deletes all opinions for a table+parent
func (r *OpinionRepository) DeleteByReportGrouped(table string, parent int) error {
	return r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Delete(&domain.Opinion{}).Error
}

// GetByMultipleReportsGrouped retrieves opinions for multiple table+parent combinations in a single query
// Returns map keyed by "table:parent" string
func (r *OpinionRepository) GetByMultipleReportsGrouped(keys []struct {
	Table  string
	Parent int
}) (map[string][]domain.Opinion, error) {
	if len(keys) == 0 {
		return map[string][]domain.Opinion{}, nil
	}

	// Build OR conditions: (sg_table = ? AND sg_parent = ?) OR ...
	tx := r.db.Model(&domain.Opinion{})
	var conditions []string
	var args []interface{}
	for _, k := range keys {
		conditions = append(conditions, "(sg_table = ? AND sg_parent = ?)")
		args = append(args, k.Table, k.Parent)
	}
	query := strings.Join(conditions, " OR ")

	var opinions []domain.Opinion
	if err := tx.Where(query, args...).Order("created_at ASC").Find(&opinions).Error; err != nil {
		return nil, err
	}

	result := make(map[string][]domain.Opinion, len(keys))
	for _, op := range opinions {
		key := fmt.Sprintf("%s:%d", op.Table, op.Parent)
		result[key] = append(result[key], op)
	}
	return result, nil
}

// GetMatchingActionOpinions retrieves action opinions with matching reasons and days
func (r *OpinionRepository) GetMatchingActionOpinions(table string, parent int) ([]domain.Opinion, error) {
	var opinions []domain.Opinion
	err := r.db.Where("sg_table = ? AND sg_parent = ? AND opinion_type = ?", table, parent, "action").
		Order("created_at ASC").
		Find(&opinions).Error
	return opinions, err
}
