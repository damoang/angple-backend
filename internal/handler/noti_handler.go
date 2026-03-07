package handler

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// NotiHandler handles /api/v1/notifications endpoints using g5_na_noti
type NotiHandler struct {
	repo gnurepo.NotiRepository
}

// NewNotiHandler creates a new NotiHandler
func NewNotiHandler(repo gnurepo.NotiRepository) *NotiHandler {
	return &NotiHandler{repo: repo}
}

// v1NotificationResponse matches frontend Notification type
type v1NotificationResponse struct {
	ID            int    `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	URL           string `json:"url,omitempty"`
	SenderID      string `json:"sender_id,omitempty"`
	SenderName    string `json:"sender_name,omitempty"`
	IsRead        bool   `json:"is_read"`
	CreatedAt     string `json:"created_at"`
	ParentSubject string `json:"parent_subject,omitempty"`
}

// v1NotificationListResponse matches frontend NotificationListResponse type
type v1NotificationListResponse struct {
	Items       []v1NotificationResponse `json:"items"`
	Total       int64                    `json:"total"`
	UnreadCount int64                    `json:"unread_count"`
	Page        int                      `json:"page"`
	Limit       int                      `json:"limit"`
	TotalPages  int64                    `json:"total_pages"`
}

// mapFromCase maps ph_from_case to frontend NotificationType
func mapFromCase(fromCase string) string {
	switch fromCase {
	case "board":
		return "comment"
	case "comment", "reply":
		return "reply"
	case "mention":
		return "mention"
	case "good", "nogood":
		return "like"
	case "write", "inquire", "answer":
		return "system"
	default:
		return "system"
	}
}

// generateTitle generates a notification title based on ph_from_case
func generateTitle(fromCase, relMbNick string) string {
	switch fromCase {
	case "board":
		return relMbNick + "님이 댓글을 달았습니다"
	case "comment", "reply":
		return relMbNick + "님이 답글을 달았습니다"
	case "mention":
		return relMbNick + "님이 회원님을 멘션했습니다"
	case "write":
		return relMbNick + "님이 새 글을 작성했습니다"
	case "good":
		return relMbNick + "님이 추천했습니다"
	case "nogood":
		return "게시글이 비추천을 받았습니다"
	case "inquire":
		return "새 문의가 등록되었습니다"
	case "answer":
		return "문의에 답변이 등록되었습니다"
	default:
		return "새 알림이 있습니다"
	}
}

// convertLegacyURL converts Gnuboard PHP URLs to SvelteKit URLs
// /bbs/board.php?bo_table=free&wr_id=123#c_456 → /free/123#c_456
func convertLegacyURL(rawURL string) string {
	if !strings.Contains(rawURL, "/bbs/board.php") {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := parsed.Query()
	boTable := q.Get("bo_table")
	wrID := q.Get("wr_id")
	if boTable == "" || wrID == "" {
		return rawURL
	}

	result := fmt.Sprintf("/%s/%s", boTable, wrID)
	if parsed.Fragment != "" {
		result += "#" + parsed.Fragment
	}
	return result
}

func toV1Notification(n gnurepo.Notification) v1NotificationResponse {
	return v1NotificationResponse{
		ID:            n.PhID,
		Type:          mapFromCase(n.PhFromCase),
		Title:         generateTitle(n.PhFromCase, n.RelMbNick),
		Content:       n.RelMsg,
		URL:           convertLegacyURL(n.RelURL),
		SenderID:      n.RelMbID,
		SenderName:    n.RelMbNick,
		IsRead:        n.PhReaded == "Y",
		CreatedAt:     n.PhDatetime.Format(time.RFC3339),
		ParentSubject: n.ParentSubject,
	}
}

// GetUnreadCount handles GET /api/v1/notifications/unread-count
func (h *NotiHandler) GetUnreadCount(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	count, err := h.repo.CountUnread(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "미읽음 알림 수 조회 실패", err)
		return
	}
	common.V2Success(c, gin.H{"total_unread": count})
}

// GetNotifications handles GET /api/v1/notifications
func (h *NotiHandler) GetNotifications(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	notifications, total, err := h.repo.GetNotifications(mbID, page, limit)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 목록 조회 실패", err)
		return
	}

	unreadCount, _ := h.repo.CountUnread(mbID)

	items := make([]v1NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		items = append(items, toV1Notification(n))
	}

	totalPages := int64(math.Ceil(float64(total) / float64(limit)))

	common.V2Success(c, v1NotificationListResponse{
		Items:       items,
		Total:       total,
		UnreadCount: unreadCount,
		Page:        page,
		Limit:       limit,
		TotalPages:  totalPages,
	})
}

// MarkAsRead handles POST /api/v1/notifications/:id/read
func (h *NotiHandler) MarkAsRead(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 알림 ID", err)
		return
	}

	if err := h.repo.MarkAsRead(mbID, id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 읽음 처리 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "읽음 처리 완료"})
}

// MarkAllAsRead handles POST /api/v1/notifications/read-all
func (h *NotiHandler) MarkAllAsRead(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	if err := h.repo.MarkAllAsRead(mbID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "전체 읽음 처리 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "전체 읽음 처리 완료"})
}

// Delete handles DELETE /api/v1/notifications/:id
func (h *NotiHandler) Delete(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 알림 ID", err)
		return
	}

	if err := h.repo.Delete(mbID, id); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}
