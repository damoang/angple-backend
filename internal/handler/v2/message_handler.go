package v2

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"github.com/damoang/angple-backend/internal/middleware"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
)

// MessageHandler handles v2 message API endpoints
type MessageHandler struct {
	messageRepo v2repo.MessageRepository
}

// NewMessageHandler creates a new MessageHandler
func NewMessageHandler(messageRepo v2repo.MessageRepository) *MessageHandler {
	return &MessageHandler{messageRepo: messageRepo}
}

func (h *MessageHandler) getUserID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(middleware.GetUserID(c), 10, 64)
}

// SendMessage handles POST /api/v2/messages
func (h *MessageHandler) SendMessage(c *gin.Context) {
	senderID, err := h.getUserID(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	var req struct {
		ReceiverID uint64 `json:"receiver_id" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	msg := &v2domain.V2Message{
		SenderID:   senderID,
		ReceiverID: req.ReceiverID,
		Content:    req.Content,
	}
	if err := h.messageRepo.Create(msg); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "쪽지 보내기 실패", err)
		return
	}
	common.V2Created(c, msg)
}

// GetInbox handles GET /api/v2/messages/inbox
func (h *MessageHandler) GetInbox(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	page, perPage := parsePagination(c)
	messages, total, err := h.messageRepo.FindInbox(userID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "받은 쪽지 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, messages, common.NewV2Meta(page, perPage, total))
}

// GetSent handles GET /api/v2/messages/sent
func (h *MessageHandler) GetSent(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	page, perPage := parsePagination(c)
	messages, total, err := h.messageRepo.FindSent(userID, page, perPage)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "보낸 쪽지 조회 실패", err)
		return
	}
	common.V2SuccessWithMeta(c, messages, common.NewV2Meta(page, perPage, total))
}

// GetMessage handles GET /api/v2/messages/:id
func (h *MessageHandler) GetMessage(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID", err)
		return
	}

	msg, err := h.messageRepo.FindByID(id)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "쪽지를 찾을 수 없습니다", err)
		return
	}

	// Only sender or receiver can view
	if msg.SenderID != userID && msg.ReceiverID != userID {
		common.V2ErrorResponse(c, http.StatusForbidden, "권한이 없습니다", nil)
		return
	}

	// Mark as read if receiver views
	if msg.ReceiverID == userID && !msg.IsRead {
		_ = h.messageRepo.MarkAsRead(id) //nolint:errcheck
	}

	common.V2Success(c, msg)
}

// DeleteMessage handles DELETE /api/v2/messages/:id
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	userID, err := h.getUserID(c)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusUnauthorized, "잘못된 사용자 인증 정보", err)
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID", err)
		return
	}

	if err := h.messageRepo.DeleteForUser(id, userID); err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "쪽지 삭제 실패", err)
		return
	}
	common.V2Success(c, gin.H{"message": "삭제 완료"})
}
