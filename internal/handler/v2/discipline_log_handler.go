package v2

import (
	"encoding/json"
	"log"
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
	PenaltyMbID       string         `json:"penalty_mb_id"`
	PenaltyPeriod     int            `json:"penalty_period"` // -1: permanent, 0: warning, >0: days
	PenaltyDateFrom   string         `json:"penalty_date_from"`
	SgTypes           []int          `json:"sg_types"`
	ReportedItems     []ReportedItem `json:"reported_items,omitempty"`
	Content           string         `json:"content,omitempty"`
	MemberReason      string         `json:"member_reason,omitempty"`      // 회원 공개 사유 (운영자 입력 시 상세에 노출)
	PublicDescription string         `json:"public_description,omitempty"` // 외부 공개용 안내문 (운영자 입력 시 상세에 노출, 기타 사유와 별개)
	// 소명 인용 등으로 이용제한이 회수(revoke)된 경우 RevokeDiscipline(damoang-backend)이 기록.
	// RevokedAt만 회원에게 공개하고 RevokedBy(운영자ID)·AdminMemo(회수사유)는 비공개(내부용).
	RevokedAt string `json:"revoked_at,omitempty"`
	RevokedBy string `json:"revoked_by,omitempty"`
	AdminMemo string `json:"admin_memo,omitempty"`
}

// ReportedItem represents a reported post or comment.
// per-item 필드(SgTypes/PenaltyDays/Memo)는 한 disciplinelog 글 안에서 항목별 적용 사유를 보존·표시.
// 신규 글은 wr_content JSON에 직접 기록되고, 레거시 글은 GetDetail이 g5_na_singo에서 보강한다.
type ReportedItem struct {
	Table       string `json:"table"`
	ID          int    `json:"id"`
	Parent      int    `json:"parent,omitempty"`
	SgTypes     []int  `json:"sg_types,omitempty"`     // 항목별 적용 사유 코드(21~40)
	PenaltyDays *int   `json:"penalty_days,omitempty"` // 항목별 일수(-1=영구), nil=미상
	Memo        string `json:"memo,omitempty"`         // 항목별 관리자 메모
	Deleted     bool   `json:"deleted"`                // 신고 접수 후 삭제된 글 여부(삭제됨 배지 표시용)
}

// disciplineBoardSlugRe는 g5_write_{board} 동적 테이블명에 쓰일 보드 슬러그를 검증(SQL 인젝션 방지).
var disciplineBoardSlugRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

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

// violationTypeMap is a pre-built lookup map for O(1) access by code
var violationTypeMap = func() map[int]*ViolationType {
	m := make(map[int]*ViolationType, len(ViolationTypes))
	for i := range ViolationTypes {
		m[ViolationTypes[i].Code] = &ViolationTypes[i]
	}
	return m
}()

// getViolationType returns the violation type by code
func getViolationType(code int) *ViolationType {
	return violationTypeMap[code]
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
	Revoked         bool     `json:"revoked,omitempty"` // 소명 인용 등으로 회수된 제재 (목록 배지용)
}

// DisciplineLogDetail represents detailed discipline log
type DisciplineLogDetail struct {
	ID                int             `json:"id"`
	MemberID          string          `json:"member_id"`
	MemberNickname    string          `json:"member_nickname"`
	PenaltyPeriod     int             `json:"penalty_period"`
	PenaltyDateFrom   string          `json:"penalty_date_from"`
	PenaltyDateTo     *string         `json:"penalty_date_to,omitempty"`
	ViolationTypes    []ViolationType `json:"violation_types"`
	ReportedItems     []ReportedItem  `json:"reported_items,omitempty"`
	Memo              string          `json:"memo,omitempty"`
	MemberReason      string          `json:"member_reason,omitempty"`      // 회원 공개 사유 (운영자 입력 시에만 노출)
	PublicDescription string          `json:"public_description,omitempty"` // 외부 공개용 안내문 (운영자 입력 시에만 노출)
	CreatedBy         string          `json:"created_by"`
	CreatedAt         string          `json:"created_at"`
	ClaimPostID       *int            `json:"claim_post_id,omitempty"`
	// 소명 인용 등으로 회수된 경우 회수 일시만 공개. ⛔ revoked_by(운영자ID)·admin_memo(회수사유)는 비공개.
	RevokedAt *string `json:"revoked_at,omitempty"`
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

// parseReasonCodes parses admin_discipline_reasons JSON ("[21,22]" 또는 ["21","22"]) into int codes.
func parseReasonCodes(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var ints []int
	if err := json.Unmarshal([]byte(s), &ints); err == nil {
		return ints
	}
	var strs []string
	if err := json.Unmarshal([]byte(s), &strs); err == nil {
		out := make([]int, 0, len(strs))
		for _, v := range strs {
			if n, convErr := strconv.Atoi(strings.TrimSpace(v)); convErr == nil {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
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
		// Filter by member_id using generated column (indexed)
		table := "g5_write_disciplinelog"
		filter := "wr_is_comment = 0 AND wr_deleted_at IS NULL AND penalty_mb_id = ?"

		h.db.Table(table).Where(filter, memberID).Count(&total)
		h.db.Table(table).Select(disciplineLogColumns).Where(filter, memberID).
			Order("wr_id DESC").Offset(offset).Limit(limit).Find(&posts)
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
			Revoked:         data.RevokedAt != "",
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

	// reported_items 항목별 "운영자가 적용한 사유"/일수/메모 구성.
	// 우선순위 1: wr_content에 항목별 값이 기록된 신규 글은 그대로 사용.
	// 우선순위 2: 레거시 글은 g5_na_singo의 admin_discipline_reasons(적용 사유)/days/detail로 보강.
	// 적용 사유를 찾지 못하면 항목별 사유 배지를 비워 둔다(상단 "제재 사유"가 적용 사유를 표시).
	// 절대 신고자 신고 사유(sg_type)로 폴백하지 않는다 — 운영자 결정과 다를 수 있어 오해 소지가 큼.
	reportedItems := make([]ReportedItem, 0, len(data.ReportedItems))
	for _, item := range data.ReportedItems {
		ri := ReportedItem{
			Table:       item.Table,
			ID:          item.ID,
			Parent:      item.Parent,
			SgTypes:     item.SgTypes,
			PenaltyDays: item.PenaltyDays,
			Memo:        item.Memo,
		}
		if len(ri.SgTypes) == 0 {
			// 적용 사유/일수/메모를 처분 연결(discipline_log_id)로 조회
			var row struct {
				Reasons *string `gorm:"column:admin_discipline_reasons"`
				Days    *int    `gorm:"column:admin_discipline_days"`
				Detail  *string `gorm:"column:admin_discipline_detail"`
			}
			h.db.Raw(`
				SELECT admin_discipline_reasons, admin_discipline_days, admin_discipline_detail
				FROM g5_na_singo
				WHERE discipline_log_id = ? AND sg_table = ? AND sg_id = ? AND admin_approved = 1
				ORDER BY id DESC LIMIT 1
			`, post.WrID, item.Table, item.ID).Scan(&row)

			if row.Reasons != nil {
				ri.SgTypes = parseReasonCodes(*row.Reasons)
			}
			if ri.PenaltyDays == nil && row.Days != nil {
				ri.PenaltyDays = row.Days
			}
			if ri.Memo == "" && row.Detail != nil {
				ri.Memo = *row.Detail
			}
		}
		reportedItems = append(reportedItems, ri)
	}

	// 신고 접수 후 삭제된 글 여부를 보드별 배치 조회로 판정(N+1 회피).
	// 행이 없으면(하드삭제) 또는 wr_deleted_at가 NULL/'0000-00-00'이 아니면 삭제된 글로 표시.
	if len(reportedItems) > 0 {
		// 보드별 wr_id 수집
		idsByTable := make(map[string][]int)
		for _, ri := range reportedItems {
			if ri.Table == "" || ri.ID == 0 || !disciplineBoardSlugRe.MatchString(ri.Table) {
				continue
			}
			idsByTable[ri.Table] = append(idsByTable[ri.Table], ri.ID)
		}

		delKey := func(table string, id int) string { return table + ":" + strconv.Itoa(id) }
		foundMap := make(map[string]bool)   // 조회 대상이고 행이 존재
		deletedMap := make(map[string]bool) // 행이 존재하고 소프트삭제됨
		for table, ids := range idsByTable {
			var rows []struct {
				WrID      int     `gorm:"column:wr_id"`
				DeletedAt *string `gorm:"column:wr_deleted_at"`
			}
			if err := h.db.Table("g5_write_"+table).
				Select("wr_id, wr_deleted_at").
				Where("wr_id IN ?", ids).
				Scan(&rows).Error; err != nil {
				// 조회 실패(없는 보드 등)는 안전 폴백(삭제 표시 안 함) + 로깅
				log.Printf("[disciplinelog] 삭제여부 조회 실패 log_id=%d table=%s: %v", id, table, err)
				continue
			}
			for _, r := range rows {
				foundMap[delKey(table, r.WrID)] = true
				if r.DeletedAt != nil && *r.DeletedAt != "" && *r.DeletedAt != "0000-00-00 00:00:00" {
					deletedMap[delKey(table, r.WrID)] = true
				}
			}
		}

		for i := range reportedItems {
			ri := &reportedItems[i]
			if ri.Table == "" || ri.ID == 0 || !disciplineBoardSlugRe.MatchString(ri.Table) {
				continue // 조회 대상이 아니면 폴백(Deleted=false)
			}
			key := delKey(ri.Table, ri.ID)
			if deletedMap[key] || !foundMap[key] {
				// 소프트삭제됐거나, 조회 대상이었는데 행이 없으면(하드삭제) 삭제로 표시
				ri.Deleted = true
			}
		}
	}

	detail := DisciplineLogDetail{
		ID:                post.WrID,
		MemberID:          data.PenaltyMbID,
		MemberNickname:    getMemberNickFromTitle(post.WrSubject),
		PenaltyPeriod:     data.PenaltyPeriod,
		PenaltyDateFrom:   data.PenaltyDateFrom,
		PenaltyDateTo:     penaltyDateTo,
		ViolationTypes:    violations,
		ReportedItems:     reportedItems,
		MemberReason:      data.MemberReason,
		PublicDescription: data.PublicDescription,
		CreatedBy:         post.MbID,
		CreatedAt:         post.WrDatetime.Format("2006-01-02 15:04:05"),
	}

	// 소명 인용 등으로 회수된 제재는 회수 일시만 공개 (revoked_by·admin_memo는 비공개)
	if data.RevokedAt != "" {
		detail.RevokedAt = &data.RevokedAt
	}

	// 소명글 존재 여부 조회 (claim 게시판에서 wr_link1 또는 wr_content 매칭)
	var claimPostID int
	linkColon := "disciplinelog:" + strconv.Itoa(id)
	linkSlash := "disciplinelog/" + strconv.Itoa(id)
	linkLike := "%disciplinelog/" + strconv.Itoa(id)
	contentLikeRel := `%href="/disciplinelog/` + strconv.Itoa(id) + `"%`
	contentLikeFull := `%href="https://damoang.net/disciplinelog/` + strconv.Itoa(id) + `"%`
	err = h.db.Table("g5_write_claim").
		Select("wr_id").
		Where("(wr_link1 = ? OR wr_link1 = ? OR wr_link1 LIKE ? OR wr_content LIKE ? OR wr_content LIKE ?) AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", linkColon, linkSlash, linkLike, contentLikeRel, contentLikeFull).
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
