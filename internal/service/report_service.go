package service

import (
	"errors"
	"fmt"
	"log"
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
	repo           *repository.ReportRepository
	disciplineRepo *repository.DisciplineRepository
	memoRepo       *repository.G5MemoRepository
	memberRepo     repository.MemberRepository
}

// NewReportService creates a new ReportService
func NewReportService(
	repo *repository.ReportRepository,
	disciplineRepo *repository.DisciplineRepository,
	memoRepo *repository.G5MemoRepository,
	memberRepo repository.MemberRepository,
) *ReportService {
	return &ReportService{
		repo:           repo,
		disciplineRepo: disciplineRepo,
		memoRepo:       memoRepo,
		memberRepo:     memberRepo,
	}
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

// findReport looks up a report by ID (frontend) or by table+parent (legacy)
func (s *ReportService) findReport(req *domain.ReportActionRequest) (*domain.Report, error) {
	// Frontend sends "id" (report primary key)
	if req.ReportID > 0 {
		return s.repo.GetByID(req.ReportID)
	}
	// Legacy: table + parent lookup
	if req.Table != "" && req.Parent > 0 {
		return s.repo.GetByTableAndParent(req.Table, req.Parent)
	}
	return nil, ErrReportNotFound
}

// Process processes a report action
func (s *ReportService) Process(adminID, clientIP string, req *domain.ReportActionRequest) error {
	// Validate action
	validActions := map[string]bool{
		"submitOpinion": true,
		"cancelOpinion": true,
		"adminApprove":  true,
		"adminDismiss":  true,
		"adminHold":     true,
	}

	if !validActions[req.Action] {
		return ErrInvalidAction
	}

	// Get report
	report, err := s.findReport(req)
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
	switch req.Action {
	case "submitOpinion":
		return s.repo.UpdateStatus(report.ID, ReportStatusMonitoring, adminID)
	case "cancelOpinion":
		return s.repo.UpdateStatus(report.ID, ReportStatusPending, adminID)
	case "adminApprove":
		return s.processApprove(report, adminID, clientIP, req)
	case "adminDismiss":
		return s.repo.UpdateStatus(report.ID, ReportStatusDismissed, adminID)
	case "adminHold":
		return s.repo.UpdateStatus(report.ID, ReportStatusMonitoring, adminID)
	default:
		return ErrInvalidAction
	}
}

// processApprove handles the full approval flow:
// 1. Create discipline log entry
// 2. Update report status + discipline_log_id
// 3. Apply member restrictions (level, intercept_date)
// 4. Send memo to target member
func (s *ReportService) processApprove(report *domain.Report, adminID, clientIP string, req *domain.ReportActionRequest) error {
	// Look up admin member (for display name)
	adminMember, err := s.memberRepo.FindByUserID(adminID)
	if err != nil {
		return fmt.Errorf("관리자 정보를 찾을 수 없습니다: %w", err)
	}
	adminName := adminMember.Nickname
	if adminName == "" {
		adminName = adminID
	}

	// Look up target member
	var targetMember *domain.Member
	if report.TargetID != "" {
		targetMember, err = s.memberRepo.FindByUserID(report.TargetID)
		if err != nil {
			log.Printf("[WARN] 피신고 회원 조회 실패 (mb_id=%s): %v", report.TargetID, err)
			// 회원을 찾을 수 없어도 승인 처리는 계속 진행
		}
	}

	targetNickname := ""
	if targetMember != nil {
		targetNickname = targetMember.Nickname
	}

	// Build discipline log content (JSON)
	content := &domain.DisciplineLogContent{
		TargetID:       report.TargetID,
		TargetNickname: targetNickname,
		PenaltyDays:    req.PenaltyDays,
		PenaltyType:    req.PenaltyType,
		PenaltyReasons: req.PenaltyReasons,
		AdminMemo:      req.AdminMemo,
		ReportID:       report.ID,
		ReportTable:    report.Table,
		TargetContent:  report.TargetContent,
		TargetTitle:    report.TargetTitle,
		ProcessedAt:    time.Now().Format("2006-01-02 15:04:05"),
		ProcessedBy:    adminID,
	}

	// Ensure non-nil slices for JSON
	if content.PenaltyType == nil {
		content.PenaltyType = []string{}
	}
	if content.PenaltyReasons == nil {
		content.PenaltyReasons = []string{}
	}

	// Step 1: Create discipline log entry
	disciplineLogID, err := s.disciplineRepo.CreateDisciplineLog(
		adminID,
		adminName,
		report.TargetID,
		targetNickname,
		content,
		report.ID,
		report.Table,
		"admin_approve",
		clientIP,
	)
	if err != nil {
		return fmt.Errorf("징계 내역 생성 실패: %w", err)
	}

	// Step 2: Update report status to approved + discipline_log_id
	if err := s.repo.UpdateStatusApproved(report.ID, adminID, disciplineLogID); err != nil {
		return fmt.Errorf("신고 상태 업데이트 실패: %w", err)
	}

	// Step 3: Apply member restrictions (best-effort — 실패해도 승인은 완료)
	if targetMember != nil && len(req.PenaltyType) > 0 {
		fields := make(map[string]interface{})

		for _, pt := range req.PenaltyType {
			switch pt {
			case "level":
				fields["mb_level"] = 1 // 등급 1로 하향
			case "intercept":
				if req.PenaltyDays == 9999 {
					fields["mb_intercept_date"] = "9999-12-31"
				} else if req.PenaltyDays > 0 {
					interceptDate := time.Now().AddDate(0, 0, req.PenaltyDays).Format("2006-01-02")
					fields["mb_intercept_date"] = interceptDate
				}
			}
		}

		if len(fields) > 0 {
			if err := s.memberRepo.UpdateFields(targetMember.ID, fields); err != nil {
				log.Printf("[ERROR] 회원 제재 적용 실패 (mb_id=%s): %v (승인은 완료됨)", report.TargetID, err)
			} else {
				log.Printf("[INFO] 회원 제재 적용 완료: mb_id=%s, fields=%v", report.TargetID, fields)
			}
		}
	}

	// Step 4: Send memo to target member (best-effort)
	if targetMember != nil && len(req.PenaltyType) > 0 {
		memo := buildDisciplineMemo(targetNickname, report.TargetID, disciplineLogID)
		if err := s.memoRepo.SendMemo(report.TargetID, "police", memo, clientIP); err != nil {
			log.Printf("[ERROR] 쪽지 발송 실패 (mb_id=%s): %v (승인은 완료됨)", report.TargetID, err)
		} else {
			log.Printf("[INFO] 제재 쪽지 발송 완료: mb_id=%s", report.TargetID)
		}
	}

	log.Printf("[INFO] 신고 승인 처리 완료: report_id=%d, discipline_log_id=%d, admin=%s", report.ID, disciplineLogID, adminID)
	return nil
}

// buildDisciplineMemo generates the memo text sent to the penalized member
// Replicates the template from ang-gnu/extend/da_user_member.memo.txt
func buildDisciplineMemo(targetNick, targetID string, disciplineLogID int) string {
	disciplineLink := fmt.Sprintf(
		"https://damoang.net/disciplinelog?bo_table=disciplinelog&sca=&sfl=wr_subject%%7C%%7Cwr_content,1&sop=and&stx=%s",
		targetID,
	)

	return fmt.Sprintf(`💌 [잠시 쉬어가기 안내] 💌


안녕하세요, %s님! 👋

잠깐! 우리 %s님께서
조금 쉬어가실 시간이 필요하신 것 같아요 🍀

다모앙 가족 모두가 행복한 공간을 만들기 위해
잠시만 충전의 시간을 가져보시는 건 어떨까요?

곧 다시 만나요! 🌈

📝 쉬어가기 상세 내용
• 내 기록 확인: %s

━━━━━━━━━━━━━━━━━━━━━━━━━━
📚 도움이 될 만한 페이지
• 이용약관: https://damoang.net/content/provision
• 운영정책: https://damoang.net/content/operation_policy
• 제재사유 안내: https://damoang.net/content/operation_policy_add
• 내 기록 확인: %s
━━━━━━━━━━━━━━━━━━━━━━━━━━
💡 잠시만 기다려주세요!
   이 기간 동안은 글쓰기, 댓글, 쪽지 기능이
   잠시 쉬어갑니다 😊

🌟 함께 더 좋은 커뮤니티를 만들어가요!
   서로를 배려하는 마음, 그것이 다모앙의 힘입니다 💪`, targetNick, targetNick, disciplineLink, disciplineLink)
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
