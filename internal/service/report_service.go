package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// truncateUTF8 truncates string to maxLen runes, appending "…" if truncated
func truncateUTF8(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "…"
}

const (
	ReportStatusPending    = "pending"
	ReportStatusMonitoring = "monitoring"
	ReportStatusHold       = "hold"
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
	repo            *repository.ReportRepository
	opinionRepo     *repository.OpinionRepository
	historyRepo     *repository.HistoryRepository
	disciplineRepo  *repository.DisciplineRepository
	aiEvaluationRepo *repository.AIEvaluationRepository // Phase 2: 통합 API용
	memoRepo        *repository.G5MemoRepository
	memberRepo      repository.MemberRepository
	boardRepo       *repository.BoardRepository
	singoUserRepo   *repository.SingoUserRepository

	// singoUserRepo.FindAll() cache (5분 TTL)
	singoUsersMu     sync.RWMutex
	singoUsersCache  []domain.SingoUser
	singoUsersCacheT time.Time
}

// NewReportService creates a new ReportService
func NewReportService(
	repo *repository.ReportRepository,
	disciplineRepo *repository.DisciplineRepository,
	memoRepo *repository.G5MemoRepository,
	memberRepo repository.MemberRepository,
	boardRepo ...*repository.BoardRepository,
) *ReportService {
	s := &ReportService{
		repo:           repo,
		disciplineRepo: disciplineRepo,
		memoRepo:       memoRepo,
		memberRepo:     memberRepo,
	}
	if len(boardRepo) > 0 {
		s.boardRepo = boardRepo[0]
	}
	return s
}

// SetOpinionRepo sets the opinion repository (optional dependency)
func (s *ReportService) SetOpinionRepo(opinionRepo *repository.OpinionRepository) {
	s.opinionRepo = opinionRepo
}

// SetHistoryRepo sets the history repository (optional dependency)
func (s *ReportService) SetHistoryRepo(historyRepo *repository.HistoryRepository) {
	s.historyRepo = historyRepo
}

// SetSingoUserRepo sets the singo user repository (optional dependency)
func (s *ReportService) SetSingoUserRepo(singoUserRepo *repository.SingoUserRepository) {
	s.singoUserRepo = singoUserRepo
}

// SetAIEvaluationRepo sets the AI evaluation repository (Phase 2: 통합 API용)
func (s *ReportService) SetAIEvaluationRepo(aiEvaluationRepo *repository.AIEvaluationRepository) {
	s.aiEvaluationRepo = aiEvaluationRepo
}

// getTotalReviewerCount returns the cached count of all singo users (5-minute TTL)
func (s *ReportService) getTotalReviewerCount() int {
	if s.singoUserRepo == nil {
		return 0
	}

	s.singoUsersMu.RLock()
	if s.singoUsersCache != nil && time.Since(s.singoUsersCacheT) < 5*time.Minute {
		count := len(s.singoUsersCache)
		s.singoUsersMu.RUnlock()
		return count
	}
	s.singoUsersMu.RUnlock()

	s.singoUsersMu.Lock()
	defer s.singoUsersMu.Unlock()

	// Double-check after acquiring write lock
	if s.singoUsersCache != nil && time.Since(s.singoUsersCacheT) < 5*time.Minute {
		return len(s.singoUsersCache)
	}

	users, err := s.singoUserRepo.FindAll()
	if err != nil {
		log.Printf("[WARN] singoUserRepo.FindAll 실패: %v", err)
		if s.singoUsersCache != nil {
			return len(s.singoUsersCache) // stale cache
		}
		return 0
	}
	s.singoUsersCache = users
	s.singoUsersCacheT = time.Now()
	return len(users)
}

// recordHistory records a status change in the history table (best-effort)
func (s *ReportService) recordHistory(table string, sgID, parent int, prevStatus, newStatus, adminID, note string) {
	if s.historyRepo == nil {
		return
	}
	if err := s.historyRepo.Record(table, sgID, parent, prevStatus, newStatus, adminID, note); err != nil {
		log.Printf("[WARN] 이력 기록 실패: table=%s, parent=%d, %s→%s: %v", table, parent, prevStatus, newStatus, err)
	}
}

// List retrieves paginated aggregated reports grouped by (table, parent)
func (s *ReportService) List(status string, page, limit int, fromDate, toDate, sort, singoRole string, minOpinions int, excludeReviewer, requestingUserID string) ([]domain.AggregatedReportResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	rows, total, err := s.repo.ListAggregated(status, page, limit, fromDate, toDate, sort, minOpinions, excludeReviewer, requestingUserID)
	if err != nil {
		return nil, 0, err
	}

	// Batch-load nicknames and board names
	userIDs := make(map[string]bool)
	boardIDs := make(map[string]bool)
	for _, r := range rows {
		if r.TargetID != "" {
			userIDs[r.TargetID] = true
		}
		if r.Table != "" {
			boardIDs[r.Table] = true
		}
	}

	ids := make([]string, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	nickMap, _ := s.memberRepo.FindNicksByIDs(ids)
	if nickMap == nil {
		nickMap = map[string]string{}
	}

	boardNameMap := make(map[string]string)
	if s.boardRepo != nil {
		boardIDList := make([]string, 0, len(boardIDs))
		for id := range boardIDs {
			boardIDList = append(boardIDList, id)
		}
		if names, err := s.boardRepo.FindByIDs(boardIDList); err == nil {
			boardNameMap = names
		}
	}

	// Get total reviewer count (cached)
	totalReviewers := s.getTotalReviewerCount()

	// Convert to response format
	responses := make([]domain.AggregatedReportResponse, len(rows))
	for i, row := range rows {
		// Compute status from aggregated flags
		rowStatus := "pending"
		if row.AdminApproved == 1 && row.Processed == 1 {
			rowStatus = "approved"
		} else if row.Processed == 1 {
			rowStatus = "dismissed"
		} else if row.AdminApproved == 1 && row.Processed == 0 {
			rowStatus = "scheduled"
		} else if row.Hold == 1 {
			rowStatus = "hold"
		} else if row.OpinionCount > 0 {
			rowStatus = "monitoring"
		}

		// Parse reviewer IDs from comma-separated string
		var reviewerIDList []string
		if row.ReviewerIDs != "" {
			reviewerIDList = strings.Split(row.ReviewerIDs, ",")
		}

		resp := domain.AggregatedReportResponse{
			Table:             row.Table,
			SGID:              row.SGID,
			Parent:            row.Parent,
			ReportCount:       row.ReportCount,
			ReporterCount:     row.ReporterCount,
			TargetID:          row.TargetID,
			TargetNickname:    nickMap[row.TargetID],
			TargetTitle:       truncateUTF8(row.TargetTitle, 50),
			TargetContent:     truncateUTF8(row.TargetContent, 100),
			BoardSubject:      boardNameMap[row.Table],
			ReportTypes:       row.ReportTypes,
			OpinionCount:      row.OpinionCount,
			ActionCount:       row.ActionCount,
			DismissCount:      row.DismissCount,
			Status:            rowStatus,
			FirstReportTime:   row.FirstReportTime,
			LatestReportTime:  row.LatestReportTime,
			ReviewedCount:     len(reviewerIDList),
			TotalReviewers:    totalReviewers,
			ReviewedByMe:      row.ReviewedByMe == 1,
			AdminUsers:        row.AdminUsers,
			ProcessedDatetime: row.ProcessedDatetime,
		}

		// super_admin만 실제 reviewer_ids 포함
		if singoRole == "super_admin" {
			resp.ReviewerIDs = reviewerIDList
		}

		responses[i] = resp
	}

	return responses, total, nil
}

// ListByTarget retrieves paginated reports grouped by target user (피신고자별 그룹핑)
func (s *ReportService) ListByTarget(status string, page, limit int, fromDate, toDate, sort, singoRole, excludeReviewer string) ([]domain.TargetAggregatedResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	rows, total, err := s.repo.ListAggregatedByTarget(status, page, limit, fromDate, toDate, sort, excludeReviewer)
	if err != nil {
		return nil, 0, err
	}

	// Batch-load nicknames
	userIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.TargetID != "" {
			userIDs = append(userIDs, r.TargetID)
		}
	}
	nickMap, _ := s.memberRepo.FindNicksByIDs(userIDs)
	if nickMap == nil {
		nickMap = map[string]string{}
	}

	// Get sub-contents for each target user
	contentRows, err := s.repo.ListAggregatedByTargetIDs(userIDs, status, fromDate, toDate)
	if err != nil {
		return nil, 0, err
	}

	// Batch-load board names for sub-contents
	boardIDs := make(map[string]bool)
	for _, cr := range contentRows {
		if cr.Table != "" {
			boardIDs[cr.Table] = true
		}
	}
	boardNameMap := make(map[string]string)
	if s.boardRepo != nil {
		boardIDList := make([]string, 0, len(boardIDs))
		for id := range boardIDs {
			boardIDList = append(boardIDList, id)
		}
		if names, err := s.boardRepo.FindByIDs(boardIDList); err == nil {
			boardNameMap = names
		}
	}

	// Get total reviewer count (cached)
	totalReviewers := s.getTotalReviewerCount()

	// Batch-load discipline counts for all target users
	disciplineCountMap := make(map[string]int)
	if s.disciplineRepo != nil && len(userIDs) > 0 {
		if counts, err := s.disciplineRepo.CountByTargetMemberIDs(userIDs); err == nil {
			disciplineCountMap = counts
		}
	}

	// Batch-load opinions for all sub-contents (1 query)
	opinionsMap := make(map[string][]domain.Opinion)
	if s.opinionRepo != nil && len(contentRows) > 0 {
		keys := make([]struct{ Table string; Parent int }, 0, len(contentRows))
		for _, cr := range contentRows {
			keys = append(keys, struct{ Table string; Parent int }{cr.Table, cr.Parent})
		}
		if opMap, err := s.opinionRepo.GetByMultipleReportsGrouped(keys); err == nil {
			opinionsMap = opMap
		}
	}

	// Batch-load reviewer nicknames for opinions
	reviewerIDSet := make(map[string]bool)
	for _, ops := range opinionsMap {
		for _, op := range ops {
			if op.ReviewerID != "" {
				reviewerIDSet[op.ReviewerID] = true
			}
		}
	}
	reviewerNickMap := make(map[string]string)
	if len(reviewerIDSet) > 0 {
		reviewerIDs := make([]string, 0, len(reviewerIDSet))
		for id := range reviewerIDSet {
			reviewerIDs = append(reviewerIDs, id)
		}
		if nicks, err := s.memberRepo.FindNicksByIDs(reviewerIDs); err == nil && nicks != nil {
			reviewerNickMap = nicks
		}
	}

	// Group sub-contents by target_mb_id
	contentsByTarget := make(map[string][]domain.AggregatedReportResponse)
	for _, cr := range contentRows {
		rowStatus := computeRowStatus(cr.AdminApproved, cr.Processed, cr.Hold, cr.OpinionCount)

		var reviewerIDList []string
		if cr.ReviewerIDs != "" {
			reviewerIDList = strings.Split(cr.ReviewerIDs, ",")
		}

		resp := domain.AggregatedReportResponse{
			Table:             cr.Table,
			SGID:              cr.SGID,
			Parent:            cr.Parent,
			ReportCount:       cr.ReportCount,
			ReporterCount:     cr.ReporterCount,
			TargetID:          cr.TargetID,
			TargetNickname:    nickMap[cr.TargetID],
			TargetTitle:       truncateUTF8(cr.TargetTitle, 50),
			TargetContent:     truncateUTF8(cr.TargetContent, 100),
			BoardSubject:      boardNameMap[cr.Table],
			ReportTypes:       cr.ReportTypes,
			OpinionCount:      cr.OpinionCount,
			ActionCount:       cr.ActionCount,
			DismissCount:      cr.DismissCount,
			Status:            rowStatus,
			FirstReportTime:   cr.FirstReportTime,
			LatestReportTime:  cr.LatestReportTime,
			ReviewedCount:     len(reviewerIDList),
			TotalReviewers:    totalReviewers,
			AdminUsers:        cr.AdminUsers,
			ProcessedDatetime: cr.ProcessedDatetime,
		}
		if singoRole == "super_admin" {
			resp.ReviewerIDs = reviewerIDList
		}

		// Attach opinions for this content
		opKey := fmt.Sprintf("%s:%d", cr.Table, cr.Parent)
		if ops, ok := opinionsMap[opKey]; ok && len(ops) > 0 {
			opResponses := make([]domain.OpinionResponse, 0, len(ops))
			for _, op := range ops {
				// 닉네임을 찾을 수 없는 경우 (탈퇴한 사용자 등) 기본값 설정
				reviewerNick := reviewerNickMap[op.ReviewerID]
				if reviewerNick == "" {
					reviewerNick = "(알 수 없음)"
				}
				opResponses = append(opResponses, domain.OpinionResponse{
					ReviewerID:   op.ReviewerID,
					ReviewerNick: reviewerNick,
					OpinionType:  op.OpinionType,
					Reasons:      op.DisciplineReasons,
					Days:         op.DisciplineDays,
					Type:         op.DisciplineType,
					Detail:       op.DisciplineDetail,
					CreatedAt:    op.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			resp.Opinions = opResponses
		}

		contentsByTarget[cr.TargetID] = append(contentsByTarget[cr.TargetID], resp)
	}

	// Build response
	responses := make([]domain.TargetAggregatedResponse, len(rows))
	for i, row := range rows {
		responses[i] = domain.TargetAggregatedResponse{
			TargetID:         row.TargetID,
			TargetNickname:   nickMap[row.TargetID],
			ReportCount:      row.ReportCount,
			ContentCount:     row.ContentCount,
			ReporterCount:    row.ReporterCount,
			LatestReportTime: row.LatestReportTime,
			FirstReportTime:  row.FirstReportTime,
			DisciplineCount:  disciplineCountMap[row.TargetID],
			Contents:         contentsByTarget[row.TargetID],
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

// GetData retrieves report data by table and parent/sgID with all related reports
// requestingUserID/singoRole: 닉네임 마스킹용 (빈 문자열이면 super_admin으로 간주)
// sgID: 특정 신고 ID (0이면 parent만 사용하여 가장 최근 신고 조회)
func (s *ReportService) GetData(table string, parent int, requestingUserID, singoRole string, sgID ...int) (*domain.ReportDetailResponse, error) {
	var primaryReport *domain.Report
	var err error

	// sg_id 기준 조회 (새 방식)
	if len(sgID) > 0 && sgID[0] > 0 {
		primaryReport, err = s.repo.GetByTableAndSgID(table, sgID[0], parent)
	} else {
		// parent 기준 조회 (레거시 호환)
		primaryReport, err = s.repo.GetByTableAndParent(table, parent)
	}

	if err != nil {
		return nil, ErrReportNotFound
	}

	// Get all reports for this content
	allReports, err := s.repo.GetAllByTableAndParent(table, primaryReport.Parent)
	if err != nil {
		allReports = []domain.Report{*primaryReport}
	}

	// Batch-load nicknames
	userIDs := make(map[string]bool)
	for _, r := range allReports {
		if r.ReporterID != "" {
			userIDs[r.ReporterID] = true
		}
		if r.TargetID != "" {
			userIDs[r.TargetID] = true
		}
	}
	ids := make([]string, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	nickMap, _ := s.memberRepo.FindNicksByIDs(ids)
	if nickMap == nil {
		nickMap = map[string]string{}
	}

	// Batch-load board name
	boardNameMap := make(map[string]string)
	if s.boardRepo != nil {
		if names, err := s.boardRepo.FindByIDs([]string{table}); err == nil {
			boardNameMap = names
		}
	}

	// Compute aggregate status
	status := computeAggregateStatus(allReports)

	// Build primary report response
	primary := toReportListResponse(primaryReport, nickMap, boardNameMap)

	// Build all reports responses
	allResponses := make([]domain.ReportListResponse, len(allReports))
	for i, r := range allReports {
		allResponses[i] = toReportListResponse(&r, nickMap, boardNameMap)
	}

	// Load opinions from opinions table
	// Use primaryReport.Parent instead of passed parent (which might be 0)
	opinions, _ := s.GetOpinions(table, primaryReport.SGID, primaryReport.Parent, requestingUserID, singoRole)

	// Build process result for processed reports
	var processResult *domain.ProcessResultResponse
	if status == ReportStatusApproved || status == ReportStatusDismissed {
		processResult = s.buildProcessResult(primaryReport, allReports)
	}

	return &domain.ReportDetailResponse{
		Report:        primary,
		AllReports:    allResponses,
		Opinions:      opinions,
		Status:        status,
		ProcessResult: processResult,
	}, nil
}

// GetDataEnhanced retrieves report data with optional includes (Phase 2: 통합 API)
// includes: "ai" (AI 평가), "history" (징계 이력)
// Example: ?include=ai,history
// sgID: 특정 신고 ID (0이면 parent만 사용)
func (s *ReportService) GetDataEnhanced(table string, parent int, requestingUserID, singoRole string, includes []string, sgID ...int) (*domain.ReportDetailEnhancedResponse, error) {
	// 1. 기본 데이터 조회 (기존 GetData 호출)
	detail, err := s.GetData(table, parent, requestingUserID, singoRole, sgID...)
	if err != nil {
		return nil, err
	}

	// 2. Enhanced response 구성
	enhanced := &domain.ReportDetailEnhancedResponse{
		ReportDetailResponse: *detail,
	}

	// 3. 옵셔널 데이터 조회 (includes 파라미터 기반)
	// Use detail.Report.Parent instead of passed parent (which might be 0)
	actualParent := detail.Report.Parent
	for _, include := range includes {
		switch include {
		case "ai":
			// AI 평가 목록 조회
			if s.aiEvaluationRepo != nil {
				if aiEvals, err := s.aiEvaluationRepo.ListByReport(table, actualParent); err == nil {
					enhanced.AIEvaluations = aiEvals
				}
			}

		case "history":
			// 징계 이력 조회 (피신고자 기준)
			if s.disciplineRepo != nil && detail.Report.TargetID != "" {
				// 페이지네이션 없이 최근 10건만 조회
				if history, _, err := s.disciplineRepo.FindByTargetMember(detail.Report.TargetID, 1, 10); err == nil {
					enhanced.DisciplineHistory = history
				}
			}
		}
	}

	return enhanced, nil
}

// buildProcessResult constructs ProcessResultResponse from the discipline log
// for already-processed reports (approved or dismissed)
func (s *ReportService) buildProcessResult(primary *domain.Report, allReports []domain.Report) *domain.ProcessResultResponse {
	result := &domain.ProcessResultResponse{}

	// Use primary report's admin_users and processed_datetime
	result.AdminUsers = primary.AdminUsers
	if primary.ProcessedDatetime != nil {
		result.ProcessedDatetime = primary.ProcessedDatetime.Format("2006-01-02 15:04:05")
	}

	// Find discipline_log_id from any of the reports
	var disciplineLogID int
	for _, r := range allReports {
		if r.DisciplineLogID != nil && *r.DisciplineLogID > 0 {
			disciplineLogID = *r.DisciplineLogID
			break
		}
	}
	if disciplineLogID == 0 && primary.DisciplineLogID != nil {
		disciplineLogID = *primary.DisciplineLogID
	}

	result.DisciplineLogID = disciplineLogID

	// If we have a discipline log ID, fetch the full content from the log
	if disciplineLogID > 0 && s.disciplineRepo != nil {
		logEntry, err := s.disciplineRepo.GetByID(disciplineLogID)
		if err != nil {
			log.Printf("[WARN] 징계 로그 조회 실패 (id=%d): %v", disciplineLogID, err)
		} else {
			// Parse the JSON content from wr_content
			var content domain.DisciplineLogContent
			if err := json.Unmarshal([]byte(logEntry.Content), &content); err != nil {
				log.Printf("[WARN] 징계 로그 내용 파싱 실패 (id=%d): %v", disciplineLogID, err)
			} else {
				result.PenaltyDays = content.PenaltyPeriod
				result.PenaltyType = content.PenaltyType
				result.PenaltyReasons = content.PenaltyReasons
				result.SgTypes = content.SgTypes
				result.IsBulk = content.IsBulk
				result.ReportCount = content.ReportCount
				result.AdminMemo = content.AdminMemo
			}
		}
	}

	// Fallback: use admin_discipline_* fields from the report itself if discipline log wasn't found
	if disciplineLogID == 0 {
		if primary.AdminDisciplineDays != 0 {
			result.PenaltyDays = primary.AdminDisciplineDays
		}
		if primary.AdminDisciplineType != "" {
			result.PenaltyType = strings.Split(primary.AdminDisciplineType, ",")
		}
		if primary.AdminDisciplineReasons != "" {
			result.AdminMemo = primary.AdminDisciplineDetail
			// Try to parse reasons JSON: "[21,22]"
			var sgTypes []int
			if err := json.Unmarshal([]byte(primary.AdminDisciplineReasons), &sgTypes); err == nil {
				result.SgTypes = sgTypes
			}
		}
	}

	return result
}

// toReportListResponse converts a Report to ReportListResponse
func toReportListResponse(report *domain.Report, nickMap, boardNameMap map[string]string) domain.ReportListResponse {
	resp := domain.ReportListResponse{
		ID:               report.ID,
		Table:            report.Table,
		Parent:           report.Parent,
		Type:             report.Type,
		BoardSubject:     boardNameMap[report.Table],
		ReporterID:       report.ReporterID,
		ReporterNickname: nickMap[report.ReporterID],
		TargetID:         report.TargetID,
		TargetNickname:   nickMap[report.TargetID],
		TargetTitle:      report.TargetTitle,
		TargetContent:    report.TargetContent,
		Reason:           reportReasonOrType(report),
		Status:           report.Status(),
		CreatedAt:        report.CreatedAt.Format("2006-01-02 15:04:05"),
		AdminUsers:       report.AdminUsers,
	}
	if report.ProcessedDatetime != nil {
		resp.ProcessedDatetime = report.ProcessedDatetime.Format("2006-01-02 15:04:05")
	}
	return resp
}

// reportReasonOrType returns sg_desc if non-empty, otherwise sg_type as string
func reportReasonOrType(r *domain.Report) string {
	if strings.TrimSpace(r.Reason) != "" {
		return r.Reason
	}
	if r.Type > 0 {
		return fmt.Sprintf("%d", r.Type)
	}
	return ""
}

// computeAggregateStatus computes aggregate status from all reports for the same content
func computeAggregateStatus(reports []domain.Report) string {
	hasApproved := false
	hasProcessed := false
	hasHold := false
	hasMonitoring := false

	for _, r := range reports {
		if r.AdminApproved {
			hasApproved = true
		}
		if r.Processed {
			hasProcessed = true
		}
		if r.Hold {
			hasHold = true
		}
		if r.MonitoringChecked {
			hasMonitoring = true
		}
	}

	if hasApproved && hasProcessed {
		return "approved"
	}
	if hasProcessed {
		return "dismissed"
	}
	if hasApproved && !hasProcessed {
		return "scheduled"
	}
	if hasHold {
		return "hold"
	}
	if hasMonitoring {
		return "monitoring"
	}
	return "pending"
}

// computeRowStatus derives status string from aggregated flag values
func computeRowStatus(adminApproved, processed, hold, opinionCount int) string {
	if adminApproved == 1 && processed == 1 {
		return "approved"
	}
	if processed == 1 {
		return "dismissed"
	}
	if adminApproved == 1 && processed == 0 {
		return "scheduled"
	}
	if hold == 1 {
		return "hold"
	}
	if opinionCount > 0 {
		return "monitoring"
	}
	return "pending"
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
		"submitOpinion":      true,
		"cancelOpinion":      true,
		"adminApprove":       true,
		"adminDismiss":       true,
		"adminHold":          true,
		"revertToPending":    true,
		"revertToMonitoring": true,
	}

	if !validActions[req.Action] {
		return ErrInvalidAction
	}

	// Get report
	report, err := s.findReport(req)
	if err != nil {
		return ErrReportNotFound
	}

	// Check if already processed (for admin actions, not for reverts)
	currentStatus := report.Status()
	if (req.Action == "adminApprove" || req.Action == "adminDismiss") &&
		(currentStatus == ReportStatusApproved || currentStatus == ReportStatusDismissed) {
		return ErrAlreadyProcessed
	}

	// Revert actions allowed on processed, hold, or scheduled reports
	if (req.Action == "revertToPending" || req.Action == "revertToMonitoring") &&
		currentStatus != ReportStatusApproved && currentStatus != ReportStatusDismissed && currentStatus != ReportStatusHold && currentStatus != "scheduled" {
		return ErrInvalidAction
	}

	// Process based on action
	switch req.Action {
	case "submitOpinion":
		return s.processSubmitOpinion(report, adminID, req)
	case "cancelOpinion":
		return s.processCancelOpinion(report, adminID)
	case "adminApprove":
		return s.processApprove(report, adminID, clientIP, req)
	case "adminDismiss":
		return s.processAdminDismiss(report, adminID)
	case "adminHold":
		return s.repo.UpdateStatus(report.ID, ReportStatusHold, adminID)
	case "revertToPending":
		return s.revertToPending(report, adminID)
	case "revertToMonitoring":
		return s.revertToMonitoring(report, adminID)
	default:
		return ErrInvalidAction
	}
}

// processApprove handles the approval flow.
// If immediate=true: full execution (discipline log + restrict + memo).
// If immediate=false (default): scheduled execution (save admin_discipline_* fields for PHP cron).
func (s *ReportService) processApprove(report *domain.Report, adminID, clientIP string, req *domain.ReportActionRequest) error {
	if !req.Immediate {
		return s.processScheduledApprove(report, adminID, req)
	}
	return s.processImmediateApprove(report, adminID, clientIP, req)
}

// processScheduledApprove sets admin_discipline_* fields and marks admin_approved=1, processed=0.
// PHP cron (매시 정각) processes these records. Cancellable until cron execution.
func (s *ReportService) processScheduledApprove(report *domain.Report, adminID string, req *domain.ReportActionRequest) error {
	// 검증: 승인 시 필수 항목 확인
	if req.PenaltyDays <= 0 {
		return fmt.Errorf("승인 시 이용제한 일수는 필수입니다 (1일 이상 또는 9999=영구)")
	}
	if len(req.PenaltyReasons) == 0 {
		return fmt.Errorf("승인 시 제재 사유는 필수입니다")
	}

	// Convert penalty_reasons to JSON integer array for PHP compatibility
	sgTypes := make([]int, 0, len(req.PenaltyReasons))
	for _, reasonKey := range req.PenaltyReasons {
		if code, ok := domain.ReasonKeyToCode[reasonKey]; ok {
			sgTypes = append(sgTypes, code)
		}
	}

	// Build reasons JSON string (e.g., "[21,22,23]")
	reasonsJSON := "[]"
	if len(sgTypes) > 0 {
		parts := make([]string, len(sgTypes))
		for i, code := range sgTypes {
			parts[i] = fmt.Sprintf("%d", code)
		}
		reasonsJSON = "[" + strings.Join(parts, ",") + "]"
	}

	// Convert penalty_type to string (e.g., "level,access")
	phpPenaltyType := make([]string, 0, len(req.PenaltyType))
	for _, pt := range req.PenaltyType {
		if pt == "intercept" {
			phpPenaltyType = append(phpPenaltyType, "access")
		} else {
			phpPenaltyType = append(phpPenaltyType, pt)
		}
	}
	disciplineType := strings.Join(phpPenaltyType, ",")

	// Convert penalty_days: 9999 → -1 for PHP
	penaltyDays := req.PenaltyDays
	if penaltyDays >= 9999 {
		penaltyDays = -1
	}

	// Update all reports for the same content
	allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
	for _, r := range allReports {
		if !r.Processed {
			// Convert adminID to JSON array format
			adminUsersJSON, err := domain.AddAdminApproval(r.AdminUsers, adminID)
			if err != nil {
				log.Printf("[ERROR] admin_users JSON 생성 실패 (id=%d): %v", r.ID, err)
				continue
			}

			if err := s.repo.UpdateStatusScheduledApprove(r.ID, adminUsersJSON, reasonsJSON, penaltyDays, disciplineType, req.AdminMemo); err != nil {
				log.Printf("[WARN] 예약 승인 업데이트 실패 (id=%d): %v", r.ID, err)
			}
		}
	}

	s.recordHistory(report.Table, report.SGID, report.Parent, report.Status(), "scheduled", adminID, "예약 승인 (크론 처리 대기)")
	log.Printf("[INFO] 예약 승인 설정: report_id=%d, admin=%s, days=%d", report.ID, adminID, penaltyDays)
	return nil
}

// processImmediateApprove handles the full immediate approval flow:
// 1. Create discipline log entry
// 2. Update report status + discipline_log_id
// 3. Apply member restrictions (level, intercept_date)
// 4. Send memo to target member
func (s *ReportService) processImmediateApprove(report *domain.Report, adminID, clientIP string, req *domain.ReportActionRequest) error {
	// 검증: 승인 시 필수 항목 확인
	if req.PenaltyDays <= 0 {
		return fmt.Errorf("승인 시 이용제한 일수는 필수입니다 (1일 이상 또는 9999=영구)")
	}
	if len(req.PenaltyReasons) == 0 {
		return fmt.Errorf("승인 시 제재 사유는 필수입니다")
	}

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

	// Convert penalty_days: frontend sends 9999 for permanent, PHP expects -1
	penaltyPeriod := req.PenaltyDays
	if penaltyPeriod >= 9999 {
		penaltyPeriod = -1
	}

	// Convert string reason keys to integer codes (PHP SingoHelper 호환)
	sgTypes := make([]int, 0, len(req.PenaltyReasons))
	for _, reasonKey := range req.PenaltyReasons {
		if code, ok := domain.ReasonKeyToCode[reasonKey]; ok {
			sgTypes = append(sgTypes, code)
		}
	}

	// Convert penalty_type: Go uses "intercept", PHP uses "access"
	phpPenaltyType := make([]string, 0, len(req.PenaltyType))
	for _, pt := range req.PenaltyType {
		if pt == "intercept" {
			phpPenaltyType = append(phpPenaltyType, "access")
		} else {
			phpPenaltyType = append(phpPenaltyType, pt)
		}
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	// Build reported URL
	reportedURL := fmt.Sprintf("/%s/%d", report.Table, report.Parent)

	// Build discipline log content (PHP disciplinelog 스킨 호환 JSON)
	content := &domain.DisciplineLogContent{
		// PHP 필수 필드
		PenaltyMbID:     report.TargetID,
		PenaltyDateFrom: nowStr,
		PenaltyPeriod:   penaltyPeriod,
		PenaltyType:     phpPenaltyType,
		SgTypes:         sgTypes,
		ReportedItems:   []domain.ReportedItem{{Table: report.Table, ID: report.Parent, Parent: 0}},
		ReportedURL:     reportedURL,
		IsBulk:          false,
		ReportCount:     1,
		// Go 확장 필드
		TargetNickname: targetNickname,
		PenaltyReasons: req.PenaltyReasons,
		AdminMemo:      req.AdminMemo,
		ReportID:       report.ID,
		ReportTable:    report.Table,
		ProcessedBy:    adminID,
	}

	// Ensure non-nil slices for JSON
	if content.PenaltyType == nil {
		content.PenaltyType = []string{}
	}
	if content.SgTypes == nil {
		content.SgTypes = []int{}
	}
	if content.ReportedItems == nil {
		content.ReportedItems = []domain.ReportedItem{}
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

	// Step 2: Update report status to approved + discipline_log_id (with optimistic locking)
	// Convert adminID to JSON array format
	adminUsersJSON, err := domain.AddAdminApproval(report.AdminUsers, adminID)
	if err != nil {
		return fmt.Errorf("admin_users JSON 생성 실패: %w", err)
	}

	if err := s.repo.UpdateStatusApprovedWithVersion(report.ID, adminUsersJSON, disciplineLogID, report.Version); err != nil {
		if errors.Is(err, repository.ErrVersionConflict) {
			return repository.ErrVersionConflict
		}
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

	s.recordHistory(report.Table, report.SGID, report.Parent, report.Status(), ReportStatusApproved, adminID,
		fmt.Sprintf("관리자 승인 (discipline_log_id=%d)", disciplineLogID))

	log.Printf("[INFO] 신고 승인 처리 완료: report_id=%d, discipline_log_id=%d, admin=%s", report.ID, disciplineLogID, adminID)
	return nil
}

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Processed int
	Failed    int
	Errors    []string
}

// ProcessBatchImmediate handles immediate batch approval by grouping reports by targetID
// so that each target gets exactly one discipline log entry
func (s *ReportService) ProcessBatchImmediate(adminID, clientIP string, req *domain.BatchReportActionRequest) (*BatchResult, error) {
	result := &BatchResult{}

	// 1. Collect all reports for the given (table, parent) pairs
	type reportGroup struct {
		reports []*domain.Report
	}
	byTarget := make(map[string]*reportGroup)

	for i := range req.Tables {
		allReports, err := s.repo.GetAllByTableAndParent(req.Tables[i], req.Parents[i])
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%d: %v", req.Tables[i], req.Parents[i], err))
			continue
		}
		if len(allReports) == 0 {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%d: 신고를 찾을 수 없습니다", req.Tables[i], req.Parents[i]))
			continue
		}

		// Use first unprocessed report as representative, skip already processed
		var representative *domain.Report
		for j := range allReports {
			if !allReports[j].Processed {
				representative = &allReports[j]
				break
			}
		}
		if representative == nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%d: 이미 처리된 신고입니다", req.Tables[i], req.Parents[i]))
			continue
		}

		targetID := representative.TargetID
		if targetID == "" {
			targetID = "__unknown__"
		}

		if _, ok := byTarget[targetID]; !ok {
			byTarget[targetID] = &reportGroup{}
		}
		byTarget[targetID].reports = append(byTarget[targetID].reports, representative)
	}

	// 2. Process each target group
	for targetID, group := range byTarget {
		if err := s.processImmediateBulkApprove(adminID, clientIP, targetID, group.reports, req); err != nil {
			result.Failed += len(group.reports)
			result.Errors = append(result.Errors, fmt.Sprintf("target=%s: %v", targetID, err))
		} else {
			result.Processed += len(group.reports)
		}
	}

	return result, nil
}

// processImmediateBulkApprove handles immediate approval for multiple reports targeting the same user.
// Creates exactly one discipline log with all reported items, applies restrictions and sends memo once.
func (s *ReportService) processImmediateBulkApprove(
	adminID, clientIP string,
	targetID string,
	reports []*domain.Report,
	req *domain.BatchReportActionRequest,
) error {
	if len(reports) == 0 {
		return nil
	}

	// 검증: 승인 시 필수 항목 확인
	if req.PenaltyDays <= 0 {
		return fmt.Errorf("승인 시 이용제한 일수는 필수입니다 (1일 이상 또는 9999=영구)")
	}
	if len(req.PenaltyReasons) == 0 {
		return fmt.Errorf("승인 시 제재 사유는 필수입니다")
	}

	// Single report: delegate to existing method for simplicity
	if len(reports) == 1 {
		actionReq := &domain.ReportActionRequest{
			Action:         "adminApprove",
			Table:          reports[0].Table,
			Parent:         reports[0].Parent,
			AdminMemo:      req.AdminMemo,
			PenaltyDays:    req.PenaltyDays,
			PenaltyType:    req.PenaltyType,
			PenaltyReasons: req.PenaltyReasons,
			Immediate:      true,
		}
		return s.processImmediateApprove(reports[0], adminID, clientIP, actionReq)
	}

	// --- Bulk path: multiple reports for same target ---

	// 1. Admin/target member lookup (1번)
	adminMember, err := s.memberRepo.FindByUserID(adminID)
	if err != nil {
		return fmt.Errorf("관리자 정보를 찾을 수 없습니다: %w", err)
	}
	adminName := adminMember.Nickname
	if adminName == "" {
		adminName = adminID
	}

	var targetMember *domain.Member
	targetNickname := ""
	if targetID != "" && targetID != "__unknown__" {
		targetMember, err = s.memberRepo.FindByUserID(targetID)
		if err != nil {
			log.Printf("[WARN] 피신고 회원 조회 실패 (mb_id=%s): %v", targetID, err)
		}
		if targetMember != nil {
			targetNickname = targetMember.Nickname
		}
	}

	// 2. Convert penalty fields (same as processImmediateApprove)
	penaltyPeriod := req.PenaltyDays
	if penaltyPeriod >= 9999 {
		penaltyPeriod = -1
	}

	sgTypes := make([]int, 0, len(req.PenaltyReasons))
	for _, reasonKey := range req.PenaltyReasons {
		if code, ok := domain.ReasonKeyToCode[reasonKey]; ok {
			sgTypes = append(sgTypes, code)
		}
	}

	phpPenaltyType := make([]string, 0, len(req.PenaltyType))
	for _, pt := range req.PenaltyType {
		if pt == "intercept" {
			phpPenaltyType = append(phpPenaltyType, "access")
		} else {
			phpPenaltyType = append(phpPenaltyType, pt)
		}
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	// 3. Build ReportedItems array (all reports)
	reportedItems := make([]domain.ReportedItem, 0, len(reports))
	for _, r := range reports {
		reportedItems = append(reportedItems, domain.ReportedItem{
			Table:  r.Table,
			ID:     r.Parent,
			Parent: 0,
		})
	}

	// Representative URL (first item)
	reportedURL := fmt.Sprintf("/%s/%d", reports[0].Table, reports[0].Parent)

	// 4. DisciplineLogContent with IsBulk=true, ReportCount=len
	content := &domain.DisciplineLogContent{
		PenaltyMbID:     targetID,
		PenaltyDateFrom: nowStr,
		PenaltyPeriod:   penaltyPeriod,
		PenaltyType:     phpPenaltyType,
		SgTypes:         sgTypes,
		ReportedItems:   reportedItems,
		ReportedURL:     reportedURL,
		IsBulk:          true,
		ReportCount:     len(reports),
		TargetNickname:  targetNickname,
		PenaltyReasons:  req.PenaltyReasons,
		AdminMemo:       req.AdminMemo,
		ReportID:        reports[0].ID,
		ReportTable:     reports[0].Table,
		ProcessedBy:     adminID,
	}

	// Ensure non-nil slices for JSON
	if content.PenaltyType == nil {
		content.PenaltyType = []string{}
	}
	if content.SgTypes == nil {
		content.SgTypes = []int{}
	}
	if content.PenaltyReasons == nil {
		content.PenaltyReasons = []string{}
	}

	// 5. CreateDisciplineLog (1번)
	disciplineLogID, err := s.disciplineRepo.CreateDisciplineLog(
		adminID,
		adminName,
		targetID,
		targetNickname,
		content,
		reports[0].ID,
		reports[0].Table,
		"admin_approve",
		clientIP,
	)
	if err != nil {
		return fmt.Errorf("징계 내역 생성 실패: %w", err)
	}

	// 6. UpdateStatusApprovedWithVersion for each report (+ all sub-reports)
	for _, report := range reports {
		// Update the representative report
		// Convert adminID to JSON array format
		adminUsersJSON, err := domain.AddAdminApproval(report.AdminUsers, adminID)
		if err != nil {
			log.Printf("[ERROR] admin_users JSON 생성 실패 (id=%d): %v", report.ID, err)
			continue
		}

		if err := s.repo.UpdateStatusApprovedWithVersion(report.ID, adminUsersJSON, disciplineLogID, report.Version); err != nil {
			log.Printf("[WARN] 벌크 승인 상태 업데이트 실패 (id=%d): %v", report.ID, err)
		}

		// Also update other reports for the same content
		allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
		for _, r := range allReports {
			if r.ID != report.ID && !r.Processed {
				subAdminUsersJSON, err := domain.AddAdminApproval(r.AdminUsers, adminID)
				if err != nil {
					log.Printf("[ERROR] admin_users JSON 생성 실패 (id=%d): %v", r.ID, err)
					continue
				}

				if err := s.repo.UpdateStatusApprovedWithVersion(r.ID, subAdminUsersJSON, disciplineLogID, r.Version); err != nil {
					log.Printf("[WARN] 벌크 승인 관련 신고 업데이트 실패 (id=%d): %v", r.ID, err)
				}
			}
		}

		s.recordHistory(report.Table, report.SGID, report.Parent, report.Status(), ReportStatusApproved, adminID,
			fmt.Sprintf("벌크 관리자 승인 (discipline_log_id=%d, %d건 묶음)", disciplineLogID, len(reports)))
	}

	// 7. Apply member restrictions (1번)
	if targetMember != nil && len(req.PenaltyType) > 0 {
		fields := make(map[string]interface{})
		for _, pt := range req.PenaltyType {
			switch pt {
			case "level":
				fields["mb_level"] = 1
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
				log.Printf("[ERROR] 벌크 회원 제재 적용 실패 (mb_id=%s): %v", targetID, err)
			} else {
				log.Printf("[INFO] 벌크 회원 제재 적용 완료: mb_id=%s, fields=%v", targetID, fields)
			}
		}
	}

	// 8. Send memo (1번)
	if targetMember != nil && len(req.PenaltyType) > 0 {
		memo := buildDisciplineMemo(targetNickname, targetID, disciplineLogID)
		if err := s.memoRepo.SendMemo(targetID, "police", memo, clientIP); err != nil {
			log.Printf("[ERROR] 벌크 쪽지 발송 실패 (mb_id=%s): %v", targetID, err)
		} else {
			log.Printf("[INFO] 벌크 제재 쪽지 발송 완료: mb_id=%s", targetID)
		}
	}

	log.Printf("[INFO] 벌크 신고 승인 처리 완료: target=%s, discipline_log_id=%d, reports=%d건, admin=%s",
		targetID, disciplineLogID, len(reports), adminID)
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

// processSubmitOpinion saves an opinion to the opinions table and checks for auto-approval/dismiss
func (s *ReportService) processSubmitOpinion(report *domain.Report, adminID string, req *domain.ReportActionRequest) error {
	// If opinions table is available, use it
	if s.opinionRepo != nil {
		// Determine opinion type from request
		opinionType := "action"
		if req.Type == "dismiss" || req.Type == "no_action" {
			opinionType = "dismiss"
		}

		// Build reasons string from penalty_reasons
		reasons := ""
		if len(req.PenaltyReasons) > 0 {
			reasons = strings.Join(req.PenaltyReasons, ",")
		} else if len(req.Reasons) > 0 {
			reasons = strings.Join(req.Reasons, ",")
		}

		disciplineDays := req.Days
		if req.PenaltyDays > 0 {
			disciplineDays = req.PenaltyDays
		}

		disciplineType := req.Type
		if len(req.PenaltyType) > 0 {
			disciplineType = strings.Join(req.PenaltyType, ",")
		}

		opinion := &domain.Opinion{
			Table:             report.Table,
			SGID:              report.SGID,
			Parent:            report.Parent,
			ReviewerID:        adminID,
			OpinionType:       opinionType,
			DisciplineReasons: reasons,
			DisciplineDays:    disciplineDays,
			DisciplineType:    disciplineType,
			DisciplineDetail:  req.Detail,
		}

		if err := s.opinionRepo.Save(opinion); err != nil {
			return fmt.Errorf("의견 저장 실패: %w", err)
		}

		// Update report status to monitoring
		if err := s.repo.UpdateStatus(report.ID, ReportStatusMonitoring, adminID); err != nil {
			return err
		}

		// Also update all other reports for the same content
		allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
		for _, r := range allReports {
			if r.ID != report.ID && !r.Processed {
				_ = s.repo.UpdateStatus(r.ID, ReportStatusMonitoring, adminID)
			}
		}

		// Record history
		s.recordHistory(report.Table, report.SGID, report.Parent, report.Status(), ReportStatusMonitoring, adminID, "의견 제출")

		// Check for auto-approval or auto-dismiss
		if err := s.checkAutoApproval(report); err != nil {
			log.Printf("[WARN] 자동 승인 체크 실패: %v", err)
		}
		if err := s.checkAutoDismiss(report); err != nil {
			log.Printf("[WARN] 자동 기각 체크 실패: %v", err)
		}

		return nil
	}

	// Fallback: simple status update (legacy behavior)
	return s.repo.UpdateStatus(report.ID, ReportStatusMonitoring, adminID)
}

// processCancelOpinion removes an opinion and recalculates status
func (s *ReportService) processCancelOpinion(report *domain.Report, adminID string) error {
	if s.opinionRepo != nil {
		// Delete this reviewer's opinion
		if err := s.opinionRepo.Delete(report.Table, report.SGID, report.Parent, adminID); err != nil {
			log.Printf("[WARN] 의견 삭제 실패: %v", err)
		}

		// Check if any opinions remain
		actionCount, dismissCount, err := s.opinionRepo.CountByReportGrouped(report.Table, report.Parent)
		if err != nil {
			log.Printf("[WARN] 의견 카운트 실패: %v", err)
		}

		if actionCount == 0 && dismissCount == 0 {
			// No opinions left — revert to pending
			allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
			for _, r := range allReports {
				if !r.Processed {
					_ = s.repo.UpdateStatus(r.ID, ReportStatusPending, adminID)
				}
			}
			return nil
		}

		// Still has opinions — keep monitoring
		return nil
	}

	// Fallback: simple status update
	return s.repo.UpdateStatus(report.ID, ReportStatusPending, adminID)
}

// processAdminDismiss handles admin dismissal and updates all related reports
func (s *ReportService) processAdminDismiss(report *domain.Report, adminID string) error {
	prevStatus := report.Status()
	// Dismiss all reports for the same content
	allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
	for _, r := range allReports {
		if !r.Processed {
			if err := s.repo.UpdateStatus(r.ID, ReportStatusDismissed, adminID); err != nil {
				log.Printf("[WARN] 신고 기각 업데이트 실패 (id=%d): %v", r.ID, err)
			}
		}
	}
	s.recordHistory(report.Table, report.SGID, report.Parent, prevStatus, ReportStatusDismissed, adminID, "관리자 기각")
	return nil
}

// checkAutoApproval checks if 2+ action opinions match on reasons+days, logging for admin review
// NOTE: 자동 승인 기능 제거됨. 최고 관리자가 직접 승인해야 함.
func (s *ReportService) checkAutoApproval(report *domain.Report) error {
	if s.opinionRepo == nil {
		return nil
	}

	opinions, err := s.opinionRepo.GetMatchingActionOpinions(report.Table, report.Parent)
	if err != nil || len(opinions) < 2 {
		return nil
	}

	// Check if at least 2 action opinions have matching reasons and days
	type opKey struct {
		Reasons string
		Days    int
	}
	counts := make(map[opKey]int)
	var matchedKey opKey
	for _, op := range opinions {
		key := opKey{Reasons: op.DisciplineReasons, Days: op.DisciplineDays}
		counts[key]++
		if counts[key] >= 2 {
			matchedKey = key
		}
	}

	for _, count := range counts {
		if count >= 2 {
			// 변경: 자동 승인 제거, 로그만 남김
			log.Printf("[INFO] ⚠️  승인 가능 상태: table=%s, parent=%d (action 의견 %d건 일치, 사유=%s, 일수=%d) - 최고 관리자 승인 대기",
				report.Table, report.Parent, count, matchedKey.Reasons, matchedKey.Days)

			// TODO: 향후 알림 시스템 추가 시 최고 관리자에게 알림 전송
			// 예: s.notificationService.NotifyAdminForApproval(report, count)

			return nil // 자동 승인 안 함
		}
	}

	return nil
}

// checkAutoDismiss checks if 2+ dismiss opinions exist, triggering auto-dismiss
func (s *ReportService) checkAutoDismiss(report *domain.Report) error {
	if s.opinionRepo == nil {
		return nil
	}

	_, dismissCount, err := s.opinionRepo.CountByReportGrouped(report.Table, report.Parent)
	if err != nil || dismissCount < 2 {
		return nil
	}

	// Auto-dismiss: update all related reports
	log.Printf("[INFO] 자동 기각: table=%s, parent=%d (dismiss 의견 %d건)", report.Table, report.Parent, dismissCount)
	allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
	for _, r := range allReports {
		if !r.Processed {
			_ = s.repo.UpdateStatus(r.ID, ReportStatusDismissed, "system")
		}
	}

	return nil
}

// revertToPending reverts all related reports to pending or monitoring state
// If opinions exist, reverts to monitoring (keeps opinions)
// If no opinions, reverts to pending (deletes any stray opinions)
func (s *ReportService) revertToPending(report *domain.Report, adminID string) error {
	prevStatus := report.Status()

	// Check if opinions exist
	opinionCount := 0
	if s.opinionRepo != nil {
		opinions, _ := s.opinionRepo.GetByReportGrouped(report.Table, report.Parent)
		opinionCount = len(opinions)
	}

	// If opinions exist, revert to monitoring (keep opinions)
	if opinionCount > 0 {
		log.Printf("[INFO] 의견이 있어 모니터링으로 되돌림: table=%s, parent=%d, opinions=%d", report.Table, report.Parent, opinionCount)
		return s.revertToMonitoring(report, adminID)
	}

	// No opinions: delete any stray opinions and revert to pending
	if s.opinionRepo != nil {
		_ = s.opinionRepo.DeleteByReportGrouped(report.Table, report.Parent)
	}

	// Revert all related reports to pending + clear admin_discipline_* fields
	allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
	for _, r := range allReports {
		_ = s.repo.UpdateStatus(r.ID, ReportStatusPending, adminID)
		_ = s.repo.ClearAdminDisciplineFields(r.ID)
	}

	s.recordHistory(report.Table, report.SGID, report.Parent, prevStatus, ReportStatusPending, adminID, "대기로 되돌리기")
	log.Printf("[INFO] 신고 되돌리기(대기): table=%s, parent=%d, admin=%s", report.Table, report.Parent, adminID)
	return nil
}

// revertToMonitoring reverts all related reports to monitoring state, keeping opinions
func (s *ReportService) revertToMonitoring(report *domain.Report, adminID string) error {
	prevStatus := report.Status()

	// Revert all related reports to monitoring (keep opinions) + clear admin_discipline_* fields
	allReports, _ := s.repo.GetAllByTableAndParent(report.Table, report.Parent)
	for _, r := range allReports {
		_ = s.repo.UpdateStatus(r.ID, ReportStatusMonitoring, adminID)
		_ = s.repo.ClearAdminDisciplineFields(r.ID)
	}

	s.recordHistory(report.Table, report.SGID, report.Parent, prevStatus, ReportStatusMonitoring, adminID, "모니터링으로 되돌리기")
	log.Printf("[INFO] 신고 되돌리기(모니터링): table=%s, parent=%d, admin=%s", report.Table, report.Parent, adminID)
	return nil
}

// GetOpinions retrieves opinions for a specific report
// requestingUserID: 요청자 mb_id, singoRole: 요청자의 singo 역할 (admin/super_admin)
func (s *ReportService) GetOpinions(table string, sgID, parent int, requestingUserID, singoRole string) ([]domain.OpinionResponse, error) {
	if s.opinionRepo == nil {
		return []domain.OpinionResponse{}, nil
	}

	opinions, err := s.opinionRepo.GetByReportGrouped(table, parent)
	if err != nil {
		return nil, err
	}

	// Batch-load nicknames
	userIDs := make([]string, 0, len(opinions))
	for _, op := range opinions {
		userIDs = append(userIDs, op.ReviewerID)
	}
	nickMap, _ := s.memberRepo.FindNicksByIDs(userIDs)
	if nickMap == nil {
		nickMap = map[string]string{}
	}

	// 담당자 닉네임 마스킹 (admin 역할일 때)
	// super_admin은 실제 닉네임 표시, admin은 마스킹
	maskReviewerNick := singoRole != "super_admin"
	maskedCounter := 1
	maskedNickMap := map[string]string{} // reviewerID → 마스킹된 닉네임 캐시

	responses := make([]domain.OpinionResponse, len(opinions))
	for i, op := range opinions {
		reviewerNick := nickMap[op.ReviewerID]

		// 닉네임을 찾을 수 없는 경우 (탈퇴한 사용자 등) 기본값 설정
		if reviewerNick == "" {
			reviewerNick = "(알 수 없음)"
		}

		if maskReviewerNick {
			if op.ReviewerID == requestingUserID {
				reviewerNick = "나(본인)"
			} else if cached, ok := maskedNickMap[op.ReviewerID]; ok {
				reviewerNick = cached
			} else {
				reviewerNick = fmt.Sprintf("검토자 %d", maskedCounter)
				maskedNickMap[op.ReviewerID] = reviewerNick
				maskedCounter++
			}
		}

		responses[i] = domain.OpinionResponse{
			ReviewerID:   op.ReviewerID,
			ReviewerNick: reviewerNick,
			OpinionType:  op.OpinionType,
			Reasons:      op.DisciplineReasons,
			Days:         op.DisciplineDays,
			Type:         op.DisciplineType,
			Detail:       op.DisciplineDetail,
			CreatedAt:    op.CreatedAt.Format("2006-01-02 15:04:05"),
		}

		// admin 역할이면 ReviewerID도 마스킹 (다른 담당자 식별 방지)
		if maskReviewerNick && op.ReviewerID != requestingUserID {
			responses[i].ReviewerID = ""
		}
	}

	return responses, nil
}

// GetStats retrieves report statistics (aggregated by unique content) — single query
func (s *ReportService) GetStats() (map[string]int64, error) {
	stats, err := s.repo.GetAllStatusCounts()
	if err != nil {
		// Fallback to legacy loop method
		return s.getStatsLegacy()
	}
	return stats, nil
}

// getStatsLegacy is the fallback method using individual queries per status
func (s *ReportService) getStatsLegacy() (map[string]int64, error) {
	stats := make(map[string]int64)

	statuses := []string{ReportStatusPending, ReportStatusMonitoring, ReportStatusHold, ReportStatusApproved, ReportStatusDismissed}
	var total int64
	for _, status := range statuses {
		count, err := s.repo.CountByStatusAggregated(status)
		if err != nil {
			count, err = s.repo.CountByStatus(status)
			if err != nil {
				return nil, err
			}
		}
		stats[status] = count
		total += count
	}
	stats["total"] = total
	return stats, nil
}
