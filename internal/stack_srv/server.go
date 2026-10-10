package stacksrv

import (
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NewServer 는 허브를 만든다. verifyToken 은 JWT → (회원 아이디, 닉네임) 검증기다.
func NewServer(store GameStore, verifyToken func(string) (string, string, error), timing Timing) *Server {
	return &Server{
		clients:     make(map[string]*Client),
		sessions:    make(map[string]string),
		rooms:       make(map[string]*Room),
		playerRoom:  make(map[string]string),
		queues:      make(map[string][]*queueEntry),
		starting:    make(map[string]bool),
		store:       store,
		verifyToken: verifyToken,
		timing:      timing,
		now:         time.Now,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

// HandleWebSocket 은 업그레이드 전에 인증을 끝낸다. 참가비가 걸리므로 익명 접속은 받지 않는다.
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	mbID, nick, err := s.authenticate(r)
	if err != nil {
		http.Error(w, codeUnauthorized, http.StatusUnauthorized)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[stack] upgrade error: %v", err)
		return
	}
	client := &Client{conn: conn, send: make(chan []byte, 256), done: make(chan struct{}), MbID: mbID, Nick: nick}
	client.alive.Store(true)
	client.limiter = newTokenBucket(20, 10)
	conn.SetPongHandler(func(string) error { client.alive.Store(true); return nil })

	if old := s.attach(client); old != nil && old.conn != nil {
		// 회원당 연결 1 — 새 연결이 이긴다(새로고침·다른 탭). 옛 연결은 닫는다.
		closeConn(old)
	}
	s.emit(client, "connected", map[string]interface{}{"mbId": mbID, "nickname": nick, "sessionId": client.sessionID, "entryFee": EntryFee, "stats": s.statsByRule(mbID)})

	go s.writePump(client)
	s.readPump(client)
}

// authenticate 는 요청의 JWT 를 검증한다.
func (s *Server) authenticate(r *http.Request) (string, string, error) {
	token := bearerToken(r)
	if token == "" {
		return "", "", errUnauthorized
	}
	mbID, nick, err := s.verifyToken(token)
	if err != nil {
		return "", "", err
	}
	if mbID == "" {
		return "", "", errUnauthorized
	}
	return mbID, nick, nil
}

// attach 는 접속을 등록하고 세션을 만든다. 같은 회원의 옛 연결이 있으면 돌려준다.
func (s *Server) attach(c *Client) *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.clients[c.MbID]
	s.clients[c.MbID] = c
	c.sessionID = "s_" + generateID(16)
	s.sessions[c.sessionID] = c.MbID
	if old != nil {
		s.removeFromQueuesLocked(old)
	}
	return old
}

// statsByRule 은 회원이 전적을 가진 규칙만 담은 전적 맵이다.
func (s *Server) statsByRule(mbID string) map[string]PlayerStats {
	out := map[string]PlayerStats{}
	if s.store == nil {
		return out
	}
	for _, rule := range OpenRules() {
		if st, ok := s.store.Stats(mbID, rule); ok {
			out[rule] = st
		}
	}
	return out
}

// bearerToken 은 Authorization 헤더 또는 ?token= 쿼리에서 토큰을 꺼낸다.
// 브라우저 WebSocket API 는 커스텀 헤더를 못 붙여서 쿼리 경로가 필요하다.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return r.URL.Query().Get("token")
}

// bearerHeader 는 Authorization 헤더의 토큰만 꺼낸다. REST(혼자하기 기록)는 쿼리 토큰을 받지 않는다 —
// 쿼리 문자열은 프록시 접근 로그에 남는다.
func bearerHeader(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// readPump 는 메시지를 읽어 처리한다. 2KB 를 넘는 메시지는 연결을 끊는다.
func (s *Server) readPump(c *Client) {
	defer func() {
		close(c.done)
		s.handleDisconnect(c)
		closeConn(c)
	}()
	c.conn.SetReadLimit(MaxMessageBytes)
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if !c.limiter.allow(s.now()) {
			continue
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		s.handleMessage(c, msg)
	}
}

// writePump 는 send 버퍼를 연결로 흘려보낸다.
// send 는 닫지 않는다(emit 이 닫힌 채널에 보내면 panic). 대신 readPump 가 끝나며 done 을 닫으면 빠져나온다.
func (s *Server) writePump(c *Client) {
	for {
		select {
		case msg := <-c.send:
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// closeConn 은 연결을 닫고 에러는 로그만 남긴다.
func closeConn(c *Client) {
	if c == nil || c.conn == nil {
		return
	}
	if err := c.conn.Close(); err != nil {
		log.Printf("[stack] close error: %v", err)
	}
}

// emit 은 {type, data} 봉투로 보낸다. 락을 잡지 않으므로 호출자가 잡고 있어도 된다.
// 버퍼가 가득 차면 버린다(느린 클라이언트가 허브를 막지 못하게).
func (s *Server) emit(c *Client, typ string, data interface{}) {
	if c == nil {
		return
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	msg, err := json.Marshal(map[string]interface{}{"type": typ, "data": data})
	if err != nil {
		return
	}
	select {
	case c.send <- msg:
	default:
		log.Printf("[stack] send buffer full, dropping %s", typ)
	}
}

// emitToLocked 는 회원에게 보낸다(접속 중일 때만). 호출자가 s.mu 를 잡고 있어야 한다.
func (s *Server) emitToLocked(mbID, typ string, data interface{}) {
	s.emit(s.clients[mbID], typ, data)
}

// sendError 는 error {code, message} 를 보낸다.
func (s *Server) sendError(c *Client, code, message string) {
	s.emit(c, statusError, map[string]interface{}{"code": code, "message": message})
}

// StartHeartbeat 은 30초마다 ping 하고 응답 없는 연결을 끊는다.
func (s *Server) StartHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		conns := make([]*Client, 0, len(s.clients))
		for _, c := range s.clients {
			conns = append(conns, c)
		}
		s.mu.Unlock()
		for _, c := range conns {
			if c.conn == nil {
				continue
			}
			if !c.alive.Swap(false) {
				closeConn(c)
				continue
			}
			if err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				closeConn(c)
			}
		}
	}
}

// StartStatusMonitor 는 운영 판단용 지표를 1분마다 로그로 남긴다.
func (s *Server) StartStatusMonitor() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		waiting := 0
		for _, q := range s.queues {
			waiting += len(q)
		}
		log.Printf("[stack] status connections=%d rooms=%d queued=%d", len(s.clients), len(s.rooms), waiting)
		s.mu.Unlock()
	}
}

// Snapshot 은 헬스체크용 요약이다(숫자만).
func (s *Server) Snapshot() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	waiting := 0
	for _, q := range s.queues {
		waiting += len(q)
	}
	return map[string]int{"connections": len(s.clients), "rooms": len(s.rooms), "queued": waiting}
}

// lobbyCounts 는 규칙별 공개 대기자 수다. 초대 대전은 지정 상대만 기다리므로 뺀다.
func (s *Server) lobbyCounts() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	waiting := map[string]int{}
	for _, rule := range OpenRules() {
		waiting[rule] = len(s.queues[queueKey(ModeRandom, rule)])
	}
	return map[string]interface{}{"waiting": waiting}
}

// HandleLobby 는 GET /stack-ws/lobby 다. 공개·인증 불요, 숫자만 내보낸다(대기자 닉네임 노출 금지).
// 서버 메모리 60초 캐시 + public, max-age=60.
func (s *Server) HandleLobby(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body := s.lobby.get(s.now(), "lobby", func() ([]byte, error) {
		return json.Marshal(s.lobbyCounts())
	})
	writePublicJSON(w, body)
}

// writePublicJSON 은 공개 캐시 가능한 JSON 응답을 쓴다.
func writePublicJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if _, err := w.Write(body); err != nil {
		log.Printf("[stack] write response failed: %v", err)
	}
}

// PublicCacheTTL 은 공개 GET 의 서버 메모리 캐시 수명이다.
const PublicCacheTTL = 60 * time.Second

// jsonCache 는 키별로 직렬화된 응답을 60초 들고 있는 캐시다.
type jsonCache struct {
	// mu 는 entries 를 지킨다.
	mu sync.Mutex
	// entries 는 키 → 캐시 항목이다.
	entries map[string]cacheEntry
}

// cacheEntry 는 캐시 항목 하나다.
type cacheEntry struct {
	// body 는 응답 본문이다.
	body []byte
	// at 은 만든 시각이다.
	at time.Time
}

// get 은 키의 캐시를 돌려주고, 없거나 60초가 지났으면 build 로 새로 만든다.
// build 가 실패하면 캐시하지 않고 nil 을 돌려준다.
func (c *jsonCache) get(now time.Time, key string, build func() ([]byte, error)) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok && now.Sub(e.at) < PublicCacheTTL {
		return e.body
	}
	body, err := build()
	if err != nil {
		return nil
	}
	if c.entries == nil {
		c.entries = make(map[string]cacheEntry)
	}
	c.entries[key] = cacheEntry{body: body, at: now}
	return body
}

// tokenBucket 은 메시지 빈도 제한이다(초당 rate, 최대 burst).
type tokenBucket struct {
	// tokens 는 남은 허용량이다.
	tokens float64
	// burst 는 최대 허용량이다.
	burst float64
	// rate 는 초당 충전량이다.
	rate float64
	// last 는 마지막 충전 시각이다.
	last time.Time
}

// newTokenBucket 은 가득 찬 버킷을 만든다.
func newTokenBucket(burst, rate float64) tokenBucket {
	return tokenBucket{tokens: burst, burst: burst, rate: rate}
}

// allow 는 한 건을 허용할지다.
func (b *tokenBucket) allow(now time.Time) bool {
	if b.burst <= 0 {
		return true
	}
	if !b.last.IsZero() {
		b.tokens += now.Sub(b.last).Seconds() * b.rate
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// generateID 는 예측 불가 식별자를 만든다. 방·세션 id 가 남의 대전에 끼어드는 데 쓰이지 않게
// crypto/rand 를 쓴다.
func generateID(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		return "t" + time.Now().Format("150405.000000000")
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// randomUint32 는 crypto/rand 로 만든 32비트 값이다(엔진 시드).
// Go 1.24 부터 crypto/rand.Read 는 실패하지 않는다(실패 시 프로세스가 멈춘다). 그래도 에러는 남긴다.
func randomUint32() uint32 {
	var b [4]byte
	if _, err := crand.Read(b[:]); err != nil {
		log.Printf("[stack] crypto/rand failed: %v", err)
	}
	return binary.LittleEndian.Uint32(b[:])
}

// randomHole 은 방해 줄 구멍 열(0~8)이다. 바이트 하나를 9의 배수 구간(0~251)에서만 받아 치우침을 없앤다.
func randomHole() int {
	var b [1]byte
	for i := 0; i < 16; i++ {
		if _, err := crand.Read(b[:]); err != nil {
			log.Printf("[stack] crypto/rand failed: %v", err)
			return 0
		}
		if b[0] < 252 {
			return int(b[0]) % Cols
		}
	}
	return int(b[0]) % Cols
}
