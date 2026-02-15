package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

const (
	penaltyTypeLevel     = "level"
	penaltyTypeIntercept = "intercept"
)

// AIEvaluator 백엔드 AI 평가 실행기
type AIEvaluator struct {
	aiRepo         *repository.AIEvaluationRepository
	reportRepo     *repository.ReportRepository
	opinionRepo    *repository.OpinionRepository
	boardRepo      *repository.BoardRepository
	memberRepo     repository.MemberRepository
	disciplineRepo *repository.DisciplineRepository
	postRepo       repository.PostRepository    // 전체 콘텐츠 평가용
	commentRepo    repository.CommentRepository // 전체 콘텐츠 평가용
	proxyURL       string                       // CLIProxyAPI base URL (e.g. "http://127.0.0.1:8317/v1")
	proxyKey       string                       // API key
	models         []string                     // e.g. ["claude-sonnet-4-5-20250929", "gpt-5", "gemini-2.5-pro"]
	httpClient     *http.Client
}

// NewAIEvaluator creates a new AIEvaluator
func NewAIEvaluator(
	aiRepo *repository.AIEvaluationRepository,
	reportRepo *repository.ReportRepository,
	opinionRepo *repository.OpinionRepository,
	boardRepo *repository.BoardRepository,
	memberRepo repository.MemberRepository,
	disciplineRepo *repository.DisciplineRepository,
	postRepo repository.PostRepository,
	commentRepo repository.CommentRepository,
	proxyURL string,
	proxyKey string,
	models []string,
) *AIEvaluator {
	return &AIEvaluator{
		aiRepo:         aiRepo,
		reportRepo:     reportRepo,
		opinionRepo:    opinionRepo,
		boardRepo:      boardRepo,
		memberRepo:     memberRepo,
		disciplineRepo: disciplineRepo,
		postRepo:       postRepo,
		commentRepo:    commentRepo,
		proxyURL:       proxyURL,
		proxyKey:       proxyKey,
		models:         models,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// EvaluateAsync 비동기 평가 실행 (이미 평가 있으면 skip)
func (e *AIEvaluator) EvaluateAsync(table string, parent int) error {
	existing, err := e.aiRepo.ListByReport(table, parent)
	if err != nil {
		log.Printf("[AI평가] 기존 평가 조회 실패: %v", err)
	}
	if len(existing) > 0 {
		log.Printf("[AI평가] skip: 이미 평가 존재 (table=%s, parent=%d, count=%d)", table, parent, len(existing))
		return nil
	}
	_, err = e.Evaluate(table, parent)
	return err
}

// Evaluate 동기 평가 실행 (3개 모델 병렬)
func (e *AIEvaluator) Evaluate(table string, parent int) ([]domain.AIEvaluation, error) {
	// 1. 리포트 로드
	report, err := e.reportRepo.GetByTableAndParent(table, parent)
	if err != nil {
		return nil, fmt.Errorf("리포트 조회 실패: %w", err)
	}

	// 2. 의견 로드
	opinions, err := e.opinionRepo.GetByReportGrouped(table, parent)
	if err != nil {
		opinions = nil
	}

	// 3. 게시판명 조회
	boardName := table
	if e.boardRepo != nil {
		if names, err := e.boardRepo.FindByIDs([]string{table}); err == nil {
			if name, ok := names[table]; ok {
				boardName = name
			}
		}
	}

	// 4. 닉네임 로드
	nickMap := e.loadNicknames(report, opinions)

	// 5. 모든 신고 로드하여 신고 사유 유형 수집
	allReports, _ := e.reportRepo.GetAllByTableAndParent(table, parent)
	reportReasons := collectReportReasons(allReports)

	// 6. 피신고자 제재 이력 로드 (최근 10건)
	var disciplineHistory []domain.DisciplineLog
	if e.disciplineRepo != nil && report.TargetID != "" {
		if history, _, err := e.disciplineRepo.FindByTargetMember(report.TargetID, 1, 10); err == nil {
			disciplineHistory = history
		}
	}

	// 7. 프롬프트 빌드
	systemPrompt := buildSystemPrompt()
	userMessage := e.buildUserMessage(report, boardName, opinions, nickMap, reportReasons, disciplineHistory)

	// 7. 3개 모델 병렬 호출
	type evalResult struct {
		eval *domain.AIEvaluation
		err  error
	}

	var wg sync.WaitGroup
	results := make([]evalResult, len(e.models))

	for i, model := range e.models {
		wg.Add(1)
		go func(idx int, modelName string) {
			defer wg.Done()
			eval, err := e.callAndSave(table, parent, modelName, systemPrompt, userMessage, "system")
			results[idx] = evalResult{eval: eval, err: err}
		}(i, model)
	}
	wg.Wait()

	// 8. 결과 수집
	var evals []domain.AIEvaluation
	for i, r := range results {
		if r.err != nil {
			log.Printf("[AI평가] 모델 %s 실패: %v", e.models[i], r.err)
			continue
		}
		if r.eval != nil {
			evals = append(evals, *r.eval)
		}
	}

	if len(evals) == 0 {
		return nil, fmt.Errorf("모든 AI 모델 평가 실패")
	}

	log.Printf("[AI평가] 완료 (table=%s, parent=%d, 성공=%d/%d)", table, parent, len(evals), len(e.models))
	return evals, nil
}

// DeleteAndReEvaluate 기존 평가 삭제 후 재평가
func (e *AIEvaluator) DeleteAndReEvaluate(table string, parent int) ([]domain.AIEvaluation, error) {
	if err := e.aiRepo.DeleteByReport(table, parent); err != nil {
		log.Printf("[AI평가] 기존 평가 삭제 실패: %v", err)
	}
	return e.Evaluate(table, parent)
}

// EvaluateFullContent 전체 콘텐츠(원본 글 + 모든 댓글) 기반 AI 평가
func (e *AIEvaluator) EvaluateFullContent(table string, parent int) ([]domain.AIEvaluation, error) {
	if e.postRepo == nil || e.commentRepo == nil {
		return nil, fmt.Errorf("전체 콘텐츠 평가 기능이 비활성화되어 있습니다 (postRepo/commentRepo 미설정)")
	}

	// 1. 리포트 로드
	report, err := e.reportRepo.GetByTableAndParent(table, parent)
	if err != nil {
		return nil, fmt.Errorf("리포트 조회 실패: %w", err)
	}

	// 2. 글/댓글 타입 판별 및 전체 콘텐츠 로드
	// 댓글 신고: report.Parent != 0 → 부모글 + 부모글의 모든 댓글
	// 글 신고: report.Parent == 0 → 해당 글 + 해당 글의 모든 댓글
	postID := report.SGID
	if report.Parent != 0 {
		postID = report.Parent
	}

	post, err := e.postRepo.FindByID(table, postID)
	if err != nil {
		return nil, fmt.Errorf("게시글 조회 실패: %w", err)
	}

	comments, err := e.commentRepo.ListByPost(table, postID)
	if err != nil {
		log.Printf("[AI평가:전체콘텐츠] 댓글 조회 실패: %v", err)
		comments = nil
	}

	// 3. 의견 로드
	opinions, err := e.opinionRepo.GetByReportGrouped(table, parent)
	if err != nil {
		opinions = nil
	}

	// 4. 게시판명 조회
	boardName := table
	if e.boardRepo != nil {
		if names, err := e.boardRepo.FindByIDs([]string{table}); err == nil {
			if name, ok := names[table]; ok {
				boardName = name
			}
		}
	}

	// 5. 닉네임 로드
	nickMap := e.loadNicknames(report, opinions)

	// 6. 신고 사유 유형 수집
	allReports, _ := e.reportRepo.GetAllByTableAndParent(table, parent)
	reportReasons := collectReportReasons(allReports)

	// 7. 피신고자 제재 이력 로드
	var disciplineHistory []domain.DisciplineLog
	if e.disciplineRepo != nil && report.TargetID != "" {
		if history, _, err := e.disciplineRepo.FindByTargetMember(report.TargetID, 1, 10); err == nil {
			disciplineHistory = history
		}
	}

	// 8. 전체 콘텐츠 프롬프트 빌드
	systemPrompt := buildSystemPrompt()
	userMessage := e.buildFullContentUserMessage(report, boardName, post, comments, opinions, nickMap, reportReasons, disciplineHistory)

	// 9. 3개 모델 병렬 호출
	type evalResult struct {
		eval *domain.AIEvaluation
		err  error
	}

	var wg sync.WaitGroup
	results := make([]evalResult, len(e.models))

	for i, model := range e.models {
		wg.Add(1)
		go func(idx int, modelName string) {
			defer wg.Done()
			eval, err := e.callAndSave(table, parent, modelName, systemPrompt, userMessage, "system:full_content")
			results[idx] = evalResult{eval: eval, err: err}
		}(i, model)
	}
	wg.Wait()

	// 10. 결과 수집
	var evals []domain.AIEvaluation
	for i, r := range results {
		if r.err != nil {
			log.Printf("[AI평가:전체콘텐츠] 모델 %s 실패: %v", e.models[i], r.err)
			continue
		}
		if r.eval != nil {
			evals = append(evals, *r.eval)
		}
	}

	if len(evals) == 0 {
		return nil, fmt.Errorf("모든 AI 모델 전체 콘텐츠 평가 실패")
	}

	log.Printf("[AI평가:전체콘텐츠] 완료 (table=%s, parent=%d, 성공=%d/%d)", table, parent, len(evals), len(e.models))
	return evals, nil
}

// buildFullContentUserMessage 전체 콘텐츠(원본 글 + 모든 댓글) 기반 프롬프트 구성
func (e *AIEvaluator) buildFullContentUserMessage(
	report *domain.Report,
	boardName string,
	post *domain.Post,
	comments []*domain.Comment,
	opinions []domain.Opinion,
	nickMap map[string]string,
	reportReasons string,
	disciplineHistory []domain.DisciplineLog,
) string {
	var parts []string

	targetType := "게시물"
	if report.Parent != 0 {
		targetType = "댓글"
	}

	// 신고 정보
	parts = append(parts, "## 신고 정보")
	parts = append(parts, fmt.Sprintf("- 대상 유형: %s", targetType))
	parts = append(parts, fmt.Sprintf("- 게시판: %s", boardName))

	if reportReasons != "" {
		parts = append(parts, fmt.Sprintf("- 신고 사유: %s", reportReasons))
	} else {
		reason := report.Reason
		if reason == "" && report.Type > 0 {
			if label, ok := sgTypeLabels[report.Type]; ok {
				reason = label
			} else {
				reason = fmt.Sprintf("%d", report.Type)
			}
		}
		parts = append(parts, fmt.Sprintf("- 신고 사유: %s", reason))
	}

	reporterNick := nickMap[report.ReporterID]
	if reporterNick == "" {
		reporterNick = report.ReporterID
	}
	parts = append(parts, fmt.Sprintf("- 신고자: %s (%s)", reporterNick, report.ReporterID))

	targetNick := nickMap[report.TargetID]
	if targetNick == "" {
		targetNick = report.TargetID
	}
	parts = append(parts, fmt.Sprintf("- 피신고자: %s (%s)", targetNick, report.TargetID))

	// 신고 대상 콘텐츠 (스냅샷)
	parts = append(parts, "")
	parts = append(parts, "## 신고 대상 콘텐츠 (신고 시점 스냅샷)")
	if report.TargetTitle != "" {
		parts = append(parts, fmt.Sprintf("제목: %s", report.TargetTitle))
	}
	if report.TargetContent != "" {
		parts = append(parts, fmt.Sprintf("내용:\n%s", report.TargetContent))
	} else {
		parts = append(parts, "(콘텐츠를 불러올 수 없음)")
	}

	// 전체 게시글 원문 (현재 상태)
	parts = append(parts, "")
	parts = append(parts, "## 전체 게시글 원문 (현재 상태)")
	if post != nil {
		parts = append(parts, fmt.Sprintf("제목: %s", post.Title))
		parts = append(parts, fmt.Sprintf("작성자: %s (%s)", post.Author, post.AuthorID))
		parts = append(parts, fmt.Sprintf("작성일: %s", post.CreatedAt.Format("2006-01-02 15:04:05")))
		content := stripHTML(post.Content)
		if len(content) > 5000 {
			content = content[:5000] + "\n...(이하 생략)"
		}
		parts = append(parts, fmt.Sprintf("내용:\n%s", content))
	} else {
		parts = append(parts, "(게시글을 불러올 수 없음)")
	}

	// 전체 댓글
	if len(comments) > 0 {
		maxComments := 50
		if len(comments) > maxComments {
			parts = append(parts, "")
			parts = append(parts, fmt.Sprintf("## 전체 댓글 (%d개 중 최근 %d개)", len(comments), maxComments))
			comments = comments[len(comments)-maxComments:]
		} else {
			parts = append(parts, "")
			parts = append(parts, fmt.Sprintf("## 전체 댓글 (%d개)", len(comments)))
		}

		for idx, c := range comments {
			// 신고 대상 댓글 표시 (댓글 신고인 경우)
			marker := ""
			if report.Parent != 0 && c.ID == report.SGID {
				marker = " ★신고대상★"
			}
			content := stripHTML(c.Content)
			if len(content) > 1000 {
				content = content[:1000] + "...(생략)"
			}
			parts = append(parts, fmt.Sprintf("\n[댓글 #%d]%s %s (%s) - %s\n%s",
				idx+1, marker, c.Author, c.AuthorID, c.CreatedAt.Format("2006-01-02 15:04"), content))
		}
	}

	// 모니터링 의견
	if len(opinions) > 0 {
		parts = append(parts, "")
		parts = append(parts, "## 모니터링 의견")
		for _, op := range opinions {
			actionLabel := "조치 필요"
			if op.OpinionType != "action" {
				actionLabel = "조치 불필요"
			}
			reviewerNick := nickMap[op.ReviewerID]
			if reviewerNick == "" {
				reviewerNick = op.ReviewerID
			}
			daysStr := ""
			if op.DisciplineDays > 0 {
				daysStr = fmt.Sprintf(" (%d일)", op.DisciplineDays)
			}
			parts = append(parts, fmt.Sprintf("- %s: %s%s", reviewerNick, actionLabel, daysStr))
			if op.DisciplineDetail != "" {
				parts = append(parts, fmt.Sprintf("  > %s", op.DisciplineDetail))
			}
		}
	}

	// 피신고자 제재 이력
	if len(disciplineHistory) > 0 {
		parts = append(parts, "")
		parts = append(parts, fmt.Sprintf("## 피신고자 제재 이력 (최근 %d건, 누적 %d회)", len(disciplineHistory), len(disciplineHistory)))
		for _, hist := range disciplineHistory {
			var content domain.DisciplineLogContent
			if err := json.Unmarshal([]byte(hist.Content), &content); err == nil {
				daysLabel := fmt.Sprintf("%d일", content.PenaltyPeriod)
				if content.PenaltyPeriod == 0 {
					daysLabel = "경고"
				} else if content.PenaltyPeriod == 9999 || content.PenaltyPeriod == -1 {
					daysLabel = "영구"
				}
				reason := hist.Wr1
				if reason == "" && len(content.SgTypes) > 0 {
					var labels []string
					for _, code := range content.SgTypes {
						if label, ok := sgTypeLabels[int8(code)]; ok {
							labels = append(labels, label)
						}
					}
					reason = strings.Join(labels, ", ")
				}
				parts = append(parts, fmt.Sprintf("- [%s] %s (%s)", hist.DateTime.Format("2006-01-02"), reason, daysLabel))
			}
		}
	}

	return strings.Join(parts, "\n")
}

// callAndSave 단일 모델 호출 + DB 저장
func (e *AIEvaluator) callAndSave(table string, parent int, model, systemPrompt, userMessage, evaluatedBy string) (*domain.AIEvaluation, error) {
	rawText, err := e.callProvider(model, systemPrompt, userMessage)
	if err != nil {
		return nil, fmt.Errorf("API 호출 실패: %w", err)
	}

	validated, err := e.parseAndValidate(rawText)
	if err != nil {
		return nil, fmt.Errorf("응답 검증 실패: %w", err)
	}

	// JSON 변환
	penaltyTypeJSON, err := json.Marshal(validated.PenaltyType)
	if err != nil {
		return nil, fmt.Errorf("penaltyType JSON 변환 실패: %w", err)
	}
	penaltyReasonsJSON, err := json.Marshal(validated.PenaltyReasons)
	if err != nil {
		return nil, fmt.Errorf("penaltyReasons JSON 변환 실패: %w", err)
	}
	flagsJSON, err := json.Marshal(validated.Flags)
	if err != nil {
		return nil, fmt.Errorf("flags JSON 변환 실패: %w", err)
	}

	eval := &domain.AIEvaluation{
		Table:             table,
		Parent:            parent,
		Score:             validated.Score,
		Confidence:        validated.Confidence,
		RecommendedAction: validated.Action,
		PenaltyDays:       validated.PenaltyDays,
		PenaltyType:       string(penaltyTypeJSON),
		PenaltyReasons:    string(penaltyReasonsJSON),
		Reasoning:         validated.Reasoning,
		Flags:             string(flagsJSON),
		RawResponse:       rawText,
		Model:             model,
		EvaluatedAt:       time.Now(),
		EvaluatedBy:       evaluatedBy,
		CreatedAt:         time.Now(),
	}

	if err := e.aiRepo.Create(eval); err != nil {
		return nil, fmt.Errorf("DB 저장 실패: %w", err)
	}

	return eval, nil
}

// callProvider CLIProxyAPI (OpenAI 포맷) 호출
func (e *AIEvaluator) callProvider(model, systemPrompt, userMessage string) (string, error) {
	reqBody := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := e.proxyURL + "/chat/completions"
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.proxyKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.proxyKey)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("응답 읽기 실패: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API 오류 (%d): %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	// OpenAI 포맷 파싱
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("응답 JSON 파싱 실패: %w", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("AI 응답에서 텍스트를 찾을 수 없습니다")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// aiRawResponse AI 응답 구조체
type aiRawResponse struct {
	Score          int      `json:"score"`
	Confidence     int      `json:"confidence"`
	Action         string   `json:"action"`
	PenaltyDays    int      `json:"penalty_days"`
	PenaltyType    []string `json:"penalty_type"`
	PenaltyReasons []int    `json:"penalty_reasons"`
	Reasoning      string   `json:"reasoning"`
	Flags          []string `json:"flags"`
}

var validActions = map[string]bool{
	"dismiss": true, "warning": true, "delete": true, "ban": true,
}

var validPenaltyDays = map[int]bool{
	0: true, 1: true, 5: true, 10: true, 30: true, 180: true, 365: true, 9999: true,
}

// extractJSON 코드블록에서 JSON 추출
func extractJSON(rawText string) string {
	if idx := strings.Index(rawText, "```"); idx >= 0 {
		start := strings.Index(rawText[idx:], "\n")
		if start >= 0 {
			end := strings.Index(rawText[idx+start+1:], "```")
			if end >= 0 {
				return strings.TrimSpace(rawText[idx+start+1 : idx+start+1+end])
			}
		}
	}
	return rawText
}

// validateFields AI 응답 필드 검증
func validateFields(resp *aiRawResponse) error {
	if resp.Score < 0 || resp.Score > 100 {
		return fmt.Errorf("score는 0-100 범위여야 합니다 (받은 값: %d)", resp.Score)
	}
	if resp.Confidence < 0 || resp.Confidence > 100 {
		return fmt.Errorf("confidence는 0-100 범위여야 합니다 (받은 값: %d)", resp.Confidence)
	}
	if !validActions[resp.Action] {
		return fmt.Errorf("action은 dismiss/warning/delete/ban 중 하나여야 합니다 (받은 값: %s)", resp.Action)
	}
	if !validPenaltyDays[resp.PenaltyDays] {
		return fmt.Errorf("penalty_days가 유효하지 않습니다 (받은 값: %d)", resp.PenaltyDays)
	}
	for _, t := range resp.PenaltyType {
		if t != penaltyTypeLevel && t != penaltyTypeIntercept {
			return fmt.Errorf("penalty_type은 level/intercept만 가능합니다 (받은 값: %s)", t)
		}
	}
	for _, r := range resp.PenaltyReasons {
		if r < 21 || r > 38 {
			return fmt.Errorf("penalty_reasons는 21-38 범위여야 합니다 (받은 값: %d)", r)
		}
	}
	return nil
}

// parseAndValidate JSON 파싱 + 검증
func (e *AIEvaluator) parseAndValidate(rawText string) (*aiRawResponse, error) {
	jsonStr := extractJSON(rawText)

	var resp aiRawResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
	}

	if err := validateFields(&resp); err != nil {
		return nil, err
	}

	// dismiss 규칙 자동 보정
	if resp.Action == "dismiss" {
		resp.PenaltyDays = 0
		resp.PenaltyType = []string{}
		resp.PenaltyReasons = []int{}
	}

	return &resp, nil
}

// loadNicknames 리포트 관련 사용자 닉네임 일괄 조회
func (e *AIEvaluator) loadNicknames(report *domain.Report, opinions []domain.Opinion) map[string]string {
	var userIDs []string
	if report.ReporterID != "" {
		userIDs = append(userIDs, report.ReporterID)
	}
	if report.TargetID != "" {
		userIDs = append(userIDs, report.TargetID)
	}
	for _, op := range opinions {
		if op.ReviewerID != "" {
			userIDs = append(userIDs, op.ReviewerID)
		}
	}
	if e.memberRepo != nil && len(userIDs) > 0 {
		if nicks, err := e.memberRepo.FindNicksByIDs(userIDs); err == nil {
			return nicks
		}
	}
	return map[string]string{}
}

// buildUserMessage 신고 정보 + 콘텐츠 + 의견 + 제재 이력을 문자열로 구성
func (e *AIEvaluator) buildUserMessage(report *domain.Report, boardName string, opinions []domain.Opinion, nickMap map[string]string, reportReasons string, disciplineHistory []domain.DisciplineLog) string {
	var parts []string

	targetType := "게시물"
	if report.Parent != 0 {
		targetType = "댓글"
	}

	parts = append(parts, "## 신고 정보")
	parts = append(parts, fmt.Sprintf("- 대상 유형: %s", targetType))
	parts = append(parts, fmt.Sprintf("- 게시판: %s", boardName))

	// 전체 신고자의 사유 유형 (collectReportReasons 결과)
	if reportReasons != "" {
		parts = append(parts, fmt.Sprintf("- 신고 사유: %s", reportReasons))
	} else {
		reason := report.Reason
		if reason == "" && report.Type > 0 {
			if label, ok := sgTypeLabels[report.Type]; ok {
				reason = label
			} else {
				reason = fmt.Sprintf("%d", report.Type)
			}
		}
		parts = append(parts, fmt.Sprintf("- 신고 사유: %s", reason))
	}

	reporterNick := nickMap[report.ReporterID]
	if reporterNick == "" {
		reporterNick = report.ReporterID
	}
	parts = append(parts, fmt.Sprintf("- 신고자: %s (%s)", reporterNick, report.ReporterID))

	targetNick := nickMap[report.TargetID]
	if targetNick == "" {
		targetNick = report.TargetID
	}
	parts = append(parts, fmt.Sprintf("- 피신고자: %s (%s)", targetNick, report.TargetID))

	parts = append(parts, "")
	parts = append(parts, "## 신고 대상 콘텐츠")
	if report.TargetTitle != "" {
		parts = append(parts, fmt.Sprintf("제목: %s", report.TargetTitle))
	}
	if report.TargetContent != "" {
		parts = append(parts, fmt.Sprintf("내용:\n%s", report.TargetContent))
	} else {
		parts = append(parts, "(콘텐츠를 불러올 수 없음)")
	}

	if len(opinions) > 0 {
		parts = append(parts, "")
		parts = append(parts, "## 모니터링 의견")
		for _, op := range opinions {
			actionLabel := "조치 필요"
			if op.OpinionType != "action" {
				actionLabel = "조치 불필요"
			}
			reviewerNick := nickMap[op.ReviewerID]
			if reviewerNick == "" {
				reviewerNick = op.ReviewerID
			}
			daysStr := ""
			if op.DisciplineDays > 0 {
				daysStr = fmt.Sprintf(" (%d일)", op.DisciplineDays)
			}
			parts = append(parts, fmt.Sprintf("- %s: %s%s", reviewerNick, actionLabel, daysStr))
			if op.DisciplineDetail != "" {
				parts = append(parts, fmt.Sprintf("  > %s", op.DisciplineDetail))
			}
		}
	}

	// 피신고자 제재 이력
	if len(disciplineHistory) > 0 {
		parts = append(parts, "")
		parts = append(parts, fmt.Sprintf("## 피신고자 제재 이력 (최근 %d건, 누적 %d회)", len(disciplineHistory), len(disciplineHistory)))
		for _, hist := range disciplineHistory {
			var content domain.DisciplineLogContent
			if err := json.Unmarshal([]byte(hist.Content), &content); err == nil {
				daysLabel := fmt.Sprintf("%d일", content.PenaltyPeriod)
				if content.PenaltyPeriod == 0 {
					daysLabel = "경고"
				} else if content.PenaltyPeriod == 9999 || content.PenaltyPeriod == -1 {
					daysLabel = "영구"
				}
				reason := hist.Wr1
				if reason == "" && len(content.SgTypes) > 0 {
					var labels []string
					for _, code := range content.SgTypes {
						if label, ok := sgTypeLabels[int8(code)]; ok {
							labels = append(labels, label)
						}
					}
					reason = strings.Join(labels, ", ")
				}
				parts = append(parts, fmt.Sprintf("- [%s] %s (%s)", hist.DateTime.Format("2006-01-02"), reason, daysLabel))
			}
		}
	}

	return strings.Join(parts, "\n")
}

// buildSystemPrompt 시스템 프롬프트 생성
func buildSystemPrompt() string {
	return `당신은 다모앙(damoang.net) 커뮤니티의 신고 처리 AI 보조입니다.
신고 내용, 대상 콘텐츠, 모니터링 의견을 분석하여 관리자에게 권고 의견을 제공합니다.
아래 운영정책을 근거로 판단하세요.

## 다모앙 서비스 운영정책

### 제7조 — 이용제한 원칙
이용제한은 "필요한 최소한의 범위 내에서, 최대한 객관적인 방식으로, 그러나 가장 강력하고 단호하게" 적용합니다.

### 제9조 — 이용제한 사유 (18개 항목)

1호 **회원비하**: 타 회원 비하·조롱·험담. 프로필·닉네임·소모임명 비하 포함. '박제'(캡처 공유) 자체는 원칙적으로 회원비하로 보지 않음. 단, 비속어·허위사실 등을 동반하면 별도 위반. 빈댓글도 원칙적으로 회원비하로 보지 않으나, 특정 회원 대상 과도한 반복 시 8호(이용방해) 해당 가능.
2호 **예의없음**: 모든 사람에게 예의를 갖추어야 함 (회원·비회원·불특정 다수 모두 포함). 경어체 미사용, 비꼬기·비아냥도 해당. 단, 장르적 특성상 경어체 미사용은 예외 가능. ※ 비회원 대상: 누적 미적용, 최대 1일.
3호 **부적절한 표현**: 욕설·비속어·은어·초성 욕설(ㅅㅂ 등)·혐오표현. 우회적인 비속어 역시 금지 대상. ※ 비회원 대상: 누적 미적용, 최대 5일.
4호 **차별행위**: 성별·인종·종교·장애·지역·직업·외모 기반 차별·혐오·비하.
5호 **분란유도**: 의도적 논쟁 유발, 게시물 분위기 저해, 타인 자극. 단, 소수 견해라고 해서 불이익 없음. 충분히 논의될만한 주제에서 소수 의견을 피력하는 것은 분란유도가 아님.
6호 **여론조성**: 의도적 여론몰이, 특정 관점 강요, 조직적 찬/반 유도. 과도한 친밀함 표출(친목질), 별도 사조직 형성도 해당.
7호 **회원기만**: 허위·과장 정보 유포, 근거 없는 루머. 회원을 가장한 홍보글(바이럴)은 가장 악질적인 회원기만 행위로 15호(광고/홍보)와 복합 적용.
8호 **이용방해**: 도배, 무의미 게시물, 시스템 악용. 어떠한 이유에서도 의도적으로 서비스 이용을 방해하는 행위 불허. 스토킹 수준의 반복 박제, 특정 회원 대상 빈댓글 반복도 해당.
9호 **용도위반**: 게시판 용도 벗어난 게시물.
10호 **거래금지 위반**: 금전 거래(양도·판매·교환·현금화).
11호 **구걸**: 금전·물품 무상 요구. 자신의 어려움을 어필하여 금전 지급을 유도하는 간접 행위도 포함.
12호 **권리침해**: 저작권·초상권·개인정보 침해. 신상 노출 포함.
13호 **외설**: 음란물, 과도한 성적 콘텐츠. ⚠️ **초범 영구제한 가능**
14호 **위법행위**: 법률 위반. ⚠️ **초범 영구제한 가능**
15호 **광고/홍보**: 영리 광고, 바이럴 마케팅, 타사이트 홍보. 단, 유용한 정보 공유 과정의 자연스러운 최소한의 홍보효과는 허용. ⚠️ **초범 영구제한 가능**. 바이럴은 특히 엄벌.
16호 **운영정책 부정**: 근거 없이 반복적으로 운영정책·운영진을 부정하는 행위. 의견 개진은 유지관리 게시판 이용 가능. ⚠️ **초범 영구제한 가능**
17호 **다중이**: 복수 계정 운영. ⚠️ **초범 영구제한 가능**
18호 **기타사유**: 위 항목 외 커뮤니티 질서 저해 행위.

### 제11조 — 이용제한 기준 (누적 횟수별 제한 기간)
| 횟수 | 기간 |
|------|------|
| 1회 | 경고 또는 1일 |
| 2회 | 5일 |
| 3회 | 10일 |
| 4회 | 30일 |
| 5회 | 180일 |
| 6회 | 365일 |
| 7회+ | 영구(9999일) |

### 특별 규칙
- 13~17호(외설/위법/광고/운영부정/다중이)는 **초범이라도 영구 이용제한** 가능
- 위반의 정도가 심각한 경우 **최대 5등급까지 가중 가능** (예: 1회 위반이지만 경고가 아닌 30일 제한)
- 2호(예의없음) 비회원 대상: 누적 미적용, 최대 1일 제한
- 3호(부적절표현) 비회원 대상: 누적 미적용, 최대 5일 제한

## 신고 사유 코드 (penalty_reasons)
해당하는 코드를 모두 선택하세요. 괄호 안은 제9조 항목 번호입니다.
- 21: 회원비하 (1호)
- 22: 예의없음 (2호)
- 23: 부적절한 표현/욕설/비속어 (3호)
- 24: 차별행위/혐오 표현 (4호)
- 25: 분란유도/갈등조장 (5호)
- 26: 여론조성 (6호)
- 27: 회원기만/허위사실 (7호)
- 28: 이용방해/도배 (8호)
- 29: 용도위반 (9호)
- 30: 거래금지위반 (10호)
- 31: 구걸 (11호)
- 32: 권리침해/개인정보노출/저작권침해 (12호)
- 33: 외설/성적 표현 (13호)
- 34: 위법행위/불법 콘텐츠 (14호)
- 35: 광고/홍보/타사이트 홍보 (15호)
- 36: 운영정책부정 (16호)
- 37: 다중이/다중 계정 (17호)
- 38: 기타사유 (18호)

## 제재 유형 (penalty_type)
- "level": 등급 제한 (회원 등급 하향)
- "intercept": 활동 제한 (글쓰기/댓글 제한)
- 조합 가능: ["level"], ["intercept"], ["level","intercept"], []

## 제재 일수 (penalty_days)
허용 값만 사용: 0(경고만), 1, 5, 10, 30, 180, 365, 9999(영구)

## 권장 조치 (action)
- dismiss: 기각 (신고 사유 부족)
- warning: 경고 (경미한 위반)
- delete: 삭제 (콘텐츠 삭제 필요)
- ban: 이용제한 (심각한 위반, 활동 제한 필요)

## 판단 규칙
1. action이 "dismiss"이면 penalty_days=0, penalty_type=[], penalty_reasons=[] 이어야 합니다.
2. action이 "warning"이면 penalty_days=0, penalty_type은 빈 배열 또는 ["level"]만 가능합니다.
3. action이 "ban"이면 반드시 penalty_type에 하나 이상의 값이 있어야 합니다.
4. penalty_reasons는 해당하는 코드만 포함하세요 (복수 선택 가능).
5. score는 신고의 타당성 (0=전혀 부적절 ~ 100=매우 타당)
6. confidence는 AI의 판단 확신도 (0~100)
7. reasoning은 **운영정책 조항을 인용**하며 판단 근거를 한글로 2-3문장 작성
8. flags는 특이사항을 한글 키워드 배열로 제공
9. penalty_days는 제11조 누적 기준을 적용하세요. 피신고자의 제재 이력이 제공되면 누적 횟수에 맞는 기간을 선택하세요. 이력이 없으면 초범으로 간주합니다.
10. 제재 이력이 제공된 경우, reasoning에 "제재 이력 N회로 제11조 기준 M회차 적용" 등 근거를 명시하세요.

## 출력 형식
반드시 아래 JSON 형식만 반환하세요. 다른 텍스트는 포함하지 마세요.

{
  "score": number,
  "confidence": number,
  "action": "dismiss" | "warning" | "delete" | "ban",
  "penalty_days": number,
  "penalty_type": string[],
  "penalty_reasons": number[],
  "reasoning": string,
  "flags": string[]
}`
}

// sgTypeLabels maps sg_type integer codes to Korean labels
var sgTypeLabels = map[int8]string{
	1: "회원비하", 2: "예의없음", 3: "부적절한 표현", 4: "차별행위",
	5: "분란유도/갈등조장", 6: "여론조성", 7: "회원기만", 8: "이용방해",
	9: "용도위반", 10: "거래금지위반", 11: "구걸", 12: "권리침해",
	13: "외설", 14: "위법행위", 15: "광고/홍보", 16: "운영정책부정",
	17: "다중이", 18: "기타사유",
	21: "회원비하", 22: "예의없음", 23: "부적절한 표현", 24: "차별행위",
	25: "분란유도/갈등조장", 26: "여론조성", 27: "회원기만", 28: "이용방해",
	29: "용도위반", 30: "거래금지위반", 31: "구걸", 32: "권리침해",
	33: "외설", 34: "위법행위", 35: "광고/홍보", 36: "운영정책부정",
	37: "다중이", 38: "기타사유",
}

// collectReportReasons 모든 신고에서 sg_type 라벨을 수집하여 "사유 (건수)" 형태로 반환
func collectReportReasons(reports []domain.Report) string {
	if len(reports) == 0 {
		return ""
	}
	counts := make(map[string]int)
	var order []string
	for _, r := range reports {
		if r.Type == 0 {
			continue
		}
		label, ok := sgTypeLabels[r.Type]
		if !ok {
			label = fmt.Sprintf("코드%d", r.Type)
		}
		if counts[label] == 0 {
			order = append(order, label)
		}
		counts[label]++
	}
	var parts []string
	for _, label := range order {
		parts = append(parts, fmt.Sprintf("%s (%d건)", label, counts[label]))
	}
	return strings.Join(parts, ", ")
}

// truncateStr truncates a string to maxLen bytes
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
