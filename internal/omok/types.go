// Package omok 은 실시간 오목 대전 서버다.
//
// 원본은 git 미관리였던 /home/damoang/go-services/omok (2025-11 이후 정지 상태)이며,
// 이번에 저장소로 편입하면서 다음을 새로 넣었다:
//   - JWT 인증 (익명 대국 금지 — 참가비를 걸려면 신원이 확정돼야 한다)
//   - 참가비 1,000P 차감 (나눔 응모와 같은 FOR UPDATE 트랜잭션 패턴)
//   - FIFO 매칭 큐 + 레이팅 밴드 (원본은 맵 순회라 대기 순서가 무시됐다)
//   - 대국·전적 영속화 (원본은 전부 메모리라 재시작 시 소실)
//   - 턴 타임아웃 = 패배 (이탈 악용 방지)
package omok

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// EntryFee 는 대국 참가비(포인트)다.
//
// ⛔ 참가비는 승자에게 가지 않고 소멸한다(운영 결정). 승패는 전적·레이팅에만
// 반영된다 — 포인트가 걸린 승부에서 상금까지 오가면 사행성 성격이 짙어지고,
// 실력 격차가 큰 상대를 노리는 유인도 생긴다. 참가비는 무분별한 매칭·잠수를
// 억제하는 비용일 뿐이다.
const EntryFee = 1000

// 매칭 모드. random·rating 은 유료, favorite(초대 대국)은 무료다 —
// 지인끼리의 연습 대국까지 유료화하면 아무도 쓰지 않는다.
const (
	ModeRandom   = "random"
	ModeRating   = "rating"
	ModeFavorite = "favorite"
)

// 턴 제한·재접속 유예
const (
	TurnTimeout      = 60 * time.Second
	ReconnectGrace   = 30 * time.Second
	DefaultRating    = 1500
	ratingBandInit   = 200 // 초기 허용 레이팅 차
	ratingBandWiden  = 30 * time.Second
	ratingBandSecond = 400
)

// Client 는 접속한 한 명이다. MbID 는 JWT 로 확정된 값이며 클라이언트가
// 보내는 값을 절대 신뢰하지 않는다.
type Client struct {
	conn         *websocket.Conn
	isAlive      bool
	matchingMode string
	MbID         string
	Nick         string
	sessionID    string
	roomID       string
	send         chan []byte
}

// queueEntry 는 대기열 한 칸이다. 슬라이스로 관리해 **선착순(FIFO)** 을 보장한다.
type queueEntry struct {
	client   *Client
	rating   int
	joinedAt time.Time
}

type Session struct {
	ID               string
	MbID             string
	Client           *Client
	GameID           string
	DisconnectedAt   *time.Time
	ReconnectTimeout *time.Timer
	GameState        *GameState
}

type Room struct {
	ID           string
	Players      []string
	GameState    *GameState
	PlayerColors map[string]int
	Status       string
	Created      time.Time
	// DBGameID 는 angple_omok_games 의 행 id — 참가비·결과 기록의 키다.
	DBGameID int64
	// Paid 는 참가비가 실제로 차감된 대국인지(초대 대국은 false).
	Paid bool
}

type GameState struct {
	Board         [][]int
	CurrentPlayer int
	BlackMbID     string
	WhiteMbID     string
	MoveHistory   []Move
	StartTime     time.Time
	LastMoveTime  time.Time
	// turnTimer 는 현재 턴의 제한시간 타이머다. 착수·종료 때마다 교체된다.
	turnTimer *time.Timer
	finished  bool
}

type Move struct {
	MbID string    `json:"mb_id"`
	X    int       `json:"x"`
	Y    int       `json:"y"`
	Time time.Time `json:"t"`
}

type Message struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data,omitempty"`
}

// Server 는 접속·방·큐를 들고 있는 허브다. 상태는 여전히 메모리지만,
// 대국 결과와 참가비는 DB(store)에 남는다.
type Server struct {
	clients              map[*Client]bool
	sessions             map[string]*Session
	disconnectedSessions map[string]*Session
	rooms                map[string]*Room
	// matchingQueue 는 모드별 **순서 있는** 대기열이다(원본은 map 이라 순서가 없었다).
	matchingQueue map[string][]*queueEntry
	mu            sync.RWMutex
	upgrader      websocket.Upgrader
	store         *Store
	verifyToken   func(string) (mbID string, nick string, err error)
}
