package handler

import (
	"net/http"
	"strconv"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// MessageHandler handles private message HTTP requests
type MessageHandler struct {
	service service.MessageService
}

// NewMessageHandler creates a new MessageHandler
func NewMessageHandler(service service.MessageService) *MessageHandler {
	return &MessageHandler{service: service}
}

// SendMessage handles POST /messages
// @Summary 쪽지 보내기
// @Tags messages
// @Accept json
// @Produce json
// @Param request body domain.SendMessageRequest true "쪽지 내용"
// @Success 200 {object} common.APIResponse{data=domain.MessageResponse}
// @Router /messages [post]
func (h *MessageHandler) SendMessage(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	var req domain.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	result, err := h.service.SendMessage(userID, &req, c.ClientIP())
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: result})
}

// GetInbox handles GET /messages/inbox
// @Summary 받은 쪽지함
// @Tags messages
// @Produce json
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.MessageResponse}
// @Router /messages/inbox [get]
func (h *MessageHandler) GetInbox(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	messages, meta, err := h.service.GetInbox(userID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "받은 쪽지함 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: messages, Meta: meta})
}

// GetSent handles GET /messages/sent
// @Summary 보낸 쪽지함
// @Tags messages
// @Produce json
// @Param page query int false "페이지"
// @Param limit query int false "페이지 크기"
// @Success 200 {object} common.APIResponse{data=[]domain.MessageResponse}
// @Router /messages/sent [get]
func (h *MessageHandler) GetSent(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	page := 1
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	messages, meta, err := h.service.GetSent(userID, page, limit)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "보낸 쪽지함 조회 실패", err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: messages, Meta: meta})
}

// GetMessage handles GET /messages/:id
// @Summary 쪽지 상세
// @Tags messages
// @Produce json
// @Param id path int true "쪽지 ID"
// @Success 200 {object} common.APIResponse{data=domain.MessageResponse}
// @Router /messages/{id} [get]
func (h *MessageHandler) GetMessage(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID입니다", err)
		return
	}

	msg, err := h.service.GetMessage(id, userID)
	if err != nil {
		common.ErrorResponse(c, http.StatusNotFound, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, common.APIResponse{Data: msg})
}

// DeleteMessage handles DELETE /messages/:id
// @Summary 쪽지 삭제
// @Tags messages
// @Produce json
// @Param id path int true "쪽지 ID"
// @Success 204
// @Router /messages/{id} [delete]
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		common.ErrorResponse(c, http.StatusUnauthorized, "로그인이 필요합니다", nil)
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "잘못된 쪽지 ID입니다", err)
		return
	}

	if err := h.service.DeleteMessage(id, userID); err != nil {
		common.ErrorResponse(c, http.StatusNotFound, "쪽지를 찾을 수 없습니다", err)
		return
	}

	c.Status(http.StatusNoContent)
}
