package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// V1MessageHandler wraps g5_memo repo for /api/v1/messages endpoints
type V1MessageHandler struct {
	memoRepo   gnurepo.MemoRepository
	memberRepo gnurepo.MemberRepository
	notiRepo   gnurepo.NotiRepository
}

// NewV1MessageHandler creates a new V1MessageHandler using g5_memo
func NewV1MessageHandler(memoRepo gnurepo.MemoRepository, memberRepo gnurepo.MemberRepository, notiRepo gnurepo.NotiRepository) *V1MessageHandler {
	return &V1MessageHandler{memoRepo: memoRepo, memberRepo: memberRepo, notiRepo: notiRepo}
}

// v1MessageResponse matches frontend Message type
type v1MessageResponse struct {
	ID           int     `json:"id"`
	SenderID     string  `json:"sender_id"`
	SenderName   string  `json:"sender_name"`
	ReceiverID   string  `json:"receiver_id"`
	ReceiverName string  `json:"receiver_name"`
	Content      string  `json:"content"`
	IsRead       bool    `json:"is_read"`
	ReadDatetime *string `json:"read_datetime,omitempty"`
	SendDatetime string  `json:"send_datetime"`
}

// v1MessageListResponse matches frontend MessageListResponse type
type v1MessageListResponse struct {
	Items       []v1MessageResponse `json:"items"`
	Total       int64               `json:"total"`
	UnreadCount int64               `json:"unread_count"`
	Page        int                 `json:"page"`
	Limit       int                 `json:"limit"`
	TotalPages  int64               `json:"total_pages"`
}

func (h *V1MessageHandler) getMbID(c *gin.Context) string {
	return middleware.GetUserID(c)
}

func (h *V1MessageHandler) toV1Message(memo *gnuboard.G5Memo, nickMap map[string]string) v1MessageResponse {
	resp := v1MessageResponse{
		ID:           memo.MeID,
		SenderID:     memo.MeSendMbID,
		SenderName:   nickMap[memo.MeSendMbID],
		ReceiverID:   memo.MeRecvMbID,
		ReceiverName: nickMap[memo.MeRecvMbID],
		Content:      memo.MeMemo,
		IsRead:       memo.IsRead(),
		SendDatetime: memo.MeSendDatetime.Format("2006-01-02 15:04:05"),
	}
	if memo.IsRead() {
		resp.ReadDatetime = &memo.MeReadDatetime
	}
	// Fall back to mb_id if nick not found
	if resp.SenderName == "" {
		resp.SenderName = memo.MeSendMbID
	}
	if resp.ReceiverName == "" {
		resp.ReceiverName = memo.MeRecvMbID
	}
	return resp
}

// collectMbIDs extracts unique mb_ids from memos for batch nickname lookup
func collectMbIDs(memos []*gnuboard.G5Memo) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, m := range memos {
		if !seen[m.MeSendMbID] {
			seen[m.MeSendMbID] = true
			ids = append(ids, m.MeSendMbID)
		}
		if !seen[m.MeRecvMbID] {
			seen[m.MeRecvMbID] = true
			ids = append(ids, m.MeRecvMbID)
		}
	}
	return ids
}

// GetMessages handles GET /api/v1/messages?kind=recv|send
func (h *V1MessageHandler) GetMessages(c *gin.Context) {
	mbID := h.getMbID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	kind := c.DefaultQuery("kind", "recv")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var memos []*gnuboard.G5Memo
	var total int64
	var err error

	if kind == "send" {
		memos, total, err = h.memoRepo.FindSent(mbID, page, limit)
	} else {
		memos, total, err = h.memoRepo.FindInbox(mbID, page, limit)
	}
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "쪽지 목록 조회 실패", err)
		return
	}

	unreadCount, _ := h.memoRepo.CountUnread(mbID)

	// Batch resolve nicknames
	nickMap, _ := h.memberRepo.FindNicksByIDs(collectMbIDs(memos))
	if nickMap == nil {
		nickMap = make(map[string]string)
	}

	items := make([]v1MessageResponse, 0, len(memos))
	for _, memo := range memos {
		items = append(items, h.toV1Message(memo, nickMap))
	}

	totalPages := int64(math.Ceil(float64(total) / float64(limit)))

	common.V2Success(c, v1MessageListResponse{
		Items:       items,
		Total:       total,
		UnreadCount: unreadCount,
		Page:        page,
		Limit:       limit,
		TotalPages:  totalPages,
	})
}

// GetMessage handles GET /api/v1/messages/:id
func (h *V1MessageHandler) GetMessage(c *gin.Context) {
	mbID := h.getMbID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID", err)
		return
	}

	memo, err := h.memoRepo.FindByID(id, mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "쪽지를 찾을 수 없습니다", err)
		return
	}

	// Mark as read if it's a received memo and not yet read
	if memo.MeRecvMbID == mbID && memo.MeType == "recv" && !memo.IsRead() {
		_ = h.memoRepo.MarkAsRead(memo.MeID)
	}

	nickMap, _ := h.memberRepo.FindNicksByIDs([]string{memo.MeSendMbID, memo.MeRecvMbID})
	if nickMap == nil {
		nickMap = make(map[string]string)
	}

	common.V2Success(c, h.toV1Message(memo, nickMap))
}

// SendMessage handles POST /api/v1/messages
func (h *V1MessageHandler) SendMessage(c *gin.Context) {
	mbID := h.getMbID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	var req struct {
		ReceiverID string `json:"receiver_id" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// Block messages to admin account
	if req.ReceiverID == "admin" {
		common.V2ErrorResponse(c, http.StatusForbidden, "관리자에게는 쪽지를 보낼 수 없습니다", nil)
		return
	}

	// Verify receiver exists
	_, err := h.memberRepo.FindByID(req.ReceiverID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "받는 사람을 찾을 수 없습니다", err)
		return
	}

	sender, err := h.memberRepo.FindByID(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "보내는 회원 정보를 찾을 수 없습니다", err)
		return
	}
	if sender.MbCertify == "" {
		common.V2ErrorResponse(c, http.StatusForbidden, "실명인증이 필요합니다", nil)
		return
	}

	memo, err := h.memoRepo.Send(mbID, req.ReceiverID, req.Content)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "쪽지 보내기 실패", err)
		return
	}

	nickMap, _ := h.memberRepo.FindNicksByIDs([]string{mbID, req.ReceiverID})
	if nickMap == nil {
		nickMap = make(map[string]string)
	}

	// 쪽지 알림 (비동기)
	go func() {
		senderNick := nickMap[mbID]
		if senderNick == "" {
			senderNick = mbID
		}
		_ = h.notiRepo.Create(&gnurepo.Notification{
			PhToCase:   "memo",
			PhFromCase: "memo",
			MbID:       req.ReceiverID,
			RelMbID:    mbID,
			RelMbNick:  senderNick,
			RelMsg:     fmt.Sprintf("%s님이 쪽지를 보냈습니다.", senderNick),
			RelURL:     fmt.Sprintf("/member/messages/%d", memo.MeID),
			PhReaded:   "N",
			PhDatetime: time.Now(),
		})
	}()

	common.V2Created(c, h.toV1Message(memo, nickMap))
}

// DeleteMessage handles DELETE /api/v1/messages/:id
func (h *V1MessageHandler) DeleteMessage(c *gin.Context) {
	mbID := h.getMbID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID", err)
		return
	}

	if err := h.memoRepo.Delete(id, mbID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "쪽지 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}

// GetUnreadCount handles GET /api/v1/messages/unread-count
func (h *V1MessageHandler) GetUnreadCount(c *gin.Context) {
	mbID := h.getMbID(c)
	if mbID == "" {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "인증이 필요합니다", nil)
		return
	}

	count, err := h.memoRepo.CountUnread(mbID)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "미읽은 쪽지 수 조회 실패", err)
		return
	}
	common.V2Success(c, gin.H{"count": count})
}
