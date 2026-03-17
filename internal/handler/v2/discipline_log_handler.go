package v2

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DisciplineLogHandler handles discipline log API endpoints
// Reads from g5_write_disciplinelog table (gnuboard legacy format)
type DisciplineLogHandler struct {
	writeRepo gnurepo.WriteRepository
	db        *gorm.DB
}

// NewDisciplineLogHandler creates a new DisciplineLogHandler
func NewDisciplineLogHandler(writeRepo gnurepo.WriteRepository, db *gorm.DB) *DisciplineLogHandler {
	return &DisciplineLogHandler{writeRepo: writeRepo, db: db}
}

// DisciplineLogContent represents the JSON structure in wr_content
type DisciplineLogContent struct {
	PenaltyMbID     string         `json:"penalty_mb_id"`
	PenaltyPeriod   int            `json:"penalty_period"` // -1: permanent, 0: warning, >0: days
	PenaltyDateFrom string         `json:"penalty_date_from"`
	SgTypes         []int          `json:"sg_types"`
	ReportedItems   []ReportedItem `json:"reported_items,omitempty"`
	Content         string         `json:"content,omitempty"`
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
	PenaltyDateTo   *string  `json:"penalty_date_to,omitempty"`
	ViolationTypes  []int    `json:"violation_types"`
	ViolationTitles []string `json:"violation_titles"`
	Memo            string   `json:"memo,omitempty"`
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
	Memo            string          `json:"memo,omitempty"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       string          `json:"created_at"`
	ClaimPostID     *int            `json:"claim_post_id,omitempty"`
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
// Title formats:
//   - "member_id(닉네임)" → "닉네임"
//   - "닉네임 (아이디) 님에 대한 이용제한 안내" → "닉네임"
func getMemberNickFromTitle(title string) string {
	// Format: "member_id(닉네임)" or "member_id(닉네임) ..."
	if openIdx := strings.Index(title, "("); openIdx > 0 {
		if closeIdx := strings.Index(title[openIdx:], ")"); closeIdx > 1 {
			return title[openIdx+1 : openIdx+closeIdx]
		}
	}
	// Fallback: "닉네임 (아이디)" format
	if idx := strings.Index(title, " ("); idx > 0 {
		return title[:idx]
	}
	if idx := strings.Index(title, "님"); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
}

// disciplineLogColumns are the columns selected for discipline log queries
var disciplineLogColumns = []string{
	"wr_id", "wr_num", "wr_reply", "wr_parent", "wr_is_comment",
	"wr_comment", "wr_comment_reply", "ca_name", "wr_option",
	"wr_subject", "wr_content", "wr_link1", "wr_link2",
	"wr_link1_hit", "wr_link2_hit", "wr_hit", "wr_good", "wr_nogood",
	"mb_id", "wr_password", "wr_name", "wr_email", "wr_homepage",
	"wr_datetime", "wr_file", "wr_last", "wr_ip",
	"wr_10",
	"wr_deleted_at", "wr_deleted_by",
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
	offset := (page - 1) * limit

	memberID := c.Query("member_id")

	var posts []*gnuboard.G5Write
	var total int64

	if memberID != "" {
		// Filter by member_id using JSON_EXTRACT on wr_content
		table := "g5_write_disciplinelog"
		filter := "wr_is_comment = 0 AND wr_deleted_at IS NULL AND JSON_VALID(wr_content) AND JSON_UNQUOTE(JSON_EXTRACT(wr_content, '$.penalty_mb_id')) = ?"

		h.db.Table(table).Where(filter, memberID).Count(&total)
		h.db.Table(table).Select(disciplineLogColumns).Where(filter, memberID).
			Order("wr_num, wr_reply").Offset(offset).Limit(limit).Find(&posts)
	} else {
		var err error
		posts, total, err = h.writeRepo.FindPosts("disciplinelog", page, limit)
		if err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "이용제한 기록 조회 실패", err)
			return
		}
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

		// Calculate penalty_date_to for time-limited penalties
		var penaltyDateTo *string
		if data.PenaltyPeriod > 0 {
			df, err := time.Parse("2006-01-02 15:04:05", data.PenaltyDateFrom)
			if err != nil {
				df, _ = time.Parse("2006-01-02", data.PenaltyDateFrom)
			}
			dt := df.AddDate(0, 0, data.PenaltyPeriod).Format("2006-01-02 15:04:05")
			penaltyDateTo = &dt
		}

		items = append(items, DisciplineLogListItem{
			ID:              post.WrID,
			MemberID:        data.PenaltyMbID,
			MemberNickname:  getMemberNickFromTitle(post.WrSubject),
			PenaltyPeriod:   data.PenaltyPeriod,
			PenaltyDateFrom: dateFrom,
			PenaltyDateTo:   penaltyDateTo,
			ViolationTypes:  data.SgTypes,
			ViolationTitles: titles,
			Memo:            data.Content,
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
		Memo:            data.Content,
		CreatedBy:       post.MbID,
		CreatedAt:       post.WrDatetime.Format("2006-01-02 15:04:05"),
	}

	// 소명글 존재 여부 조회 (claim 게시판에서 wr_link1 = 'disciplinelog:{id}')
	var claimPostID int
	linkValue := "disciplinelog:" + strconv.Itoa(id)
	err = h.db.Table("g5_write_claim").
		Select("wr_id").
		Where("wr_link1 = ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", linkValue).
		Order("wr_id DESC").
		Limit(1).
		Scan(&claimPostID).Error
	if err == nil && claimPostID > 0 {
		detail.ClaimPostID = &claimPostID
	}

	common.V2Success(c, detail)
}

// GetViolationTypes handles GET /api/v1/discipline-logs/violation-types
func (h *DisciplineLogHandler) GetViolationTypes(c *gin.Context) {
	common.V2Success(c, ViolationTypes)
}
