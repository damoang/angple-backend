package v2

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// ExpHandler handles experience point-related endpoints
type ExpHandler struct {
	expRepo         v2repo.ExpRepository
	notiRepo        gnurepo.NotiRepository
	pointConfigRepo v2repo.PointConfigRepository
}

// NewExpHandler creates a new ExpHandler
func NewExpHandler(expRepo v2repo.ExpRepository) *ExpHandler {
	return &ExpHandler{expRepo: expRepo}
}

// SetNotiRepository sets the optional notification repository for level-up notifications
func (h *ExpHandler) SetNotiRepository(repo gnurepo.NotiRepository) {
	h.notiRepo = repo
}

// SetPointConfigRepository sets the point config repository for point expiry settings
func (h *ExpHandler) SetPointConfigRepository(repo v2repo.PointConfigRepository) {
	h.pointConfigRepo = repo
}

// createLevelUpNotification inserts a level-up notification into g5_na_noti
func (h *ExpHandler) createLevelUpNotification(mbID string, newLevel int) {
	if h.notiRepo == nil {
		return
	}
	noti := &gnurepo.Notification{
		MbID:          mbID,
		PhFromCase:    "levelup",
		PhToCase:      "me",
		BoTable:       "@system",
		WrID:          0,
		RelMbID:       "system",
		RelMbNick:     "시스템",
		RelMsg:        fmt.Sprintf("레벨 %d로 승급했습니다!", newLevel),
		RelURL:        "/mypage/exp",
		PhReaded:      "N",
		ParentSubject: fmt.Sprintf("레벨 %d 달성", newLevel),
	}
	if err := h.notiRepo.Create(noti); err != nil {
		log.Printf("[xp] levelup notification failed for %s: %v", mbID, err)
	}
}

// GetExpSummary handles GET /api/v1/my/exp
func (h *ExpHandler) GetExpSummary(c *gin.Context) {
	mbID := middleware.GetUserID(c) // mb_id from JWT
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	summary, err := h.expRepo.GetSummary(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 조회에 실패했습니다", err)
		return
	}

	common.V2Success(c, summary)
}

// GetExpHistory handles GET /api/v1/my/exp/history
func (h *ExpHandler) GetExpHistory(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	history, total, err := h.expRepo.GetHistory(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 내역 조회에 실패했습니다", err)
		return
	}

	// Get summary as well
	summary, _ := h.expRepo.GetSummary(mbID)

	totalPages := (int(total) + limit - 1) / limit

	common.V2Success(c, gin.H{
		"summary": summary,
		"items":   history,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// ========================================
// Admin XP Management Handlers
// ========================================

// AdminListMemberXP handles GET /api/v2/admin/xp/members
func (h *ExpHandler) AdminListMemberXP(c *gin.Context) {
	search := c.Query("search")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	members, total, err := h.expRepo.ListMembersWithXP(search, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "회원 경험치 목록 조회에 실패했습니다", err)
		return
	}

	totalPages := (int(total) + limit - 1) / limit

	common.V2Success(c, gin.H{
		"members": members,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// AdminGetMemberXPHistory handles GET /api/v2/admin/xp/members/:mbId/history
func (h *ExpHandler) AdminGetMemberXPHistory(c *gin.Context) {
	mbID := c.Param("mbId")
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "회원 ID가 필요합니다", nil)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	history, total, err := h.expRepo.GetHistory(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 내역 조회에 실패했습니다", err)
		return
	}

	summary, _ := h.expRepo.GetSummary(mbID)

	totalPages := (int(total) + limit - 1) / limit

	common.V2Success(c, gin.H{
		"summary": summary,
		"items":   history,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// adminGrantXPRequest represents the request body for manual XP grant
type adminGrantXPRequest struct {
	Point   int    `json:"point" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// AdminGrantXP handles POST /api/v2/admin/xp/members/:mbId/grant
func (h *ExpHandler) AdminGrantXP(c *gin.Context) {
	mbID := c.Param("mbId")
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "회원 ID가 필요합니다", nil)
		return
	}

	var req adminGrantXPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	if req.Point == 0 {
		common.V2ErrorResponse(c, http.StatusBadRequest, "경험치는 0이 될 수 없습니다", nil)
		return
	}

	adminID := middleware.GetUserID(c)
	today := time.Now().Format("2006-01-02")
	relID := "admin-" + adminID + "-" + today

	result, err := h.expRepo.AddExp(mbID, req.Point, req.Content, "@admin", relID, "@admin-grant")
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "경험치 지급에 실패했습니다", err)
		return
	}

	if result.LevelUp {
		go h.createLevelUpNotification(mbID, result.NewLevel)
	}

	common.V2Success(c, gin.H{
		"message":  "경험치가 지급되었습니다",
		"mb_id":    mbID,
		"point":    req.Point,
		"level_up": result.LevelUp,
	})
}

// AdminGetXPConfig handles GET /api/v2/admin/xp/config
func (h *ExpHandler) AdminGetXPConfig(c *gin.Context) {
	config, err := h.expRepo.GetXPConfig()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회에 실패했습니다", err)
		return
	}
	common.V2Success(c, config)
}

// adminUpdateXPConfigRequest represents the request body for updating XP config
// Uses pointers for partial update (nil = keep existing value)
type adminUpdateXPConfigRequest struct {
	LoginXP        *int  `json:"login_xp"`
	WriteXP        *int  `json:"write_xp"`
	CommentXP      *int  `json:"comment_xp"`
	LoginEnabled   *bool `json:"login_enabled"`
	WriteEnabled   *bool `json:"write_enabled"`
	CommentEnabled *bool `json:"comment_enabled"`
}

// AdminUpdateXPConfig handles PUT /api/v2/admin/xp/config
func (h *ExpHandler) AdminUpdateXPConfig(c *gin.Context) {
	var req adminUpdateXPConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	// Load existing config to merge
	existing, err := h.expRepo.GetXPConfig()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회에 실패했습니다", err)
		return
	}

	// Partial update: only override non-nil fields
	if req.LoginXP != nil {
		if *req.LoginXP < 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "경험치는 0 이상이어야 합니다", nil)
			return
		}
		existing.LoginXP = *req.LoginXP
	}
	if req.WriteXP != nil {
		if *req.WriteXP < 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "경험치는 0 이상이어야 합니다", nil)
			return
		}
		existing.WriteXP = *req.WriteXP
	}
	if req.CommentXP != nil {
		if *req.CommentXP < 0 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "경험치는 0 이상이어야 합니다", nil)
			return
		}
		existing.CommentXP = *req.CommentXP
	}
	if req.LoginEnabled != nil {
		existing.LoginEnabled = *req.LoginEnabled
	}
	if req.WriteEnabled != nil {
		existing.WriteEnabled = *req.WriteEnabled
	}
	if req.CommentEnabled != nil {
		existing.CommentEnabled = *req.CommentEnabled
	}

	if err := h.expRepo.UpdateXPConfig(existing); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 저장에 실패했습니다", err)
		return
	}

	common.V2Success(c, existing)
}

// ========================================
// Admin Point Config Handlers
// ========================================

// AdminGetPointConfig handles GET /api/v2/admin/point/config
func (h *ExpHandler) AdminGetPointConfig(c *gin.Context) {
	if h.pointConfigRepo == nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 설정 기능이 비활성화되어 있습니다", nil)
		return
	}

	config, err := h.pointConfigRepo.GetPointConfig()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회에 실패했습니다", err)
		return
	}
	common.V2Success(c, config)
}

// adminUpdatePointConfigRequest represents the request body for updating point config
type adminUpdatePointConfigRequest struct {
	ExpiryEnabled *bool `json:"expiry_enabled"`
	ExpiryDays    *int  `json:"expiry_days"`
}

// AdminUpdatePointConfig handles PUT /api/v2/admin/point/config
func (h *ExpHandler) AdminUpdatePointConfig(c *gin.Context) {
	if h.pointConfigRepo == nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "포인트 설정 기능이 비활성화되어 있습니다", nil)
		return
	}

	var req adminUpdatePointConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청입니다", err)
		return
	}

	existing, err := h.pointConfigRepo.GetPointConfig()
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 조회에 실패했습니다", err)
		return
	}

	if req.ExpiryEnabled != nil {
		existing.ExpiryEnabled = *req.ExpiryEnabled
	}
	if req.ExpiryDays != nil {
		if *req.ExpiryDays < 1 {
			common.V2ErrorResponse(c, http.StatusBadRequest, "유효기간은 1일 이상이어야 합니다", nil)
			return
		}
		existing.ExpiryDays = *req.ExpiryDays
	}

	if err := h.pointConfigRepo.UpdatePointConfig(existing); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정 저장에 실패했습니다", err)
		return
	}

	common.V2Success(c, existing)
}
