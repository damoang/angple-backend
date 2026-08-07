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

// mapNotificationType maps a notification row to the frontend NotificationType.
//
// ⛔ ph_from_case 하나만 보면 안 된다(bug#13242). 30일 실측:
//
//	write   + subscribe      914,410  구독 게시판 새 글   ← 예전엔 전부 "system"(회색)
//	good    + good           740,872  글 공감 + 댓글 공감이 한 덩어리
//	comment + comment        131,461  내 글에 댓글        ← 예전엔 "reply"(답글)로 표시
//	write   + follow          97,405  팔로우한 분 새 글   ← 예전엔 "system"(회색)
//	comment + comment_reply   32,904  내 댓글에 답글
//
// write 계열만 101만건 = 전체의 52% 가 의미 없는 회색 아이콘이었다.
//
// 공감의 글/댓글은 ph_to_case 도 'good' 으로 같아서 wr_parent 로 가른다.
// 3일 실측에서 완전히 분리된다 — wr_parent==wr_id 106,236건은 전부 "글을 추천",
// wr_parent!=wr_id 68,127건은 전부 "댓글을 추천". rel_url 의 '#c_' 유무로 센
// 집합과도 정확히 일치한다(독립 신호 2개 합치).
//
// ⛔ "board" 는 실 데이터에 없다(30일 0건). 사문화됐지만 옛 행이 남아 있을 수 있어
// 지우지 않는다.
func mapNotificationType(fromCase, toCase string, wrID, wrParent int) string {
	switch fromCase {
	case "board":
		return "comment"
	case "comment", "reply":
		if toCase == "comment_reply" {
			return "reply"
		}
		// 내 '글'에 달린 댓글. 이게 comment 계열의 80% 다.
		return "comment"
	case "mention":
		return "mention"
	case "good":
		// wr_parent 가 0 이면 판별 불가 — 안전하게 글 공감으로 둔다.
		if wrParent != 0 && wrParent != wrID {
			return "like_comment"
		}
		return "like"
	case "nogood":
		return "like"
	case "memo":
		return "memo"
	case "digest":
		return "digest"
	case "levelup", "promote":
		return "levelup"
	case "point_expiry":
		return "point"
	case "write":
		if toCase == "follow" {
			return "follow"
		}
		return "subscribe"
	default:
		return "system"
	}
}

// generateTitle generates a notification title.
//
// ⛔ 문구를 창작하지 말 것. 여기 표현은 DB 의 rel_msg 가 실제로 쓰는 문장과 같게
// 맞춘 것이다. 예전에는 comment 계열을 모두 "답글을 달았습니다"로 썼는데,
// 실제로는 그중 80% 가 "회원님의 글에 댓글을 남겼습니다" 였다(bug#13242).
//
// 종류를 문장 뒤쪽에 두면 닉네임이 길 때 line-clamp 로 먼저 잘려 사라진다.
// 그래서 화면은 별도로 맨 앞에 종류 표식을 그린다 — 이 문자열은 정확성만 책임진다.
func generateTitle(fromCase, toCase, relMbNick string, wrID, wrParent int) string {
	switch fromCase {
	case "board":
		return relMbNick + "님이 회원님의 글에 댓글을 남겼습니다"
	case "comment", "reply":
		if toCase == "comment_reply" {
			return relMbNick + "님이 회원님의 댓글에 답글을 남겼습니다"
		}
		return relMbNick + "님이 회원님의 글에 댓글을 남겼습니다"
	case "mention":
		return relMbNick + "님이 회원님을 멘션했습니다"
	case "write":
		return relMbNick + "님이 새 글을 작성했습니다"
	case "digest":
		return "새 글 요약이 도착했습니다"
	case "good":
		if wrParent != 0 && wrParent != wrID {
			return relMbNick + "님이 회원님의 댓글을 추천했습니다"
		}
		return relMbNick + "님이 회원님의 글을 추천했습니다"
	case "nogood":
		return "게시글이 비추천을 받았습니다"
	case "memo":
		return relMbNick + "님이 쪽지를 보냈습니다"
	case "inquire":
		return "새 문의가 등록되었습니다"
	case "answer":
		return "문의에 답변이 등록되었습니다"
	default:
		return "새 알림이 있습니다"
	}
}

// NotificationTitle exposes generateTitle to other packages (push worker 등).
// 제목 문구의 단일 소스 — 다른 곳에 복제하지 말 것(bug#13242 재발 방지).
func NotificationTitle(fromCase, toCase, relMbNick string, wrID, wrParent int) string {
	return generateTitle(fromCase, toCase, relMbNick, wrID, wrParent)
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
		Type:          mapNotificationType(n.PhFromCase, n.PhToCase, n.WrID, n.WrParent),
		Title:         generateTitle(n.PhFromCase, n.PhToCase, n.RelMbNick, n.WrID, n.WrParent),
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

// MarkSeen 은 알림함 열람을 기록한다 — 뱃지만 0 이 되고 항목별 읽음은 그대로다.
// (seen/read 분리 — bug/13367·13332·13206·12991 체인. 레딧·페이스북과 같은 모델)
func (h *NotiHandler) MarkSeen(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}
	if err := h.repo.MarkSeen(mbID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "열람 기록 실패", err)
		return
	}
	common.V2Success(c, gin.H{"seen": true})
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
	// 병합 모드(merge=target) 전용 — 기본 모드 응답에는 실리지 않는다
	TargetKey    string `json:"target_key,omitempty"`
	GoodCount    int    `json:"good_count,omitempty"`
	CommentCount int    `json:"comment_count,omitempty"`
	ReplyCount   int    `json:"reply_count,omitempty"`
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
// ⛔ 문구는 generateTitle 과 같은 규칙이다(bug#13242) — 종류를 정확히 쓴다.
// 둘 중 하나만 고치면 묶음 알림과 낱개 알림이 서로 다른 말을 한다.
func generateGroupTitle(fromCase, toCase, latestSender string, senderCount, wrID, wrParent int) string {
	action := ""
	switch fromCase {
	case "board":
		action = "회원님의 글에 댓글을 남겼습니다"
	case "comment", "reply":
		if toCase == "comment_reply" {
			action = "회원님의 댓글에 답글을 남겼습니다"
		} else {
			action = "회원님의 글에 댓글을 남겼습니다"
		}
	case "mention":
		action = "회원님을 멘션했습니다"
	case "good":
		if wrParent != 0 && wrParent != wrID {
			action = "회원님의 댓글을 추천했습니다"
		} else {
			action = "회원님의 글을 추천했습니다"
		}
	case "nogood":
		action = "비추천했습니다"
	case "write":
		action = "새 글을 작성했습니다"
	case "memo":
		action = "쪽지를 보냈습니다"
	case "inquire":
		return "새 문의가 등록되었습니다"
	case "answer":
		return "문의에 답변이 등록되었습니다"
	case "digest":
		// 요약 구독(level=3) — 발신자 개념이 없어 문장형으로 (실측 604행이 default 로 뭉개졌었다)
		return "구독한 게시판의 새 글 요약이 도착했습니다"
	case "levelup":
		return "레벨이 올랐습니다! 🎉"
	case "promote":
		return "등급이 올랐습니다! 🎉"
	case "point_expiry":
		return "곧 소멸되는 포인트가 있습니다"
	default:
		return "새 알림이 있습니다"
	}

	if senderCount > 1 {
		return fmt.Sprintf("%s님 외 %d명이 %s", latestSender, senderCount-1, action)
	}
	return fmt.Sprintf("%s님이 %s", latestSender, action)
}

// generateMergedTitle — 대상 항목 단위 통합 묶음 한 줄 제목.
// "OO님 외 N명이 회원님의 글에 댓글 5 · 좋아요 3" 형태(free/6875994 Java 님 제안).
// isCommentTarget 은 묶음의 대상이 댓글인지 판정한다.
// tkey 접두 규약: cg:·r: = 댓글, s: = 비묶음, 그 외 = 글.
func isCommentTarget(targetKey string) bool {
	return strings.HasPrefix(targetKey, "cg:") || strings.HasPrefix(targetKey, "r:")
}

// mergedItemType 은 묶음 알림의 표시 종류를 정한다 (bug/13332).
//
// ⛔ 예전에는 묶음이면 무조건 "merged" 를 내려보냈고, 프론트 아이콘 매핑에 그 케이스가
// 없어 **default(Info) 회색 느낌표**로 떨어졌다. 「알림 묶어 보기」가 기본 켬이라
// 사실상 모든 알림이 같은 회색 아이콘이 됐다:
//
//	"보시는 것처럼 댓글과 답글 구분이 어려워서 확인하기 번거롭습니다" (bug/13332)
//
// 제목은 이미 "회원님의 글에 댓글 2 · 좋아요 3" 처럼 종류를 말하고 있었다. 아이콘만
// 죽어 있었던 것이라, 개수에서 종류를 되살린다.
//
// 한 종류뿐이면 그 종류로 정확히 표시하고, 섞였을 때만 묶음 표시를 쓴다. 이때도
// **대상(글/댓글)은 알려준다** — 섞였다고 회색으로 뭉개면 예전과 같아진다.
//
// ⛔ 섞였을 때 "가장 많은 종류" 로 대표시키지 않는다. 좋아요 5·댓글 1 을 좋아요로만
// 보여주면 댓글이 달린 사실이 화면에서 사라진다.
func mergedItemType(targetKey string, good, cmt, reply int) string {
	kinds := 0
	for _, n := range []int{good, cmt, reply} {
		if n > 0 {
			kinds++
		}
	}
	onComment := isCommentTarget(targetKey)

	if kinds == 1 {
		switch {
		case good > 0:
			if onComment {
				return "like_comment"
			}
			return "like"
		case reply > 0:
			return "reply"
		default:
			return "comment"
		}
	}
	// 0종(폴백) 또는 2종 이상 — 대상만이라도 구분해 준다.
	if onComment {
		return "merged_comment"
	}
	return "merged_post"
}

func generateMergedTitle(targetKey, latestSender string, senderCount, good, cmt, reply int) string {
	target := "글"
	if isCommentTarget(targetKey) {
		target = "댓글"
	}
	parts := make([]string, 0, 3)
	if cmt > 0 {
		parts = append(parts, fmt.Sprintf("댓글 %d", cmt))
	}
	if reply > 0 {
		parts = append(parts, fmt.Sprintf("답글 %d", reply))
	}
	if good > 0 {
		parts = append(parts, fmt.Sprintf("좋아요 %d", good))
	}
	if len(parts) == 0 {
		// fromCase 를 모르는 폴백 경로 — default 분기("새 알림이 있습니다")로 떨어진다.
		return generateGroupTitle("", "", latestSender, senderCount, 0, 0)
	}
	who := latestSender + "님"
	if senderCount > 1 {
		who = fmt.Sprintf("%s님 외 %d명", latestSender, senderCount-1)
	}
	return fmt.Sprintf("%s이 회원님의 %s에 %s", who, target, strings.Join(parts, " · "))
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

	// merge=target: 같은 글의 좋아요·댓글을 한 줄로 합친 대상 단위 묶음.
	// 파라미터 없으면 기존 유형별 묶음 그대로 — 구 클라이언트가 깨지지 않는다.
	if c.Query("merge") == "target" {
		h.getMergedNotifications(c, mbID, page, limit, filterType)
		return
	}

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
			Type:          mapNotificationType(g.PhFromCase, g.PhToCase, g.WrID, g.WrParent),
			BoTable:       g.BoTable,
			WrID:          g.WrID,
			Title:         generateGroupTitle(g.PhFromCase, g.PhToCase, g.LatestSender, g.SenderCount, g.WrID, g.WrParent),
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
// getMergedNotifications — merge=target 모드 응답 조립.
func (h *NotiHandler) getMergedNotifications(c *gin.Context, mbID string, page, limit int, filterType string) {
	groups, totalGroups, unreadCount, err := h.repo.GetMergedNotifications(mbID, page, limit, filterType)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "알림 목록 조회 실패", err)
		return
	}

	items := make([]groupedNotificationResponse, 0, len(groups))
	for _, g := range groups {
		senders := make([]string, 0, 5)
		// 구분자는 grouped 와 동일한 '||' — ',' 는 닉네임에 쉼표가 있으면 분해가 깨진다
		for _, nick := range strings.Split(g.Senders, "||") {
			if nick != "" {
				senders = append(senders, nick)
			}
		}
		// s:(비묶음 — 구독 새글·팔로우·쪽지·멘션)는 병합 대상이 아니다. 유형·제목을
		// 원래대로 유지해야 한다 — 'merged' 로 뭉개면 제목이 "새 알림이 있습니다"가
		// 되어 닉네임이 사라지고(bug/13199) 아이콘도 전부 Info 로 죽는다.
		itemType := mergedItemType(g.TargetKey, g.GoodCount, g.CommentCount, g.ReplyCount)
		fromCase := "merged"
		title := generateMergedTitle(g.TargetKey, g.LatestSender, g.SenderCount, g.GoodCount, g.CommentCount, g.ReplyCount)
		if strings.HasPrefix(g.TargetKey, "s:") {
			itemType = mapNotificationType(g.LatestFromCase, g.LatestToCase, g.WrID, g.LatestWrParent)
			fromCase = g.LatestFromCase
			title = generateGroupTitle(g.LatestFromCase, g.LatestToCase, g.LatestSender, g.SenderCount, g.WrID, g.LatestWrParent)
		}
		items = append(items, groupedNotificationResponse{
			Type:          itemType,
			BoTable:       g.BoTable,
			WrID:          g.WrID,
			Title:         title,
			URL:           convertLegacyURL(g.RelURL),
			ParentSubject: g.ParentSubject,
			Content:       g.RelMsg,
			LatestSender:  g.LatestSender,
			Senders:       senders,
			SenderCount:   g.SenderCount,
			UnreadCount:   g.UnreadCount,
			HasUnread:     g.UnreadCount > 0,
			LatestAt:      g.LatestAt.Format(time.RFC3339),
			FromCase:      fromCase,
			TargetKey:     g.TargetKey,
			GoodCount:     g.GoodCount,
			CommentCount:  g.CommentCount,
			ReplyCount:    g.ReplyCount,
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

func (h *NotiHandler) MarkGroupAsRead(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var req struct {
		BoTable   string `json:"bo_table"`
		WrID      int    `json:"wr_id"`
		FromCase  string `json:"from_case"`
		TargetKey string `json:"target_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 요청", err)
		return
	}

	// 병합 묶음은 목록과 같은 파생 키 식으로 읽음 처리한다 — 식이 갈라지면
	// 한 줄을 읽어도 일부 행이 안읽음으로 남는다.
	if req.TargetKey != "" {
		if err := h.repo.MarkMergedGroupAsRead(mbID, req.BoTable, req.TargetKey); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "그룹 읽음 처리 실패", err)
			return
		}
		common.V2Success(c, gin.H{"message": "그룹 읽음 처리 완료"})
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
	NotiComment        bool `json:"noti_comment"`
	NotiReply          bool `json:"noti_reply"`
	NotiMention        bool `json:"noti_mention"`
	NotiLike           bool `json:"noti_like"`
	NotiFollow         bool `json:"noti_follow"`
	NotiBoardSubscribe bool `json:"noti_board_subscribe"`
	LikeThreshold      int  `json:"like_threshold"`
}

func toNotiPreferenceResponse(pref *gnurepo.NotiPreference) notiPreferenceResponse {
	return notiPreferenceResponse{
		NotiComment:        pref.NotiComment,
		NotiReply:          pref.NotiReply,
		NotiMention:        pref.NotiMention,
		NotiLike:           pref.NotiLike,
		NotiFollow:         pref.NotiFollow,
		NotiBoardSubscribe: pref.NotiBoardSubscribe,
		LikeThreshold:      pref.LikeThreshold,
	}
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

	common.V2Success(c, toNotiPreferenceResponse(pref))
}

// UpdatePreferences handles PUT /api/v1/notifications/preferences
func (h *NotiHandler) UpdatePreferences(c *gin.Context) {
	mbID := middleware.GetUserID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var req struct {
		NotiComment        *bool `json:"noti_comment"`
		NotiReply          *bool `json:"noti_reply"`
		NotiMention        *bool `json:"noti_mention"`
		NotiLike           *bool `json:"noti_like"`
		NotiFollow         *bool `json:"noti_follow"`
		NotiBoardSubscribe *bool `json:"noti_board_subscribe"`
		LikeThreshold      *int  `json:"like_threshold"`
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
	if req.NotiBoardSubscribe != nil {
		pref.NotiBoardSubscribe = *req.NotiBoardSubscribe
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

	common.V2Success(c, toNotiPreferenceResponse(pref))
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

	// 병합 묶음 삭제 — 목록·읽음과 같은 파생 키 식. 없으면 기존 유형별 경로.
	if targetKey := c.Query("target_key"); targetKey != "" {
		if err := h.repo.DeleteMergedGroup(mbID, boTable, targetKey); err != nil {
			common.V2ErrorResponse(c, http.StatusInternalServerError, "그룹 삭제 실패", err)
			return
		}
		common.V2Success(c, gin.H{"message": "그룹 삭제 완료"})
		return
	}

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
