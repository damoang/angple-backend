package v2

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// DisciplineLogHandler handles discipline log API endpoints
// Reads from g5_write_disciplinelog table (gnuboard legacy format)
type DisciplineLogHandler struct {
	writeRepo gnurepo.WriteRepository
}

// NewDisciplineLogHandler creates a new DisciplineLogHandler
func NewDisciplineLogHandler(writeRepo gnurepo.WriteRepository) *DisciplineLogHandler {
	return &DisciplineLogHandler{writeRepo: writeRepo}
}

// DisciplineLogContent represents the JSON structure in wr_content
type DisciplineLogContent struct {
	PenaltyMbID     string         `json:"penalty_mb_id"`
	PenaltyPeriod   int            `json:"penalty_period"` // -1: permanent, 0: warning, >0: days
	PenaltyDateFrom string         `json:"penalty_date_from"`
	SgTypes         []int          `json:"sg_types"`
	ReportedItems   []ReportedItem `json:"reported_items,omitempty"`
}

// ReportedItem represents a reported post or comment
type ReportedItem struct {
	Table  string `json:"table"`
	ID     int    `json:"id"`
	Parent int    `json:"parent,omitempty"`
}

// ViolationType represents a type of rule violation
type ViolationType struct {
	Code        int    `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ViolationTypes is the list of all violation types (from disciplinelog.inc.php)
var ViolationTypes = []ViolationType{
	// 기본 위반 유형 (1-18)
	{1, "회원비하", "회원을 비난하거나 비하하는 행위"},
	{2, "예의없음", "반말 등 예의를 갖추지 않은 행위"},
	{3, "부적절한 표현", "욕설, 비속어, 혐오표현 등 부적절한 표현을 사용하는 행위"},
	{4, "차별행위", "지역, 세대, 성, 인종 등 특정한 집단에 대한 차별행위"},
	{5, "분란유도/갈등조장", "분란을 유도하거나 갈등을 조장하는 행위"},
	{6, "여론조성", "특정한 목적을 숨기고 여론을 조성하는 행위"},
	{7, "회원기만", "회원을 기만하는 행위"},
	{8, "이용방해", "회원의 서비스 이용을 방해하는 행위"},
	{9, "용도위반", "게시판의 용도를 위반하는 행위"},
	{10, "거래금지위반", "회사의 허락 없이 게시판을 통해 물품/금전을 거래하는 행위"},
	{11, "구걸", "금전을 요구하거나 금전의 지급을 유도하는 행위"},
	{12, "권리침해", "타인의 권리를 침해하는 행위"},
	{13, "외설", "지나치게 외설적인 표현물을 공유하는 행위"},
	{14, "위법행위", "불법정보, 불법촬영물을 공유하는 등 현행법에 위배되는 행위"},
	{15, "광고/홍보", "회사의 허락 없이 광고나 홍보하는 행위"},
	{16, "운영정책부정", "운영진/운영정책을 근거 없이 반복적으로 부정하는 행위"},
	{17, "다중이", "다중계정 또는 징계회피목적으로 재가입하는 행위"},
	{18, "기타사유", "기타 전항 각호에 준하는 사유"},
	// 확장 위반 유형 (21-38: 1-18과 동일)
	{21, "회원비하", "회원을 비난하거나 비하하는 행위"},
	{22, "예의없음", "반말 등 예의를 갖추지 않은 행위"},
	{23, "부적절한 표현", "욕설, 비속어, 혐오표현 등 부적절한 표현을 사용하는 행위"},
	{24, "차별행위", "지역, 세대, 성, 인종 등 특정한 집단에 대한 차별행위"},
	{25, "분란유도/갈등조장", "분란을 유도하거나 갈등을 조장하는 행위"},
	{26, "여론조성", "특정한 목적을 숨기고 여론을 조성하는 행위"},
	{27, "회원기만", "회원을 기만하는 행위"},
	{28, "이용방해", "회원의 서비스 이용을 방해하는 행위"},
	{29, "용도위반", "게시판의 용도를 위반하는 행위"},
	{30, "거래금지위반", "회사의 허락 없이 게시판을 통해 물품/금전을 거래하는 행위"},
	{31, "구걸", "금전을 요구하거나 금전의 지급을 유도하는 행위"},
	{32, "권리침해", "타인의 권리를 침해하는 행위"},
	{33, "외설", "지나치게 외설적인 표현물을 공유하는 행위"},
	{34, "위법행위", "불법정보, 불법촬영물을 공유하는 등 현행법에 위배되는 행위"},
	{35, "광고/홍보", "회사의 허락 없이 광고나 홍보하는 행위"},
	{36, "운영정책부정", "운영진/운영정책을 근거 없이 반복적으로 부정하는 행위"},
	{37, "다중이", "다중계정 또는 징계회피목적으로 재가입하는 행위"},
	{38, "기타사유", "기타 전항 각호에 준하는 사유"},
	// 추가 유형 (39-40)
	{39, "뉴스펌글누락", "뉴스 펌글 작성 시 필수 사항(스크린샷, 출처, 의견) 누락"},
	{40, "뉴스전문전재", "뉴스 전문을 허가 없이 전재하는 행위"},
}

// getViolationType returns the violation type by code
func getViolationType(code int) *ViolationType {
	for _, v := range ViolationTypes {
		if v.Code == code {
			return &v
		}
	}
	return nil
}

// DisciplineLogListItem represents a discipline log item in list
type DisciplineLogListItem struct {
	ID              int      `json:"id"`
	MemberID        string   `json:"member_id"`
	MemberNickname  string   `json:"member_nickname"`
	PenaltyPeriod   int      `json:"penalty_period"`
	PenaltyDateFrom string   `json:"penalty_date_from"`
	ViolationTypes  []int    `json:"violation_types"`
	ViolationTitles []string `json:"violation_titles"`
}

// DisciplineLogDetail represents detailed discipline log
type DisciplineLogDetail struct {
	ID              int             `json:"id"`
	MemberID        string          `json:"member_id"`
	MemberNickname  string          `json:"member_nickname"`
	PenaltyPeriod   int             `json:"penalty_period"`
	PenaltyDateFrom string          `json:"penalty_date_from"`
	PenaltyDateTo   *string         `json:"penalty_date_to,omitempty"`
	ViolationTypes  []ViolationType `json:"violation_types"`
	ReportedItems   []ReportedItem  `json:"reported_items,omitempty"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       string          `json:"created_at"`
}

// parseContentJSON parses the wr_content JSON or extracts from HTML
func parseContentJSON(content string) (*DisciplineLogContent, error) {
	var data DisciplineLogContent

	// First try direct JSON parse
	if err := json.Unmarshal([]byte(content), &data); err == nil {
		return &data, nil
	}

	// If not pure JSON, try to extract JSON from HTML content
	// Look for JSON block within HTML (sometimes wrapped in <p> or other tags)
	jsonPattern := regexp.MustCompile(`\{[^{}]*"penalty_mb_id"[^{}]*\}`)
	match := jsonPattern.FindString(content)
	if match != "" {
		if err := json.Unmarshal([]byte(match), &data); err == nil {
			return &data, nil
		}
	}

	// Try to find JSON in hidden div or script
	jsonBlockPattern := regexp.MustCompile(`(?s)\{.*?"penalty_mb_id".*?\}`)
	if matches := jsonBlockPattern.FindStringSubmatch(content); len(matches) > 0 {
		// Try each potential JSON block
		for _, m := range matches {
			if err := json.Unmarshal([]byte(m), &data); err == nil {
				return &data, nil
			}
		}
	}

	return nil, nil // No valid JSON found
}

// getMemberNickFromTitle extracts member nickname from title
// Title format is usually: "닉네임 (아이디) 님에 대한 이용제한 안내"
func getMemberNickFromTitle(title string) string {
	// Try to extract nickname before "(" or "님"
	if idx := strings.Index(title, " ("); idx > 0 {
		return title[:idx]
	}
	if idx := strings.Index(title, "님"); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
}

// GetList handles GET /api/v1/discipline-logs
func (h *DisciplineLogHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// Read from g5_write_disciplinelog
	posts, total, err := h.writeRepo.FindPosts("disciplinelog", page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이용제한 기록 조회 실패", err)
		return
	}

	items := make([]DisciplineLogListItem, 0, len(posts))
	for _, post := range posts {
		data, err := parseContentJSON(post.WrContent)
		if err != nil || data == nil {
			continue
		}

		// Get violation titles
		titles := make([]string, 0, len(data.SgTypes))
		for _, code := range data.SgTypes {
			if vt := getViolationType(code); vt != nil {
				titles = append(titles, vt.Title)
			}
		}

		// Extract date part only
		dateFrom := data.PenaltyDateFrom
		if len(dateFrom) > 10 {
			dateFrom = dateFrom[:10]
		}

		items = append(items, DisciplineLogListItem{
			ID:              post.WrID,
			MemberID:        data.PenaltyMbID,
			MemberNickname:  getMemberNickFromTitle(post.WrSubject),
			PenaltyPeriod:   data.PenaltyPeriod,
			PenaltyDateFrom: dateFrom,
			ViolationTypes:  data.SgTypes,
			ViolationTitles: titles,
		})
	}

	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, limit, total))
}

// GetDetail handles GET /api/v1/discipline-logs/:id
func (h *DisciplineLogHandler) GetDetail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 ID", err)
		return
	}

	post, err := h.writeRepo.FindPostByID("disciplinelog", id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "이용제한 기록을 찾을 수 없습니다", err)
		return
	}

	data, err := parseContentJSON(post.WrContent)
	if err != nil || data == nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "이용제한 기록 파싱 실패", err)
		return
	}

	// Get violation types
	violations := make([]ViolationType, 0, len(data.SgTypes))
	for _, code := range data.SgTypes {
		if vt := getViolationType(code); vt != nil {
			violations = append(violations, *vt)
		}
	}

	// Calculate penalty end date
	var penaltyDateTo *string
	if data.PenaltyPeriod > 0 {
		dateFrom, err := time.Parse("2006-01-02 15:04:05", data.PenaltyDateFrom)
		if err != nil {
			dateFrom, _ = time.Parse("2006-01-02", data.PenaltyDateFrom)
		}
		dateTo := dateFrom.AddDate(0, 0, data.PenaltyPeriod).Format("2006-01-02 15:04:05")
		penaltyDateTo = &dateTo
	}

	// Convert reported items format (table -> board_id)
	reportedItems := make([]ReportedItem, 0, len(data.ReportedItems))
	for _, item := range data.ReportedItems {
		reportedItems = append(reportedItems, ReportedItem{
			Table:  item.Table,
			ID:     item.ID,
			Parent: item.Parent,
		})
	}

	detail := DisciplineLogDetail{
		ID:              post.WrID,
		MemberID:        data.PenaltyMbID,
		MemberNickname:  getMemberNickFromTitle(post.WrSubject),
		PenaltyPeriod:   data.PenaltyPeriod,
		PenaltyDateFrom: data.PenaltyDateFrom,
		PenaltyDateTo:   penaltyDateTo,
		ViolationTypes:  violations,
		ReportedItems:   reportedItems,
		CreatedBy:       post.MbID,
		CreatedAt:       post.WrDatetime.Format("2006-01-02 15:04:05"),
	}

	common.V2Success(c, detail)
}

// GetViolationTypes handles GET /api/v1/discipline-logs/violation-types
func (h *DisciplineLogHandler) GetViolationTypes(c *gin.Context) {
	common.V2Success(c, ViolationTypes)
}
