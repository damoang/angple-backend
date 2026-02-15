package handler

import (
	"net/http"
	"strings"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WSHandler handles WebSocket connections
type WSHandler struct {
	hub            *ws.Hub
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

// NewWSHandler creates a new WSHandler
func NewWSHandler(hub *ws.Hub, allowedOrigins string) *WSHandler {
	h := &WSHandler{
		hub:            hub,
		allowedOrigins: parseOrigins(allowedOrigins),
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     h.checkOrigin,
	}
	return h
}

// parseOrigins parses comma-separated origins string
func parseOrigins(origins string) []string {
	if origins == "" {
		return nil
	}
	parts := strings.Split(origins, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// checkOrigin validates the request origin against allowed origins
func (h *WSHandler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Same-origin requests don't have Origin header
	}

	// If no allowed origins configured, allow all (development mode)
	if len(h.allowedOrigins) == 0 {
		return true
	}

	// Check against allowed origins
	for _, allowed := range h.allowedOrigins {
		if origin == allowed {
			return true
		}
	}

	return false
}

// Connect handles GET /ws/notifications — WebSocket upgrade
// @Summary 실시간 알림 WebSocket
// @Tags notifications
// @Router /ws/notifications [get]
func (h *WSHandler) Connect(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "로그인이 필요합니다"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := ws.NewClient(h.hub, conn, userID)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
