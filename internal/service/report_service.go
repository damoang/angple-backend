package service

import (
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

const (
	ReportStatusPending    = "pending"
	ReportStatusMonitoring = "monitoring"
	ReportStatusApproved   = "approved"
	ReportStatusDismissed  = "dismissed"
)

var (
	ErrReportNotFound   = errors.New("신고를 찾을 수 없습니다")
	ErrInvalidAction    = errors.New("유효하지 않은 액션입니다")
	ErrAlreadyProcessed = errors.New("이미 처리된 신고입니다")
	ErrReportAdminOnly  = errors.New("관리자 권한이 필요합니다")
)

// ReportService handles report business logic
type ReportService struct {
	repo *repository.ReportRepository
}

// NewReportService creates a new ReportService
func NewReportService(repo *repository.ReportRepository) *ReportService {
	return &ReportService{repo: repo}
}

// List retrieves paginated reports
func (s *ReportService) List(status string, page, limit int) ([]domain.ReportListResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	reports, total, err := s.repo.List(status, page, limit)
	if err != nil {
		return nil, 0, err
	}

	// Convert to response format
	responses := make([]domain.ReportListResponse, len(reports))
	for i, report := range reports {
		responses[i] = domain.ReportListResponse{
			ID:         report.ID,
			Table:      report.Table,
			Parent:     report.Parent,
			ReporterID: report.ReporterID,
			TargetID:   report.TargetID,
			Reason:     report.Reason,
			Status:     report.Status(), // Call Status() method
			CreatedAt:  report.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return responses, total, nil
}

// GetRecent retrieves recent reports
func (s *ReportService) GetRecent(limit int) ([]domain.ReportListResponse, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}

	reports, err := s.repo.GetRecent(limit)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	responses := make([]domain.ReportListResponse, len(reports))
	for i, report := range reports {
		responses[i] = domain.ReportListResponse{
			ID:         report.ID,
			Table:      report.Table,
			Parent:     report.Parent,
			ReporterID: report.ReporterID,
			TargetID:   report.TargetID,
			Reason:     report.Reason,
			Status:     report.Status(), // Call Status() method
			CreatedAt:  report.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return responses, nil
}

// GetData retrieves report data by table and id
func (s *ReportService) GetData(table string, id int) (*domain.Report, error) {
	report, err := s.repo.GetByTableAndParent(table, id)
	if err != nil {
		return nil, ErrReportNotFound
	}
	return report, nil
}

// Process processes a report action
func (s *ReportService) Process(adminID string, req *domain.ReportActionRequest) error {
	// Validate action
	validActions := map[string]bool{
		"submitOpinion": true,
		"cancelOpinion": true,
		"adminApprove":  true,
		"adminDismiss":  true,
	}

	if !validActions[req.Action] {
		return ErrInvalidAction
	}

	// Get report
	report, err := s.repo.GetByTableAndParent(req.Table, req.Parent)
	if err != nil {
		return ErrReportNotFound
	}

	// Check if already processed (for admin actions)
	currentStatus := report.Status()
	if (req.Action == "adminApprove" || req.Action == "adminDismiss") &&
		(currentStatus == ReportStatusApproved || currentStatus == ReportStatusDismissed) {
		return ErrAlreadyProcessed
	}

	// Process based on action
	var newStatus string
	switch req.Action {
	case "submitOpinion":
		newStatus = ReportStatusMonitoring
	case "cancelOpinion":
		newStatus = ReportStatusPending
	case "adminApprove":
		newStatus = ReportStatusApproved
	case "adminDismiss":
		newStatus = ReportStatusDismissed
	}

	return s.repo.UpdateStatus(report.ID, newStatus, adminID)
}

// Create creates a new report
func (s *ReportService) Create(reporterID, targetID, table string, parent int, reason string) (*domain.Report, error) {
	now := time.Now()
	report := &domain.Report{
		Table:             table,
		Parent:            parent,
		ReporterID:        reporterID,
		TargetID:          targetID,
		Reason:            reason,
		Flag:              0, // pending
		Processed:         false,
		MonitoringChecked: false,
		AdminApproved:     false,
		CreatedAt:         now,
		WriteTime:         now,
		IP:                "0.0.0.0",
	}

	if err := s.repo.Create(report); err != nil {
		return nil, err
	}

	return report, nil
}

// GetMyReports returns reports submitted by the given user
func (s *ReportService) GetMyReports(userID string, limit int) ([]domain.ReportListResponse, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}

	reports, err := s.repo.GetByReporter(userID, limit)
	if err != nil {
		return nil, err
	}

	responses := make([]domain.ReportListResponse, len(reports))
	for i, report := range reports {
		responses[i] = domain.ReportListResponse{
			ID:         report.ID,
			Table:      report.Table,
			Parent:     report.Parent,
			ReporterID: report.ReporterID,
			TargetID:   report.TargetID,
			Reason:     report.Reason,
			Status:     report.Status(),
			CreatedAt:  report.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return responses, nil
}

// GetStats retrieves report statistics
func (s *ReportService) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	statuses := []string{ReportStatusPending, ReportStatusMonitoring, ReportStatusApproved, ReportStatusDismissed}
	for _, status := range statuses {
		count, err := s.repo.CountByStatus(status)
		if err != nil {
			return nil, err
		}
		stats[status] = count
	}

	// Total count
	total, err := s.repo.CountByStatus("")
	if err != nil {
		return nil, err
	}
	stats["total"] = total

	return stats, nil
}
