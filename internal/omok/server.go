package omok

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// NewServer 는 허브를 만든다. verifyToken 은 JWT → (mb_id, 닉네임) 검증기다.
func NewServer(store *Store, verifyToken func(string) (string, string, error)) *Server {
	return &Server{
		clients:              make(map[*Client]bool),
		sessions:             make(map[string]*Session),
		disconnectedSessions: make(map[string]*Session),
		rooms:                make(map[string]*Room),
		matchingQueue: map[string][]*queueEntry{
			ModeRandom:   {},
			ModeRating:   {},
			ModeFavorite: {},
		},
		store:       store,
		verifyToken: verifyToken,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// HandleWebSocket 은 업그레이드 전에 **인증을 끝낸다**.
//
// ⛔ 원본은 클라이언트가 보내온 playerId 를 그대로 신뢰했다. 참가비가 걸리는
// 이상 남의 이름으로 대국하거나 남의 포인트를 태우는 일이 없어야 한다.
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mbID, nick, err := s.verifyToken(token)
	if err != nil || mbID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[omok] upgrade error:", err)
		return
	}

	client := &Client{
		conn: conn, isAlive: true, send: make(chan []byte, 256),
		MbID: mbID, Nick: nick,
	}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	sess := s.createSessionForPlayer(client)
	log.Printf("[omok] connected mb_id=%s", mbID)

	conn.SetPongHandler(func(string) error { client.isAlive = true; return nil })

	wins, losses, draws, rating := s.store.Stats(mbID)
	s.sendToClient(client, map[string]interface{}{
		"type": "connected", "mbId": mbID, "nickname": nick,
		"sessionId": sess.ID, "entryFee": EntryFee,
		"stats": map[string]int{"wins": wins, "losses": losses, "draws": draws, "rating": rating},
	})

	go s.writePump(client)
	s.readPump(client)
}

// bearerToken 은 Authorization 헤더 또는 ?token= 쿼리에서 토큰을 꺼낸다.
// 브라우저 WebSocket API 는 커스텀 헤더를 못 붙여서 쿼리 경로가 필요하다.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if p := r.Header.Get("Sec-WebSocket-Protocol"); strings.HasPrefix(p, "bearer,") {
		return strings.TrimSpace(p[7:])
	}
	return r.URL.Query().Get("token")
}

func (s *Server) readPump(client *Client) {
	defer func() {
		s.handleDisconnect(client)
		client.conn.Close()
	}()
	client.conn.SetReadLimit(16 << 10)
	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		s.handleMessage(client, msg)
	}
}

func (s *Server) writePump(client *Client) {
	for message := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// sendToClient 는 락을 잡지 않는다 — 호출자가 이미 잡고 있을 수 있다.
// ⛔ 원본은 채널이 가득 차면 여기서 close(send) + delete(clients) 를 했는데,
//
//	락 상태에서 맵을 건드려 데드락·이중 close 위험이 있었다. 지금은 버린다.
func (s *Server) sendToClient(client *Client, data interface{}) {
	if client == nil {
		return
	}
	msg, err := json.Marshal(data)
	if err != nil {
		return
	}
	select {
	case client.send <- msg:
	default:
		log.Printf("[omok] send buffer full, dropping message for %s", client.MbID)
	}
}

func generateID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 9)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

func generateSessionID() string {
	return "s_" + time.Now().Format("20060102150405") + "_" + generateID()
}

// StartHeartbeat 은 30초마다 ping 하고 죽은 연결을 끊는다.
func (s *Server) StartHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for client := range s.clients {
			if !client.isAlive {
				client.conn.Close()
				continue
			}
			client.isAlive = false
			_ = client.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second))
		}
		s.mu.Unlock()
	}
}

// StartStatusMonitor 는 운영 판단용 지표를 로그로 남긴다(접속·대기열·대국 수).
func (s *Server) StartStatusMonitor() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.RLock()
		log.Printf("[omok] status connections=%d rooms=%d queue(random=%d rating=%d favorite=%d)",
			len(s.clients), len(s.rooms),
			len(s.matchingQueue[ModeRandom]), len(s.matchingQueue[ModeRating]), len(s.matchingQueue[ModeFavorite]))
		s.mu.RUnlock()
	}
}

// Snapshot 은 헬스체크용 요약이다.
func (s *Server) Snapshot() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]int{
		"connections": len(s.clients),
		"rooms":       len(s.rooms),
		"queue":       len(s.matchingQueue[ModeRandom]) + len(s.matchingQueue[ModeRating]) + len(s.matchingQueue[ModeFavorite]),
	}
}
