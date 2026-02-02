package handler

import (
	"net/http"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: 운영 환경에서는 origin 검증 필요
		return true
	},
}

// WSHandler handles WebSocket connections
type WSHandler struct {
	hub *ws.Hub
}

// NewWSHandler creates a new WSHandler
func NewWSHandler(hub *ws.Hub) *WSHandler {
	return &WSHandler{hub: hub}
}

// Connect handles GET /ws/notifications — WebSocket upgrade
// @Summary 실시간 알림 WebSocket
// @Tags notifications
// @Router /ws/notifications [get]
func (h *WSHandler) Connect(c *gin.Context) {
	userID := middleware.GetDamoangUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "로그인이 필요합니다"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := ws.NewClient(h.hub, conn, userID)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
