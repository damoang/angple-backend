package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

var (
	// ErrVersionConflict indicates that a concurrent modification was detected
	ErrVersionConflict = errors.New("다른 관리자가 먼저 처리했습니다. 새로고침 후 다시 시도해주세요")
)

// Report status constants
const (
	ReportStatusPending    = "pending"
	ReportStatusMonitoring = "monitoring"
	ReportStatusHold       = "hold"
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
func (r *ReportRepository) List(status string, page, limit int, fromDate, toDate string) ([]domain.Report, int64, error) {
	var reports []domain.Report
	var total int64

	query := r.db.Model(&domain.Report{})

	// Filter by status
	switch status {
	case ReportStatusPending:
		query = query.Where("processed = 0 AND monitoring_checked = 0 AND hold = 0")
	case ReportStatusMonitoring:
		query = query.Where("processed = 0 AND monitoring_checked = 1 AND hold = 0")
	case ReportStatusHold:
		query = query.Where("processed = 0 AND hold = 1")
	case ReportStatusApproved:
		query = query.Where("processed = 1 AND admin_approved = 1")
	case ReportStatusDismissed:
		query = query.Where("processed = 1 AND admin_approved = 0")
	}

	// Filter by date range
	if fromDate != "" {
		query = query.Where("sg_time >= ?", fromDate+" 00:00:00")
	}
	if toDate != "" {
		query = query.Where("sg_time <= ?", toDate+" 23:59:59")
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

// GetByTableAndParent retrieves the most relevant report by table and parent.
// Prioritizes unprocessed (processed=0) reports, then most recent.
// Searches by sg_id first (for sg_id-based grouping), then sg_parent (legacy).
func (r *ReportRepository) GetByTableAndParent(table string, parent int) (*domain.Report, error) {
	var report domain.Report
	// Try sg_id first (post reports: sg_id = sg_parent, comment reports: sg_id = comment_id)
	if err := r.db.Where("sg_table = ? AND sg_id = ?", table, parent).
		Order("processed ASC, sg_time DESC").
		First(&report).Error; err == nil {
		return &report, nil
	}

	// Fallback to sg_parent (legacy compatibility)
	if err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("processed ASC, sg_time DESC").
		First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

// GetAllByTableAndParent retrieves all reports for a given table and parent
// First tries to find by sg_id, then falls back to sg_parent for backward compatibility
func (r *ReportRepository) GetAllByTableAndParent(table string, parent int) ([]domain.Report, error) {
	var reports []domain.Report
	// Try sg_id first
	if err := r.db.Where("sg_table = ? AND sg_id = ?", table, parent).
		Order("sg_time DESC").
		Find(&reports).Error; err == nil && len(reports) > 0 {
		return reports, nil
	}

	// Fallback to sg_parent
	if err := r.db.Where("sg_table = ? AND sg_parent = ?", table, parent).
		Order("sg_time DESC").
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// GetByTableAndSgID retrieves the primary report by table and sg_id.
// Returns the report with matching sg_id (unique identifier for specific report).
// If parent is 0, it's ignored (used for URL-based lookups without parent info).
func (r *ReportRepository) GetByTableAndSgID(table string, sgID, parent int) (*domain.Report, error) {
	var report domain.Report
	query := r.db.Where("sg_table = ? AND sg_id = ?", table, sgID)

	// Only add parent condition if it's provided (non-zero)
	if parent > 0 {
		query = query.Where("sg_parent = ?", parent)
	}

	if err := query.First(&report).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

// GetAllByTableAndSgID retrieves all reports for the same content based on sg_id.
// Uses sg_id to find sg_parent, then returns all reports with matching table+parent.
func (r *ReportRepository) GetAllByTableAndSgID(table string, sgID, parent int) ([]domain.Report, error) {
	// First get the primary report to confirm sg_parent
	primaryReport, err := r.GetByTableAndSgID(table, sgID, parent)
	if err != nil {
		return nil, err
	}

	// Return all reports for this content (table + parent)
	return r.GetAllByTableAndParent(table, primaryReport.Parent)
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
	case ReportStatusHold:
		updates["hold"] = true
	case ReportStatusMonitoring:
		updates["monitoring_checked"] = true
		updates["monitoring_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	case ReportStatusPending:
		updates["monitoring_checked"] = false
		updates["monitoring_datetime"] = nil
		updates["hold"] = false
	case ReportStatusApproved:
		updates["processed"] = true
		updates["admin_approved"] = true
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	case ReportStatusDismissed:
		updates["processed"] = true
		updates["admin_approved"] = false
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	}

	// Add admin user to admin_users field
	if processedBy != "" && (status == ReportStatusApproved || status == ReportStatusDismissed) {
		updates["admin_users"] = processedBy
	}

	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateStatusApproved updates report to approved status with discipline_log_id in one atomic update
func (r *ReportRepository) UpdateStatusApproved(id int, processedBy string, disciplineLogID int) error {
	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"processed":          true,
			"admin_approved":     true,
			"admin_datetime":     gorm.Expr("NOW()"),
			"processed_datetime": gorm.Expr("NOW()"),
			"admin_users":        processedBy,
			"discipline_log_id":  disciplineLogID,
			"hold":               false,
		}).Error
}

// UpdateStatusWithVersion updates report status with optimistic locking (version check)
func (r *ReportRepository) UpdateStatusWithVersion(id int, status, processedBy string, currentVersion uint) error {
	updates := map[string]interface{}{
		"version": gorm.Expr("version + 1"),
	}

	switch status {
	case ReportStatusHold:
		updates["hold"] = true
	case ReportStatusMonitoring:
		updates["monitoring_checked"] = true
		updates["monitoring_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	case ReportStatusPending:
		updates["monitoring_checked"] = false
		updates["monitoring_datetime"] = nil
		updates["hold"] = false
	case ReportStatusApproved:
		updates["processed"] = true
		updates["admin_approved"] = true
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	case ReportStatusDismissed:
		updates["processed"] = true
		updates["admin_approved"] = false
		updates["admin_datetime"] = gorm.Expr("NOW()")
		updates["processed_datetime"] = gorm.Expr("NOW()")
		updates["hold"] = false
	}

	if processedBy != "" && (status == ReportStatusApproved || status == ReportStatusDismissed) {
		updates["admin_users"] = processedBy
	}

	result := r.db.Model(&domain.Report{}).
		Where("id = ? AND version = ?", id, currentVersion).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}

// UpdateStatusApprovedWithVersion updates report to approved with optimistic locking
func (r *ReportRepository) UpdateStatusApprovedWithVersion(id int, processedBy string, disciplineLogID int, currentVersion uint) error {
	// Validate JSON format before saving (CRITICAL: must be JSON array)
	if _, err := domain.ParseAdminUsers(processedBy); err != nil {
		return fmt.Errorf("invalid admin_users JSON format: %w", err)
	}

	result := r.db.Model(&domain.Report{}).
		Where("id = ? AND version = ?", id, currentVersion).
		Updates(map[string]interface{}{
			"processed":          true,
			"admin_approved":     true,
			"admin_datetime":     gorm.Expr("NOW()"),
			"processed_datetime": gorm.Expr("NOW()"),
			"admin_users":        processedBy,
			"discipline_log_id":  disciplineLogID,
			"version":            gorm.Expr("version + 1"),
			"hold":               false,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
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
		query = query.Where("processed = 0 AND monitoring_checked = 0 AND hold = 0")
	case ReportStatusMonitoring:
		query = query.Where("processed = 0 AND monitoring_checked = 1 AND hold = 0")
	case ReportStatusHold:
		query = query.Where("processed = 0 AND hold = 1")
	case ReportStatusApproved:
		query = query.Where("processed = 1 AND admin_approved = 1")
	case ReportStatusDismissed:
		query = query.Where("processed = 1 AND admin_approved = 0")
	}

	err := query.Count(&count).Error
	return count, err
}

// AggregatedReportRow holds a single row from the aggregated report query
type AggregatedReportRow struct {
	Table                       string `gorm:"column:sg_table"`
	SGID                        int    `gorm:"column:sg_id"`
	Parent                      int    `gorm:"column:sg_parent"`
	ReportCount                 int    `gorm:"column:report_count"`
	ReporterCount               int    `gorm:"column:reporter_count"`
	ReporterID                  string `gorm:"column:reporter_mb_id"`
	TargetID                    string `gorm:"column:target_mb_id"`
	TargetTitle                 string `gorm:"column:target_title"`
	TargetContent               string `gorm:"column:target_content"`
	ReportTypes                 string `gorm:"column:report_types"`
	FirstReportTime             string `gorm:"column:first_report_time"`
	LatestReportTime            string `gorm:"column:latest_report_time"`
	MonitoringChecked           int    `gorm:"column:monitoring_checked"`
	Hold                        int    `gorm:"column:hold"`
	AdminApproved               int    `gorm:"column:admin_approved"`
	Processed                   int    `gorm:"column:processed"`
	OpinionCount                int    `gorm:"column:opinion_count"`
	ActionCount                 int    `gorm:"column:action_count"`
	DismissCount                int    `gorm:"column:dismiss_count"`
	ReviewerIDs                 string `gorm:"column:reviewer_ids"`
	ReviewedByMe                int    `gorm:"column:reviewed_by_me"`
	AdminUsers                  string `gorm:"column:admin_users"`
	ProcessedDatetime           string `gorm:"column:processed_datetime"`
	MonitoringDisciplineReasons string `gorm:"column:monitoring_discipline_reasons"`
	MonitoringDisciplineDays    *int   `gorm:"column:monitoring_discipline_days"`
	MonitoringDisciplineType    string `gorm:"column:monitoring_discipline_type"`
	MonitoringDisciplineDetail  string `gorm:"column:monitoring_discipline_detail"`
}

// ListAggregated retrieves paginated aggregated reports grouped by (table, parent)
func (r *ReportRepository) ListAggregated(status string, page, limit int, fromDate, toDate, sort string, minOpinions int, excludeReviewer, requestingUserID string) ([]AggregatedReportRow, int64, error) {
	// Build HAVING clause using SELECT aliases (MySQL 8 supports aliases in HAVING)
	havingClause := ""

	// Support comma-separated statuses (e.g., "approved,scheduled")
	if strings.Contains(status, ",") {
		statuses := strings.Split(status, ",")
		var conditions []string
		for _, st := range statuses {
			st = strings.TrimSpace(st)
			switch st {
			case ReportStatusPending:
				conditions = append(conditions, "(opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0)")
			case ReportStatusMonitoring:
				// 진행중: 의견 있는 건 (needs_review/needs_final_approval 제외)
				conditions = append(conditions, "(opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0 AND NOT (action_count > 0 AND dismiss_count > 0) AND NOT (action_count >= 2 AND dismiss_count = 0))")
			case "needs_review":
				// 검토필요: 의견 갈림 (action과 dismiss 모두 존재)
				conditions = append(conditions, "(action_count > 0 AND dismiss_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0)")
			case "needs_final_approval":
				// 최종승인 대기: 조치 의견 2개 이상 일치, 최고관리자 승인 대기
				conditions = append(conditions, "(action_count >= 2 AND dismiss_count = 0 AND admin_approved = 0 AND processed = 0)")
			case ReportStatusHold:
				conditions = append(conditions, "(hold = 1 AND processed = 0)")
			case ReportStatusApproved:
				conditions = append(conditions, "(admin_approved = 1 AND processed = 1)")
			case "scheduled":
				// 예약대기: 최고관리자가 승인함, 크론 처리 대기 중
				conditions = append(conditions, "(admin_approved = 1 AND processed = 0)")
			case ReportStatusDismissed:
				conditions = append(conditions, "(admin_approved = 0 AND processed = 1)")
			}
		}
		if len(conditions) > 0 {
			havingClause = "HAVING (" + strings.Join(conditions, " OR ") + ")"
		}
	} else {
		// Single status
		switch status {
		case ReportStatusPending:
			havingClause = "HAVING opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
		case ReportStatusMonitoring:
			// 진행중: 의견 있는 건 (needs_review/needs_final_approval 제외)
			havingClause = "HAVING opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0 AND NOT (action_count > 0 AND dismiss_count > 0) AND NOT (action_count >= 2 AND dismiss_count = 0)"
		case "needs_review":
			// 검토필요: 의견 갈림 (action과 dismiss 모두 존재)
			havingClause = "HAVING action_count > 0 AND dismiss_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
		case "needs_final_approval":
			// 최종승인 대기: 조치 의견 2개 이상 일치, 최고관리자 승인 대기
			havingClause = "HAVING action_count >= 2 AND dismiss_count = 0 AND admin_approved = 0 AND processed = 0"
		case ReportStatusHold:
			havingClause = "HAVING hold = 1 AND processed = 0"
		case ReportStatusApproved:
			havingClause = "HAVING admin_approved = 1 AND processed = 1"
		case "scheduled":
			// 예약대기: 최고관리자가 승인함, 크론 처리 대기 중
			havingClause = "HAVING admin_approved = 1 AND processed = 0"
		case ReportStatusDismissed:
			havingClause = "HAVING admin_approved = 0 AND processed = 1"
		}
	}

	// min_opinions 필터: HAVING 절에 opinion_count >= N 추가
	if minOpinions > 0 {
		if havingClause == "" {
			havingClause = fmt.Sprintf("HAVING opinion_count >= %d", minOpinions)
		} else {
			havingClause += fmt.Sprintf(" AND opinion_count >= %d", minOpinions)
		}
	}

	// Date filter
	dateFilter := ""
	dateArgs := []interface{}{}
	if fromDate != "" {
		dateFilter += " AND s.sg_time >= ?"
		dateArgs = append(dateArgs, fromDate+" 00:00:00")
	}
	if toDate != "" {
		dateFilter += " AND s.sg_time <= ?"
		dateArgs = append(dateArgs, toDate+" 23:59:59")
	}

	// excludeReviewer filter: 이미 검토한 건 제외
	excludeFilter := ""
	excludeArgs := []interface{}{}
	if excludeReviewer != "" {
		excludeFilter = " AND NOT EXISTS (SELECT 1 FROM g5_na_singo_opinions exc WHERE exc.sg_table = s.sg_table AND exc.sg_id = s.sg_id AND exc.reviewer_id = ?)"
		excludeArgs = append(excludeArgs, excludeReviewer)
	}

	// Count query — inner SELECT must include all columns referenced by HAVING
	countArgs := append(dateArgs, excludeArgs...)
	countSQL := `
		SELECT COUNT(*) FROM (
			SELECT s.sg_table, s.sg_parent,
				MAX(s.admin_approved) as admin_approved,
				MAX(s.processed) as processed,
				MAX(s.hold) as hold,
				IFNULL(op.opinion_count, 0) as opinion_count,
				IFNULL(op.action_count, 0) as action_count,
				IFNULL(op.dismiss_count, 0) as dismiss_count
			FROM g5_na_singo s
			LEFT JOIN (
				SELECT o.sg_table, o.sg_parent,
					   COUNT(DISTINCT o.reviewer_id) as opinion_count,
					   COUNT(DISTINCT CASE WHEN o.opinion_type='action' THEN o.reviewer_id END) as action_count,
					   COUNT(DISTINCT CASE WHEN o.opinion_type='dismiss' THEN o.reviewer_id END) as dismiss_count
				FROM g5_na_singo_opinions o
				LEFT JOIN g5_member m ON o.reviewer_id = CAST(m.mb_no AS CHAR)
				LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
				WHERE su.mb_id IS NOT NULL
				GROUP BY o.sg_table, o.sg_parent
			) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
			WHERE 1=1` + dateFilter + excludeFilter + `
			GROUP BY s.sg_table, s.sg_id
			` + havingClause + `
		) t`

	var total int64
	if err := r.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// Dynamic ORDER BY based on sort parameter
	orderClause := "ORDER BY latest_report_time DESC" // newest (default)
	switch sort {
	case "oldest":
		orderClause = "ORDER BY first_report_time ASC"
	case "most_reported":
		orderClause = "ORDER BY report_count DESC, latest_report_time DESC"
	case "most_recent":
		orderClause = "ORDER BY latest_report_time DESC"
	}

	// reviewed_by_me: LEFT JOIN으로 현재 사용자 검토 여부 확인
	myReviewJoin := ""
	myReviewSelect := "0 as reviewed_by_me"
	myReviewArgs := []interface{}{}
	if requestingUserID != "" {
		myReviewJoin = ` LEFT JOIN (
			SELECT DISTINCT sg_table, sg_id
			FROM g5_na_singo_opinions
			WHERE reviewer_id = ?
		) my_review ON s.sg_table = my_review.sg_table AND s.sg_id = my_review.sg_id`
		myReviewSelect = "CASE WHEN my_review.sg_table IS NOT NULL THEN 1 ELSE 0 END as reviewed_by_me"
		myReviewArgs = append(myReviewArgs, requestingUserID)
	}

	// Main query
	offset := (page - 1) * limit
	mainSQL := `
		SELECT
			s.sg_table, s.sg_parent,
			MAX(s.sg_id) as sg_id,
			COUNT(*) as report_count,
			COUNT(DISTINCT s.mb_id) as reporter_count,
			MAX(s.mb_id) as reporter_mb_id,
			MAX(s.target_mb_id) as target_mb_id,
			MAX(s.target_title) as target_title,
			MAX(s.target_content) as target_content,
			GROUP_CONCAT(DISTINCT s.sg_type) as report_types,
			MIN(s.sg_time) as first_report_time,
			MAX(s.sg_time) as latest_report_time,
			MAX(s.monitoring_checked) as monitoring_checked,
			MAX(s.hold) as hold,
			MAX(s.admin_approved) as admin_approved,
			MAX(s.processed) as processed,
			MAX(s.admin_users) as admin_users,
			MAX(s.processed_datetime) as processed_datetime,
			MAX(s.monitoring_discipline_reasons) as monitoring_discipline_reasons,
			MAX(s.monitoring_discipline_days) as monitoring_discipline_days,
			MAX(s.monitoring_discipline_type) as monitoring_discipline_type,
			MAX(s.monitoring_discipline_detail) as monitoring_discipline_detail,
			IFNULL(op.opinion_count, 0) as opinion_count,
			IFNULL(op.action_count, 0) as action_count,
			IFNULL(op.dismiss_count, 0) as dismiss_count,
			IFNULL(op.reviewer_ids, '') as reviewer_ids,
			` + myReviewSelect + `
		FROM g5_na_singo s
		LEFT JOIN (
			SELECT o.sg_table, o.sg_parent,
				   COUNT(DISTINCT o.reviewer_id) as opinion_count,
				   COUNT(DISTINCT CASE WHEN o.opinion_type='action' THEN o.reviewer_id END) as action_count,
				   COUNT(DISTINCT CASE WHEN o.opinion_type='dismiss' THEN o.reviewer_id END) as dismiss_count,
				   GROUP_CONCAT(DISTINCT o.reviewer_id) as reviewer_ids
			FROM g5_na_singo_opinions o
			LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
			LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
			WHERE su.mb_id IS NOT NULL
			GROUP BY o.sg_table, o.sg_parent
		) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent` + myReviewJoin + `
		WHERE 1=1` + dateFilter + excludeFilter + `
		GROUP BY s.sg_table, s.sg_id
		` + havingClause + `
		` + orderClause + `
		LIMIT ? OFFSET ?`

	mainArgs := append(myReviewArgs, dateArgs...)
	mainArgs = append(mainArgs, excludeArgs...)
	mainArgs = append(mainArgs, limit, offset)
	var rows []AggregatedReportRow
	if err := r.db.Raw(mainSQL, mainArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// CountByStatusAggregated counts unique (table, parent) groups by status
func (r *ReportRepository) CountByStatusAggregated(status string) (int64, error) {
	havingClause := ""
	switch status {
	case ReportStatusPending:
		havingClause = "HAVING opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
	case ReportStatusMonitoring:
		havingClause = "HAVING opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
	case ReportStatusHold:
		havingClause = "HAVING hold = 1 AND processed = 0"
	case ReportStatusApproved:
		havingClause = "HAVING admin_approved = 1"
	case ReportStatusDismissed:
		havingClause = "HAVING admin_approved = 0 AND processed = 1"
	}

	sql := `
		SELECT COUNT(*) FROM (
			SELECT s.sg_table, s.sg_parent,
				MAX(s.admin_approved) as admin_approved,
				MAX(s.processed) as processed,
				MAX(s.hold) as hold,
				IFNULL(op.opinion_count, 0) as opinion_count
			FROM g5_na_singo s
			LEFT JOIN (
				SELECT o.sg_table, o.sg_parent, COUNT(*) as opinion_count
				FROM g5_na_singo_opinions o
				LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
				LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
				WHERE su.mb_id IS NOT NULL
				GROUP BY o.sg_table, o.sg_parent
			) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
			GROUP BY s.sg_table, s.sg_parent
			` + havingClause + `
		) t`

	var count int64
	if err := r.db.Raw(sql).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListAggregatedByTarget retrieves paginated reports grouped by target_mb_id (피신고자별 그룹핑)
func (r *ReportRepository) ListAggregatedByTarget(status string, page, limit int, fromDate, toDate, sort, excludeReviewer string) ([]domain.TargetAggregatedRow, int64, error) {
	// Build inner HAVING clause for status filter (same logic as ListAggregated but at content level)
	statusHaving := r.buildStatusHaving(status)

	// Date filter
	dateFilter := ""
	dateArgs := []interface{}{}
	if fromDate != "" {
		dateFilter += " AND s.sg_time >= ?"
		dateArgs = append(dateArgs, fromDate+" 00:00:00")
	}
	if toDate != "" {
		dateFilter += " AND s.sg_time <= ?"
		dateArgs = append(dateArgs, toDate+" 23:59:59")
	}

	// excludeReviewer filter
	excludeFilter := ""
	excludeArgs := []interface{}{}
	if excludeReviewer != "" {
		excludeFilter = " AND NOT EXISTS (SELECT 1 FROM g5_na_singo_opinions exc WHERE exc.sg_table = s.sg_table AND exc.sg_id = s.sg_id AND exc.reviewer_id = ?)"
		excludeArgs = append(excludeArgs, excludeReviewer)
	}

	// Count query — count distinct target_mb_id
	countArgs := append(dateArgs, excludeArgs...)
	countSQL := `
		SELECT COUNT(DISTINCT target_mb_id) FROM (
			SELECT s.target_mb_id, s.sg_table, s.sg_parent,
				MAX(s.admin_approved) as admin_approved,
				MAX(s.processed) as processed,
				MAX(s.hold) as hold,
				IFNULL(op.opinion_count, 0) as opinion_count
			FROM g5_na_singo s
			LEFT JOIN (
				SELECT o.sg_table, o.sg_parent, COUNT(*) as opinion_count
				FROM g5_na_singo_opinions o
				LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
				LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
				WHERE su.mb_id IS NOT NULL
				GROUP BY o.sg_table, o.sg_parent
			) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
			WHERE 1=1` + dateFilter + excludeFilter + `
			GROUP BY s.target_mb_id, s.sg_table, s.sg_parent
			` + statusHaving + `
		) t`

	var total int64
	if err := r.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// Dynamic ORDER BY
	orderClause := "ORDER BY latest_report_time DESC"
	switch sort {
	case "oldest":
		orderClause = "ORDER BY first_report_time ASC"
	case "most_reported":
		orderClause = "ORDER BY report_count DESC, latest_report_time DESC"
	case "most_recent":
		orderClause = "ORDER BY latest_report_time DESC"
	}

	// Main query — aggregate at target_mb_id level from content-level filtered results
	offset := (page - 1) * limit
	mainSQL := `
		SELECT
			target_mb_id,
			SUM(report_count) as report_count,
			COUNT(*) as content_count,
			SUM(reporter_count) as reporter_count,
			MAX(latest_report_time) as latest_report_time,
			MIN(first_report_time) as first_report_time
		FROM (
			SELECT
				s.target_mb_id,
				s.sg_table,
				s.sg_parent,
				COUNT(*) as report_count,
				COUNT(DISTINCT s.mb_id) as reporter_count,
				MIN(s.sg_time) as first_report_time,
				MAX(s.sg_time) as latest_report_time,
				MAX(s.admin_approved) as admin_approved,
				MAX(s.processed) as processed,
				MAX(s.hold) as hold,
				IFNULL(op.opinion_count, 0) as opinion_count
			FROM g5_na_singo s
			LEFT JOIN (
				SELECT o.sg_table, o.sg_parent, COUNT(*) as opinion_count
				FROM g5_na_singo_opinions o
				LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
				LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
				WHERE su.mb_id IS NOT NULL
				GROUP BY o.sg_table, o.sg_parent
			) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
			WHERE 1=1` + dateFilter + excludeFilter + `
			GROUP BY s.target_mb_id, s.sg_table, s.sg_parent
			` + statusHaving + `
		) filtered
		GROUP BY target_mb_id
		` + orderClause + `
		LIMIT ? OFFSET ?`

	mainArgs := append(dateArgs, excludeArgs...)
	mainArgs = append(mainArgs, limit, offset)
	var rows []domain.TargetAggregatedRow
	if err := r.db.Raw(mainSQL, mainArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// ListAggregatedByTargetIDs retrieves content-level aggregated reports for specific target user IDs
// Used for expanding a target user's sub-contents in the target grouping view
func (r *ReportRepository) ListAggregatedByTargetIDs(targetIDs []string, status string, fromDate, toDate string) ([]AggregatedReportRow, error) {
	if len(targetIDs) == 0 {
		return nil, nil
	}

	statusHaving := r.buildStatusHaving(status)

	// Date filter
	dateFilter := ""
	dateArgs := []interface{}{targetIDs}
	if fromDate != "" {
		dateFilter += " AND s.sg_time >= ?"
		dateArgs = append(dateArgs, fromDate+" 00:00:00")
	}
	if toDate != "" {
		dateFilter += " AND s.sg_time <= ?"
		dateArgs = append(dateArgs, toDate+" 23:59:59")
	}

	mainSQL := `
		SELECT
			s.sg_table,
			MAX(s.sg_id) as sg_id,
			MAX(s.sg_parent) as sg_parent,
			COUNT(*) as report_count,
			COUNT(DISTINCT s.mb_id) as reporter_count,
			MAX(s.mb_id) as reporter_mb_id,
			MAX(s.target_mb_id) as target_mb_id,
			MAX(s.target_title) as target_title,
			MAX(s.target_content) as target_content,
			GROUP_CONCAT(DISTINCT s.sg_type) as report_types,
			MIN(s.sg_time) as first_report_time,
			MAX(s.sg_time) as latest_report_time,
			MAX(s.monitoring_checked) as monitoring_checked,
			MAX(s.hold) as hold,
			MAX(s.admin_approved) as admin_approved,
			MAX(s.processed) as processed,
			MAX(s.admin_users) as admin_users,
			MAX(s.processed_datetime) as processed_datetime,
			IFNULL(op.opinion_count, 0) as opinion_count,
			IFNULL(op.action_count, 0) as action_count,
			IFNULL(op.dismiss_count, 0) as dismiss_count,
			IFNULL(op.reviewer_ids, '') as reviewer_ids
		FROM g5_na_singo s
		LEFT JOIN (
			SELECT o.sg_table, o.sg_parent,
				   COUNT(DISTINCT o.reviewer_id) as opinion_count,
				   COUNT(DISTINCT CASE WHEN o.opinion_type='action' THEN o.reviewer_id END) as action_count,
				   COUNT(DISTINCT CASE WHEN o.opinion_type='dismiss' THEN o.reviewer_id END) as dismiss_count,
				   GROUP_CONCAT(DISTINCT o.reviewer_id) as reviewer_ids
			FROM g5_na_singo_opinions o
			LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
			LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
			WHERE su.mb_id IS NOT NULL
			GROUP BY o.sg_table, o.sg_parent
		) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
		WHERE s.target_mb_id IN (?)` + dateFilter + `
		GROUP BY s.sg_table, s.sg_id
		` + statusHaving + `
		ORDER BY MAX(s.sg_time) DESC`

	var rows []AggregatedReportRow
	if err := r.db.Raw(mainSQL, dateArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

// buildStatusHaving generates HAVING clause for status filtering (shared between methods)
func (r *ReportRepository) buildStatusHaving(status string) string {
	// Support comma-separated statuses (e.g., "approved,scheduled")
	if strings.Contains(status, ",") {
		statuses := strings.Split(status, ",")
		var conditions []string
		for _, st := range statuses {
			st = strings.TrimSpace(st)
			switch st {
			case ReportStatusPending:
				conditions = append(conditions, "(opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0)")
			case ReportStatusMonitoring:
				conditions = append(conditions, "(opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0)")
			case "needs_final_approval":
				conditions = append(conditions, "(action_count >= 2 AND dismiss_count = 0 AND admin_approved = 0 AND processed = 0)")
			case ReportStatusHold:
				conditions = append(conditions, "(hold = 1 AND processed = 0)")
			case ReportStatusApproved:
				conditions = append(conditions, "(admin_approved = 1 AND processed = 1)")
			case "scheduled":
				conditions = append(conditions, "(admin_approved = 1 AND processed = 0)")
			case ReportStatusDismissed:
				conditions = append(conditions, "(admin_approved = 0 AND processed = 1)")
			}
		}
		if len(conditions) > 0 {
			return "HAVING (" + strings.Join(conditions, " OR ") + ")"
		}
		return ""
	}

	// Single status
	switch status {
	case ReportStatusPending:
		return "HAVING opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
	case ReportStatusMonitoring:
		return "HAVING opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0"
	case "needs_final_approval":
		return "HAVING action_count >= 2 AND dismiss_count = 0 AND admin_approved = 0 AND processed = 0"
	case ReportStatusHold:
		return "HAVING hold = 1 AND processed = 0"
	case ReportStatusApproved:
		return "HAVING admin_approved = 1 AND processed = 1"
	case "scheduled":
		return "HAVING admin_approved = 1 AND processed = 0"
	case ReportStatusDismissed:
		return "HAVING admin_approved = 0 AND processed = 1"
	default:
		return ""
	}
}

// UpdateStatusScheduledApprove sets admin_approved=1, processed=0 with admin_discipline_* fields
// for PHP cron to process later
func (r *ReportRepository) UpdateStatusScheduledApprove(id int, processedBy, reasonsJSON string, days int, disciplineType, detail string) error {
	// Validate JSON format before saving (CRITICAL: must be JSON array)
	if _, err := domain.ParseAdminUsers(processedBy); err != nil {
		return fmt.Errorf("invalid admin_users JSON format: %w", err)
	}

	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"admin_approved":           true,
			"processed":                false,
			"admin_datetime":           gorm.Expr("NOW()"),
			"admin_users":              processedBy,
			"hold":                     false,
			"admin_discipline_reasons": reasonsJSON,
			"admin_discipline_days":    days,
			"admin_discipline_type":    disciplineType,
			"admin_discipline_detail":  detail,
		}).Error
}

// ClearAdminDisciplineFields resets admin_discipline_* fields (used when reverting)
func (r *ReportRepository) ClearAdminDisciplineFields(id int) error {
	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"admin_discipline_reasons": "",
			"admin_discipline_days":    0,
			"admin_discipline_type":    "",
			"admin_discipline_detail":  "",
		}).Error
}

// UpdateMonitoringRecommendation updates monitoring_discipline_* fields for scheduled reports
// (does NOT set admin_approved, only stores recommendation for super_admin review)
func (r *ReportRepository) UpdateMonitoringRecommendation(id int, reasonsJSON string, days int, disciplineType, detail string) error {
	return r.db.Model(&domain.Report{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"monitoring_discipline_reasons": reasonsJSON,
			"monitoring_discipline_days":    days,
			"monitoring_discipline_type":    disciplineType,
			"monitoring_discipline_detail":  detail,
		}).Error
}

// GetAllStatusCounts retrieves all status counts in a single query (replaces 5+ CountByStatusAggregated calls)
func (r *ReportRepository) GetAllStatusCounts() (map[string]int64, error) {
	sql := `
		SELECT
			SUM(CASE WHEN opinion_count = 0 AND admin_approved = 0 AND processed = 0 AND hold = 0 THEN 1 ELSE 0 END) as pending_count,
			SUM(CASE WHEN opinion_count > 0 AND admin_approved = 0 AND processed = 0 AND hold = 0
				AND NOT (action_count > 0 AND dismiss_count > 0)
				AND NOT (action_count >= 2 AND dismiss_count = 0)
				THEN 1 ELSE 0 END) as monitoring_count,
			SUM(CASE WHEN hold = 1 AND processed = 0 THEN 1 ELSE 0 END) as hold_count,
			SUM(CASE WHEN admin_approved = 1 THEN 1 ELSE 0 END) as approved_count,
			SUM(CASE WHEN admin_approved = 0 AND processed = 1 THEN 1 ELSE 0 END) as dismissed_count,
			SUM(CASE WHEN action_count > 0 AND dismiss_count > 0
				AND admin_approved = 0 AND processed = 0 AND hold = 0 THEN 1 ELSE 0 END) as needs_review_count,
			SUM(CASE WHEN action_count >= 2 AND dismiss_count = 0
				AND admin_approved = 0 AND processed = 0 AND hold = 0 THEN 1 ELSE 0 END) as needs_final_approval_count,
			COUNT(*) as total_count
		FROM (
			SELECT s.sg_table, s.sg_parent,
				MAX(s.admin_approved) as admin_approved,
				MAX(s.processed) as processed,
				MAX(s.hold) as hold,
				IFNULL(op.opinion_count, 0) as opinion_count,
				IFNULL(op.action_count, 0) as action_count,
				IFNULL(op.dismiss_count, 0) as dismiss_count
			FROM g5_na_singo s
			LEFT JOIN (
				SELECT o.sg_table, o.sg_parent,
					COUNT(*) as opinion_count,
					SUM(CASE WHEN o.opinion_type = 'action' THEN 1 ELSE 0 END) as action_count,
					SUM(CASE WHEN o.opinion_type = 'dismiss' THEN 1 ELSE 0 END) as dismiss_count
				FROM g5_na_singo_opinions o
				LEFT JOIN g5_member m ON o.reviewer_id = m.mb_no
				LEFT JOIN singo_users su ON m.mb_id COLLATE utf8mb4_unicode_ci = su.mb_id COLLATE utf8mb4_unicode_ci
				WHERE su.mb_id IS NOT NULL
				GROUP BY o.sg_table, o.sg_parent
			) op ON s.sg_table=op.sg_table AND s.sg_parent=op.sg_parent
			GROUP BY s.sg_table, s.sg_parent
		) t`

	var result struct {
		PendingCount            int64 `gorm:"column:pending_count"`
		MonitoringCount         int64 `gorm:"column:monitoring_count"`
		HoldCount               int64 `gorm:"column:hold_count"`
		ApprovedCount           int64 `gorm:"column:approved_count"`
		DismissedCount          int64 `gorm:"column:dismissed_count"`
		NeedsReviewCount        int64 `gorm:"column:needs_review_count"`
		NeedsFinalApprovalCount int64 `gorm:"column:needs_final_approval_count"`
		TotalCount              int64 `gorm:"column:total_count"`
	}
	if err := r.db.Raw(sql).Scan(&result).Error; err != nil {
		return nil, err
	}

	stats := map[string]int64{
		"pending":              result.PendingCount,
		"monitoring":           result.MonitoringCount,
		"hold":                 result.HoldCount,
		"approved":             result.ApprovedCount,
		"dismissed":            result.DismissedCount,
		"needs_review":         result.NeedsReviewCount,
		"needs_final_approval": result.NeedsFinalApprovalCount,
		"total":                result.TotalCount,
	}
	return stats, nil
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

// CountDistinctReporters counts unique reporters for a specific post (Phase 6-1: 자동 잠금)
func (r *ReportRepository) CountDistinctReporters(table string, sgID int) (int64, error) {
	var count int64
	err := r.db.Model(&domain.Report{}).
		Where("sg_table = ? AND sg_id = ? AND sg_flag = 0", table, sgID).
		Distinct("mb_id").
		Count(&count).Error
	return count, err
}

// UpdatePostLockField updates wr_7 field in write_* and g5_board_new tables (Phase 6-1: 자동 잠금)
func (r *ReportRepository) UpdatePostLockField(table string, sgID int, value interface{}) error {
	writeTable := "write_" + table

	// Update write_* table
	if err := r.db.Table(writeTable).
		Where("wr_id = ?", sgID).
		Update("wr_7", value).Error; err != nil {
		return err
	}

	// Update g5_board_new table (최신글 목록)
	r.db.Table("g5_board_new").
		Where("bo_table = ? AND wr_id = ?", table, sgID).
		Update("wr_singo", value)

	return nil
}

// GetAdjacentReport retrieves the adjacent report (previous or next) based on created_at timestamp
// Supports filtering by status, date range, and search
func (r *ReportRepository) GetAdjacentReport(table string, sgID int, direction, status, sort, fromDate, toDate, search string) (*domain.Report, error) {
	// First, get the current report's created_at timestamp
	var currentReport domain.Report
	if err := r.db.Where("sg_table = ? AND sg_id = ?", table, sgID).First(&currentReport).Error; err != nil {
		return nil, errors.New("현재 신고를 찾을 수 없습니다")
	}

	query := r.db.Model(&domain.Report{})

	// Apply the same status filter as List
	switch status {
	case ReportStatusPending:
		query = query.Where("processed = 0 AND monitoring_checked = 0 AND hold = 0")
	case ReportStatusMonitoring:
		query = query.Where("processed = 0 AND monitoring_checked = 1 AND hold = 0")
	case ReportStatusHold:
		query = query.Where("processed = 0 AND hold = 1")
	case ReportStatusApproved:
		query = query.Where("processed = 1 AND admin_approved = 1")
	case ReportStatusDismissed:
		query = query.Where("processed = 1 AND admin_approved = 0")
	}

	// Apply date range filter
	if fromDate != "" {
		query = query.Where("sg_time >= ?", fromDate+" 00:00:00")
	}
	if toDate != "" {
		query = query.Where("sg_time <= ?", toDate+" 23:59:59")
	}

	// Apply search filter (search in target_content, target_title, target_mb_id)
	if search != "" {
		query = query.Where("target_content LIKE ? OR target_title LIKE ? OR target_mb_id LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// Apply direction filter and ordering
	var orderBy string
	if sort == "oldest" {
		// oldest 정렬 (created_at ASC)
		if direction == "prev" {
			// 이전 건: created_at < current
			query = query.Where("sg_time < ?", currentReport.CreatedAt)
			orderBy = "sg_time DESC" // 가장 가까운 이전 건
		} else {
			// 다음 건: created_at > current
			query = query.Where("sg_time > ?", currentReport.CreatedAt)
			orderBy = "sg_time ASC" // 가장 가까운 다음 건
		}
	} else {
		// newest 정렬 (created_at DESC) - 기본값
		if direction == "prev" {
			// 이전 건: created_at > current (더 최근)
			query = query.Where("sg_time > ?", currentReport.CreatedAt)
			orderBy = "sg_time ASC" // 가장 가까운 이전 건
		} else {
			// 다음 건: created_at < current (더 오래된)
			query = query.Where("sg_time < ?", currentReport.CreatedAt)
			orderBy = "sg_time DESC" // 가장 가까운 다음 건
		}
	}

	var adjacentReport domain.Report
	if err := query.Order(orderBy).Limit(1).First(&adjacentReport).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("인접 신고를 찾을 수 없습니다")
		}
		return nil, err
	}

	return &adjacentReport, nil
}
