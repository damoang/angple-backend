package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
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
	MbInterceptDate *string `json:"mb_intercept_date,omitempty"`
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
	}
	if m.MbTodayLogin != "" && m.MbTodayLogin != "0000-00-00" {
		resp.MbTodayLogin = &m.MbTodayLogin
	}
	if m.MbLeaveDate != "" {
		resp.MbLeaveDate = &m.MbLeaveDate
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
func (h *AdminMemberHandler) UpdateMember(c *gin.Context) {
	mbID := c.Param("mbId")
	var req struct {
		MbLevel     *int    `json:"mb_level"`
		MbPoint     *int    `json:"mb_point"`
		MbName      *string `json:"mb_name"`
		MbRealName  *string `json:"mb_real_name"`
		MbEmail     *string `json:"mb_email"`
		MbSignature *string `json:"mb_signature"`
		MbMemo      *string `json:"mb_memo"`
		MbLeave     *bool   `json:"mb_leave"`
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
			updates["mb_leave_date"] = time.Now().Format("2006-01-02")
		} else {
			updates["mb_leave_date"] = ""
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
	common.V2Success(c, gin.H{"message": "수정 완료"})
}

// BanMember handles POST /api/v1/admin/members/:id/ban
func (h *AdminMemberHandler) BanMember(c *gin.Context) {
	mbID := c.Param("mbId")
	now := time.Now().Format("2006-01-02")
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
	common.V2Success(c, gin.H{"message": fmt.Sprintf("%d명 레벨 변경 완료", len(req.MemberIDs))})
}
