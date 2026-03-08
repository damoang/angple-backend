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
	repo     gnurepo.NotiRepository
	prefRepo gnurepo.NotiPreferenceRepository
}

// NewNotiHandler creates a new NotiHandler
func NewNotiHandler(repo gnurepo.NotiRepository, prefRepo gnurepo.NotiPreferenceRepository) *NotiHandler {
	return &NotiHandler{repo: repo, prefRepo: prefRepo}
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

// groupedNotificationResponse is the response for a single grouped notification
type groupedNotificationResponse struct {
	Type          string   `json:"type"`
	BoTable       string   `json:"bo_table"`
	WrID          int      `json:"wr_id"`
	Title         string   `json:"title"`
	URL           string   `json:"url,omitempty"`
	ParentSubject string   `json:"parent_subject,omitempty"`
	Content       string   `json:"content,omitempty"`
	LatestSender  string   `json:"latest_sender"`
	Senders       []string `json:"senders"`
	SenderCount   int      `json:"sender_count"`
	UnreadCount   int      `json:"unread_count"`
	HasUnread     bool     `json:"has_unread"`
	LatestAt      string   `json:"latest_at"`
	FromCase      string   `json:"from_case"`
}

// groupedNotificationListResponse is the paginated response for grouped notifications
type groupedNotificationListResponse struct {
	Items       []groupedNotificationResponse `json:"items"`
	Total       int64                         `json:"total"`
	UnreadCount int64                         `json:"unread_count"`
	Page        int                           `json:"page"`
	Limit       int                           `json:"limit"`
	TotalPages  int64                         `json:"total_pages"`
}

// generateGroupTitle generates a title for grouped notifications
func generateGroupTitle(fromCase, latestSender string, senderCount int) string {
	action := ""
	switch fromCase {
	case "board":
		action = "댓글을 달았습니다"
	case "comment", "reply":
		action = "답글을 달았습니다"
	case "mention":
		action = "회원님을 멘션했습니다"
	case "good":
		action = "추천했습니다"
	case "nogood":
		action = "비추천했습니다"
	case "write":
		action = "새 글을 작성했습니다"
	case "inquire":
		return "새 문의가 등록되었습니다"
	case "answer":
		return "문의에 답변이 등록되었습니다"
	default:
		return "새 알림이 있습니다"
	}

	if senderCount > 1 {
		return fmt.Sprintf("%s님 외 %d명이 %s", latestSender, senderCount-1, action)
	}
	return fmt.Sprintf("%s님이 %s", latestSender, action)
}

// GetGroupedNotifications handles GET /api/v1/notifications/grouped
func (h *NotiHandler) GetGroupedNotifications(c *gin.Context) {
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
	filterType := c.DefaultQuery("type", "") // "", "comment", "like", "mention", "system"

	groups, totalGroups, unreadCount, err := h.repo.GetGroupedNotifications(mbID, page, limit, filterType)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 목록 조회 실패", err)
		return
	}

	items := make([]groupedNotificationResponse, 0, len(groups))
	for _, g := range groups {
		senders := strings.Split(g.Senders, "||")
		// Remove empty strings
		filteredSenders := make([]string, 0, len(senders))
		for _, s := range senders {
			if s != "" {
				filteredSenders = append(filteredSenders, s)
			}
		}

		items = append(items, groupedNotificationResponse{
			Type:          mapFromCase(g.PhFromCase),
			BoTable:       g.BoTable,
			WrID:          g.WrID,
			Title:         generateGroupTitle(g.PhFromCase, g.LatestSender, g.SenderCount),
			URL:           convertLegacyURL(g.RelURL),
			ParentSubject: g.ParentSubject,
			Content:       g.RelMsg,
			LatestSender:  g.LatestSender,
			Senders:       filteredSenders,
			SenderCount:   g.SenderCount,
			UnreadCount:   g.UnreadCount,
			HasUnread:     g.UnreadCount > 0,
			LatestAt:      g.LatestAt.Format(time.RFC3339),
			FromCase:      g.PhFromCase,
		})
	}

	totalPages := int64(math.Ceil(float64(totalGroups) / float64(limit)))

	common.V2Success(c, groupedNotificationListResponse{
		Items:       items,
		Total:       totalGroups,
		UnreadCount: unreadCount,
		Page:        page,
		Limit:       limit,
		TotalPages:  totalPages,
	})
}

// MarkGroupAsRead handles POST /api/v1/notifications/group/read
func (h *NotiHandler) MarkGroupAsRead(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var req struct {
		BoTable  string `json:"bo_table"`
		WrID     int    `json:"wr_id"`
		FromCase string `json:"from_case"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	if err := h.repo.MarkGroupAsRead(mbID, req.BoTable, req.WrID, req.FromCase); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "그룹 읽음 처리 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "그룹 읽음 처리 완료"})
}

// notiPreferenceResponse is the response for notification preferences
type notiPreferenceResponse struct {
	NotiComment   bool `json:"noti_comment"`
	NotiReply     bool `json:"noti_reply"`
	NotiMention   bool `json:"noti_mention"`
	NotiLike      bool `json:"noti_like"`
	NotiFollow    bool `json:"noti_follow"`
	LikeThreshold int  `json:"like_threshold"`
}

// GetPreferences handles GET /api/v1/notifications/preferences
func (h *NotiHandler) GetPreferences(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	pref, err := h.prefRepo.Get(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 설정 조회 실패", err)
		return
	}

	common.V2Success(c, notiPreferenceResponse{
		NotiComment:   pref.NotiComment,
		NotiReply:     pref.NotiReply,
		NotiMention:   pref.NotiMention,
		NotiLike:      pref.NotiLike,
		NotiFollow:    pref.NotiFollow,
		LikeThreshold: pref.LikeThreshold,
	})
}

// UpdatePreferences handles PUT /api/v1/notifications/preferences
func (h *NotiHandler) UpdatePreferences(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var req struct {
		NotiComment   *bool `json:"noti_comment"`
		NotiReply     *bool `json:"noti_reply"`
		NotiMention   *bool `json:"noti_mention"`
		NotiLike      *bool `json:"noti_like"`
		NotiFollow    *bool `json:"noti_follow"`
		LikeThreshold *int  `json:"like_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	pref, err := h.prefRepo.Get(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 설정 조회 실패", err)
		return
	}

	if req.NotiComment != nil {
		pref.NotiComment = *req.NotiComment
	}
	if req.NotiReply != nil {
		pref.NotiReply = *req.NotiReply
	}
	if req.NotiMention != nil {
		pref.NotiMention = *req.NotiMention
	}
	if req.NotiLike != nil {
		pref.NotiLike = *req.NotiLike
	}
	if req.NotiFollow != nil {
		pref.NotiFollow = *req.NotiFollow
	}
	if req.LikeThreshold != nil {
		if *req.LikeThreshold < 1 {
			*req.LikeThreshold = 1
		}
		pref.LikeThreshold = *req.LikeThreshold
	}

	if err := h.prefRepo.Upsert(pref); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 설정 저장 실패", err)
		return
	}

	common.V2Success(c, notiPreferenceResponse{
		NotiComment:   pref.NotiComment,
		NotiReply:     pref.NotiReply,
		NotiMention:   pref.NotiMention,
		NotiLike:      pref.NotiLike,
		NotiFollow:    pref.NotiFollow,
		LikeThreshold: pref.LikeThreshold,
	})
}

// DeleteGroup handles DELETE /api/v1/notifications/group
func (h *NotiHandler) DeleteGroup(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	boTable := c.Query("bo_table")
	wrID, _ := strconv.Atoi(c.Query("wr_id"))
	fromCase := c.Query("from_case")

	if boTable == "" || fromCase == "" {
		common.V2ErrorResponse(c, http.StatusBadRequest, "필수 파라미터 누락", nil)
		return
	}

	if err := h.repo.DeleteGroup(mbID, boTable, wrID, fromCase); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "그룹 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "그룹 삭제 완료"})
}
