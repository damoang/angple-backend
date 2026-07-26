package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminMemberHandler handles admin member management (g5_member based)
type AdminMemberHandler struct {
	db *gorm.DB
}

// NewAdminMemberHandler creates a new AdminMemberHandler
func NewAdminMemberHandler(db *gorm.DB) *AdminMemberHandler {
	return &AdminMemberHandler{db: db}
}

// adminMemberResponse is the response DTO matching frontend AdminMember type
type adminMemberResponse struct {
	MbID            string  `json:"mb_id"`
	MbName          string  `json:"mb_name"`
	MbRealName      string  `json:"mb_real_name"`
	MbEmail         string  `json:"mb_email"`
	MbLevel         int     `json:"mb_level"`
	MbPoint         int     `json:"mb_point"`
	MbSignature     string  `json:"mb_signature"`
	MbMemo          string  `json:"mb_memo"`
	MbDatetime      string  `json:"mb_datetime"`
	MbTodayLogin    *string `json:"mb_today_login"`
	MbImage         *string `json:"mb_image,omitempty"`
	MbLeaveDate     *string `json:"mb_leave_date,omitempty"`
	MbLeaveReason   *string `json:"mb_leave_reason,omitempty"`
	MbInterceptDate *string `json:"mb_intercept_date,omitempty"`
	// 실명인증 상태. 빈 문자열이면 미인증. 관리 화면이 인증/해제 버튼을 고르는 근거다.
	MbCertify string `json:"mb_certify"`
	// 명의 중복 검사(DI) 보유 여부. ⛔ DI 원본은 절대 내려보내지 않는다 — 존재 여부만.
	// 수동 인증은 DI 를 만들지 않으므로, 인증돼 있어도 이 값이 false 일 수 있다.
	HasDupinfo bool `json:"has_dupinfo"`
}

func toAdminMemberResponse(m *gnuboard.G5Member) adminMemberResponse {
	resp := adminMemberResponse{
		MbID:        m.MbID,
		MbName:      m.MbNick,
		MbRealName:  m.MbName,
		MbEmail:     m.MbEmail,
		MbLevel:     m.MbLevel,
		MbPoint:     m.MbPoint,
		MbSignature: m.MbSignature,
		MbMemo:      m.MbMemo,
		MbDatetime:  m.MbDatetime.Format(time.RFC3339),
		MbCertify:   m.MbCertify,
		HasDupinfo:  strings.TrimSpace(m.MbDupInfo) != "",
	}
	if m.MbTodayLogin != "" && m.MbTodayLogin != "0000-00-00" {
		resp.MbTodayLogin = &m.MbTodayLogin
	}
	if m.MbLeaveDate != "" {
		resp.MbLeaveDate = &m.MbLeaveDate
		if m.MbLeaveReason != "" {
			resp.MbLeaveReason = &m.MbLeaveReason
		}
	}
	if m.MbInterceptDate != "" {
		resp.MbInterceptDate = &m.MbInterceptDate
	}
	if m.MbImagePath != "" {
		resp.MbImage = &m.MbImagePath
	}
	return resp
}

// allowedSortColumns maps sort_by param to g5_member columns
var allowedSortColumns = map[string]string{
	"datetime": "mb_datetime",
	"name":     "mb_nick",
	"level":    "mb_level",
	"point":    "mb_point",
	"login":    "mb_today_login",
}

// ListMembers handles GET /api/v1/admin/members
//
//nolint:gocyclo // Admin filtering stays explicit to keep parameter-to-query mapping easy to audit.
func (h *AdminMemberHandler) ListMembers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := h.db.Model(&gnuboard.G5Member{})

	// 검색
	search := c.Query("search")
	searchField := c.DefaultQuery("search_field", "name")
	if search != "" {
		like := "%" + search + "%"
		switch searchField {
		case "email":
			query = query.Where("mb_email LIKE ?", like)
		case "id":
			query = query.Where("mb_id LIKE ?", like)
		default: // name
			query = query.Where("mb_nick LIKE ?", like)
		}
	}

	// 레벨 필터
	if levelStr := c.Query("level"); levelStr != "" {
		if level, err := strconv.Atoi(levelStr); err == nil {
			query = query.Where("mb_level = ?", level)
		}
	}

	// 상태 필터
	status := c.Query("status")
	switch status {
	case "banned":
		query = query.Where("mb_intercept_date != ''")
	case "left":
		query = query.Where("mb_leave_date != ''")
	case "active":
		query = query.Where("mb_intercept_date = '' AND mb_leave_date = ''")
	}

	// 가입일 범위
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		query = query.Where("mb_datetime >= ?", dateFrom)
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		query = query.Where("mb_datetime <= ?", dateTo+" 23:59:59")
	}

	// 포인트 범위
	if pointMin := c.Query("point_min"); pointMin != "" {
		if v, err := strconv.Atoi(pointMin); err == nil {
			query = query.Where("mb_point >= ?", v)
		}
	}
	if pointMax := c.Query("point_max"); pointMax != "" {
		if v, err := strconv.Atoi(pointMax); err == nil {
			query = query.Where("mb_point <= ?", v)
		}
	}

	// 최근 로그인 범위
	if loginFrom := c.Query("login_from"); loginFrom != "" {
		query = query.Where("mb_today_login >= ?", loginFrom)
	}
	if loginTo := c.Query("login_to"); loginTo != "" {
		query = query.Where("mb_today_login <= ?", loginTo+" 23:59:59")
	}

	// 카운트
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 수 조회 실패", err)
		return
	}

	// 정렬
	sortBy := c.DefaultQuery("sort_by", "datetime")
	sortOrder := strings.ToUpper(c.DefaultQuery("sort_order", "desc"))
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}
	col, ok := allowedSortColumns[sortBy]
	if !ok {
		col = "mb_datetime"
	}
	query = query.Order(fmt.Sprintf("%s %s", col, sortOrder)) // #nosec G201 -- col and sortOrder are from allowedSortColumns whitelist, not user input

	// 조회
	offset := (page - 1) * limit
	var members []gnuboard.G5Member
	if err := query.Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 목록 조회 실패", err)
		return
	}

	// 응답 변환
	result := make([]adminMemberResponse, len(members))
	for i, m := range members {
		result[i] = toAdminMemberResponse(&m)
	}

	// 프론트엔드가 기대하는 형식: { data: { members, total, page, limit } }
	common.V2Success(c, gin.H{
		"members": result,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// GetMember handles GET /api/v1/admin/members/:id
func (h *AdminMemberHandler) GetMember(c *gin.Context) {
	mbID := c.Param("mbId")
	var member gnuboard.G5Member
	if err := h.db.Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}
	common.V2Success(c, toAdminMemberResponse(&member))
}

// UpdateMember handles PUT /api/v1/admin/members/:id
//
//nolint:gocyclo // Admin patch validation is clearer inline than split across many tiny helpers.
func (h *AdminMemberHandler) UpdateMember(c *gin.Context) {
	mbID := c.Param("mbId")
	var req struct {
		MbLevel       *int    `json:"mb_level"`
		MbPoint       *int    `json:"mb_point"`
		MbName        *string `json:"mb_name"`
		MbRealName    *string `json:"mb_real_name"`
		MbEmail       *string `json:"mb_email"`
		MbSignature   *string `json:"mb_signature"`
		MbMemo        *string `json:"mb_memo"`
		MbLeave       *bool   `json:"mb_leave"`
		MbLeaveReason *string `json:"mb_leave_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	updates := map[string]interface{}{}
	if req.MbLevel != nil {
		if *req.MbLevel < 1 || *req.MbLevel > 10 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "레벨은 1~10 범위여야 합니다", nil)
			return
		}
		updates["mb_level"] = *req.MbLevel
	}
	if req.MbPoint != nil {
		updates["mb_point"] = *req.MbPoint
	}
	if req.MbName != nil {
		name := strings.TrimSpace(*req.MbName)
		if name == "" {
			common.V2ErrorResponse(c, http.StatusBadRequest, "닉네임은 비워둘 수 없습니다", nil)
			return
		}
		updates["mb_nick"] = name
	}
	if req.MbRealName != nil {
		updates["mb_name"] = strings.TrimSpace(*req.MbRealName)
	}
	if req.MbEmail != nil {
		email := strings.TrimSpace(*req.MbEmail)
		if email == "" {
			common.V2ErrorResponse(c, http.StatusBadRequest, "이메일은 비워둘 수 없습니다", nil)
			return
		}
		updates["mb_email"] = email
	}
	if req.MbSignature != nil {
		updates["mb_signature"] = *req.MbSignature
	}
	if req.MbMemo != nil {
		updates["mb_memo"] = *req.MbMemo
	}
	if req.MbLeave != nil {
		if *req.MbLeave {
			updates["mb_leave_date"] = time.Now().Format("20060102")
			reason := "self"
			if req.MbLeaveReason != nil && *req.MbLeaveReason != "" {
				reason = *req.MbLeaveReason
			}
			updates["mb_leave_reason"] = reason
		} else {
			updates["mb_leave_date"] = ""
			updates["mb_leave_reason"] = ""
		}
	}
	if len(updates) == 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "변경할 항목이 없습니다", nil)
		return
	}

	if err := h.db.Model(&gnuboard.G5Member{}).Where("mb_id = ?", mbID).Updates(updates).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 수정 실패", err)
		return
	}
	// 레벨 변경 시 v2_users.level 미러 동기화 (이원 저장 desync 방지)
	if req.MbLevel != nil {
		if err := h.db.Table("v2_users").Where("username = ?", mbID).Update("level", *req.MbLevel).Error; err != nil {
			// 미러 동기화 실패는 관리자 작업 자체를 막지 않는다(best-effort). g5_member 는 이미 반영됨.
			log.Printf("[admin] v2_users level 동기화 스킵 (%s): %v", mbID, err)
		}
	}
	common.V2Success(c, gin.H{"message": "수정 완료"})
}

// BanMember handles POST /api/v1/admin/members/:id/ban
func (h *AdminMemberHandler) BanMember(c *gin.Context) {
	mbID := c.Param("mbId")
	now := time.Now().Format("20060102")
	if err := h.db.Model(&gnuboard.G5Member{}).Where("mb_id = ?", mbID).Update("mb_intercept_date", now).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "차단 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "차단 완료"})
}

// UnbanMember handles POST /api/v1/admin/members/:id/unban
func (h *AdminMemberHandler) UnbanMember(c *gin.Context) {
	mbID := c.Param("mbId")
	if err := h.db.Model(&gnuboard.G5Member{}).Where("mb_id = ?", mbID).Update("mb_intercept_date", "").Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "차단 해제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "차단 해제 완료"})
}

// BulkUpdateLevel handles POST /api/v1/admin/members/bulk/level
func (h *AdminMemberHandler) BulkUpdateLevel(c *gin.Context) {
	var req struct {
		MemberIDs []string `json:"member_ids" binding:"required"`
		Level     int      `json:"level" binding:"required,min=1,max=10"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if len(req.MemberIDs) == 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "선택된 회원이 없습니다", nil)
		return
	}
	if len(req.MemberIDs) > 100 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "최대 100명까지 일괄 변경 가능합니다", nil)
		return
	}
	if err := h.db.Model(&gnuboard.G5Member{}).Where("mb_id IN ?", req.MemberIDs).Update("mb_level", req.Level).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "일괄 레벨 변경 실패", err)
		return
	}
	// v2_users.level 미러 동기화 (이원 저장 desync 방지)
	if err := h.db.Table("v2_users").Where("username IN ?", req.MemberIDs).Update("level", req.Level).Error; err != nil {
		// 미러 동기화 실패는 일괄 작업 자체를 막지 않는다(best-effort). g5_member 는 이미 반영됨.
		log.Printf("[admin] v2_users level 일괄 동기화 스킵: %v", err)
	}
	common.V2Success(c, gin.H{"message": fmt.Sprintf("%d명 레벨 변경 완료", len(req.MemberIDs))})
}

// 수동 실명인증에서 쓰는 mb_certify 값.
//
// 기존 해외인증 회원 195명이 이미 이 값을 쓰고 있어 그대로 따른다(새 값을 만들면
// 과거 데이터와 통계가 갈린다). 인증 게이트들은 값의 종류를 보지 않고
// mb_certify != "" 만 검사하므로, 이 값 하나로 모든 인증 권한이 열린다.
const certifyManualValue = "abroad"

// 이 화면이 다룰 수 있는 상태는 "미인증" 과 "수동 인증" 둘뿐이다.
//
// ⛔ 블랙리스트(simple/ipin/hp 만 차단)로 두면 admin·email 같은 그 밖의 값이 뚫린다.
//
//	실제로 DB 에 admin 1명·email 1명이 있어 관리자가 덮어쓸 수 있었다.
//	허용 목록으로 뒤집어, 모르는 값은 전부 막는다.
func isManageableCertify(v string) bool {
	return v == "" || v == certifyManualValue
}

// CertifyMember handles POST /api/v1/admin/members/:mbId/certify
//
// 해외 앙님처럼 국내 휴대폰 인증이 불가능한 회원을 관리자가 직접 인증 처리한다.
// 지금까지는 DB 를 직접 고쳐 왔다(이 값을 설정하는 코드가 없었다).
//
// ⛔ 수동 인증은 DI(mb_dupinfo)를 만들지 않는다. 명의 중복 검사를 거치지 않은 채
//
//	인증 회원 권한(인증필수 게시판 6곳·쪽지 발송·자동 등급 승급)이 부여되므로
//	사유를 필수로 받고 감사 로그에 남긴다.
func (h *AdminMemberHandler) CertifyMember(c *gin.Context) {
	mbID := c.Param("mbId")

	var req struct {
		// true = 인증 부여, false = 인증 해제.
		// ⛔ 포인터로 받는다. bool 로 받으면 필드를 빠뜨린 요청이 false 로 해석되어
		//    의도치 않게 "해제"가 실행된다.
		Certify *bool `json:"certify"`
		// 왜 인증했는지. 나중에 "이 사람이 왜 인증됐나"를 추적할 유일한 단서다.
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}
	if req.Certify == nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "certify 값이 필요합니다", nil)
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) < 10 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "사유를 10자 이상 입력해주세요", nil)
		return
	}

	var member gnuboard.G5Member
	if err := h.db.Select("mb_id, mb_nick, mb_certify, mb_leave_date, mb_dupinfo").
		Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "회원을 찾을 수 없습니다", err)
		return
	}

	if strings.TrimSpace(member.MbLeaveDate) != "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "탈퇴한 회원은 인증 처리할 수 없습니다", nil)
		return
	}

	prev := strings.TrimSpace(member.MbCertify)

	// 실제 본인확인을 거친 값은 관리자가 건드리지 않는다.
	// 되돌릴 수 없는 정보(DI 동반 인증)를 화면 조작 한 번으로 지우면 안 된다.
	if !isManageableCertify(prev) {
		common.V2ErrorResponse(c, http.StatusConflict,
			"본인확인을 거친 회원("+prev+")은 이 화면에서 변경할 수 없습니다", nil)
		return
	}

	next := ""
	action := common.AuditCertifyManualClear
	if *req.Certify {
		next = certifyManualValue
		action = common.AuditCertifyManualSet
	}

	if prev == next {
		common.V2ErrorResponse(c, http.StatusBadRequest, "이미 같은 상태입니다", nil)
		return
	}

	if err := h.db.Model(&gnuboard.G5Member{}).
		Where("mb_id = ?", mbID).
		Update("mb_certify", next).Error; err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "인증 처리 실패", err)
		return
	}

	// 감사 로그는 best-effort — 기록 실패가 인증 처리를 되돌리지 않는다.
	common.WriteAudit(h.db, c, common.AuditEntry{
		// ⛔ GetUserID 는 Bearer 경로에서 v2_users.id(숫자)를 준다. 기존 감사 로그는
		//    전부 mb_id 라, 섞이면 행위자 추적이 깨진다. GetUsername 이 두 경로 모두 mb_id.
		UserID:     middleware.GetUsername(c),
		Action:     action,
		Resource:   "member",
		ResourceID: mbID,
		Details: map[string]any{
			"nickname": member.MbNick,
			"from":     prev,
			"to":       next,
			"reason":   reason,
			// 이 회원이 명의 중복 검사(DI)를 이미 갖고 있었는지 — 실제 값을 남긴다.
			// 미인증 회원 중에도 DI 보유자가 676명 있어 하드코딩하면 사실과 달라진다.
			"had_dupinfo": strings.TrimSpace(member.MbDupInfo) != "",
		},
	})

	msg := "인증 해제 완료"
	if *req.Certify {
		msg = "인증 처리 완료"
	}
	common.V2Success(c, gin.H{"message": msg, "mb_certify": next})
}
