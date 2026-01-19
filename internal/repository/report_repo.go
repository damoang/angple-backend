package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// ReportRepository handles report data operations
type ReportRepository struct {
	db *gorm.DB
}

// NewReportRepository creates a new ReportRepository
func NewReportRepository(db *gorm.DB) *ReportRepository {
	return &ReportRepository{db: db}
}

// List retrieves paginated reports with optional status filter
func (r *ReportRepository) List(status string, page, limit int) ([]domain.Report, int64, error) {
	var reports []domain.Report
	var total int64

	query := r.db.Model(&domain.Report{})

	if status != "" {
		query = query.Where("sg_status = ?", status)
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("sg_datetime DESC").
		Offset(offset).
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// GetByID retrieves a single report by ID
func (r *ReportRepository) GetByID(id int) (*domain.Report, error) {
	var report domain.Report
	if err := r.db.Where("sg_id = ?", id).First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

// GetByTableAndParent retrieves a report by table and parent
func (r *ReportRepository) GetByTableAndParent(table string, parent int) (*domain.Report, error) {
	var report domain.Report
	if err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

// GetRecent retrieves recent reports
func (r *ReportRepository) GetRecent(limit int) ([]domain.Report, error) {
	var reports []domain.Report
	if err := r.db.Order("sg_datetime DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// Create creates a new report
func (r *ReportRepository) Create(report *domain.Report) error {
	return r.db.Create(report).Error
}

// UpdateStatus updates report status
func (r *ReportRepository) UpdateStatus(id int, status, processedBy string) error {
	return r.db.Model(&domain.Report{}).
		Where("sg_id = ?", id).
		Updates(map[string]interface{}{
			"sg_status":       status,
			"sg_processed_by": processedBy,
			"sg_processed_at": gorm.Expr("NOW()"),
		}).Error
}

// Delete deletes a report
func (r *ReportRepository) Delete(id int) error {
	return r.db.Where("sg_id = ?", id).Delete(&domain.Report{}).Error
}

// CountByStatus counts reports by status
func (r *ReportRepository) CountByStatus(status string) (int64, error) {
	var count int64
	query := r.db.Model(&domain.Report{})
	if status != "" {
		query = query.Where("sg_status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

// GetByReporter retrieves reports by reporter ID
func (r *ReportRepository) GetByReporter(reporterID string, limit int) ([]domain.Report, error) {
	var reports []domain.Report
	if err := r.db.Where("mb_id = ?", reporterID).
		Order("sg_datetime DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// GetByTarget retrieves reports by target member ID
func (r *ReportRepository) GetByTarget(targetID string, limit int) ([]domain.Report, error) {
	var reports []domain.Report
	if err := r.db.Where("target_mb_id = ?", targetID).
		Order("sg_datetime DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}
