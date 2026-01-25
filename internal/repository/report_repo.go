package repository

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// Report status constants
const (
	ReportStatusPending    = "pending"
	ReportStatusMonitoring = "monitoring"
	ReportStatusApproved   = "approved"
	ReportStatusDismissed  = "dismissed"
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
// Status mapping: pending=0, monitoring=1, approved=2, dismissed=3
func (r *ReportRepository) List(status string, page, limit int) ([]domain.Report, int64, error) {
	var reports []domain.Report
	var total int64

	query := r.db.Model(&domain.Report{})

	// Filter by status
	switch status {
	case ReportStatusPending:
		query = query.Where("processed = 0 AND monitoring_checked = 0")
	case ReportStatusMonitoring:
		query = query.Where("processed = 0 AND monitoring_checked = 1")
	case ReportStatusApproved:
		query = query.Where("processed = 1 AND admin_approved = 1")
	case ReportStatusDismissed:
		query = query.Where("processed = 1 AND admin_approved = 0")
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	if err := query.Order("sg_time DESC").
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
	if err := r.db.Where("id = ?", id).First(&report).Error; err != nil {
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
	if err := r.db.Order("sg_time DESC").
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

// UpdateStatus updates report status based on action
func (r *ReportRepository) UpdateStatus(id int, status, processedBy string) error {
	updates := map[string]interface{}{}

	switch status {
	case ReportStatusMonitoring:
		updates["monitoring_checked"] = true
		updates["monitoring_datetime"] = gorm.Expr("NOW()")
	case ReportStatusPending:
		updates["monitoring_checked"] = false
		updates["monitoring_datetime"] = nil
	case ReportStatusApproved:
		updates["processed"] = true
		updates["admin_approved"] = true
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
	case ReportStatusDismissed:
		updates["processed"] = true
		updates["admin_approved"] = false
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
	}

	// Add admin user to admin_users field
	if processedBy != "" && (status == ReportStatusApproved || status == ReportStatusDismissed) {
		updates["admin_users"] = processedBy
	}

	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// Delete deletes a report
func (r *ReportRepository) Delete(id int) error {
	return r.db.Where("id = ?", id).Delete(&domain.Report{}).Error
}

// CountByStatus counts reports by status
func (r *ReportRepository) CountByStatus(status string) (int64, error) {
	var count int64
	query := r.db.Model(&domain.Report{})

	switch status {
	case ReportStatusPending:
		query = query.Where("processed = 0 AND monitoring_checked = 0")
	case ReportStatusMonitoring:
		query = query.Where("processed = 0 AND monitoring_checked = 1")
	case ReportStatusApproved:
		query = query.Where("processed = 1 AND admin_approved = 1")
	case ReportStatusDismissed:
		query = query.Where("processed = 1 AND admin_approved = 0")
	}

	err := query.Count(&count).Error
	return count, err
}

// GetByReporter retrieves reports by reporter ID
func (r *ReportRepository) GetByReporter(reporterID string, limit int) ([]domain.Report, error) {
	var reports []domain.Report
	if err := r.db.Where("mb_id = ?", reporterID).
		Order("sg_time DESC").
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
		Order("sg_time DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}
