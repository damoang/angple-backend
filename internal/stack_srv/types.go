// Package stacksrv 는 앙쌓기(낙하 블록 퍼즐)의 온라인 대전·점수판 서버다.
//
// 각 클라이언트는 자기 엔진을 직접 돌린다. 서버는 시드 발급(crypto/rand)·매칭·방해 줄(양·구멍 열)·
// 판정·타이머·타당성 검사·기록만 맡는다. 골격은 장기 서버(janggi_srv)를 그대로 따른다:
// JWT 인증, random 매칭 참가비 1,000P(초대 대전 무료), FOR UPDATE 트랜잭션 차감, 기동 시 중단 대전 환불.
//
// 판 직렬화는 162자 문자열이다. 9열 × 18행, 맨 위 행부터 아래로, 각 행은 왼쪽에서 오른쪽(행 우선).
// 칸 값은 '0'(빈칸), '1'~'7'(굳은 조각), '8'(방해 줄)이다.
package stacksrv

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// EntryFee 는 random 대전 한 판의 참가비(포인트)다. 재대결도 판마다 다시 걷는다.
// 참가비는 승자에게 가지 않고 소멸한다(장기·오목과 같은 운영 결정).
const EntryFee = 1000

// ModeRandom 은 아무나와 붙는 유료 매칭이다.
const ModeRandom = "random"

// ModeFavorite 은 초대 코드로 지인과 붙는 무료 매칭이다.
const ModeFavorite = "favorite"

// DefaultRating 은 규칙별 ELO 의 시작값이다.
const DefaultRating = 1500

// MaxRooms 는 동시에 열 수 있는 방의 상한이다.
const MaxRooms = 200

// MaxMessageBytes 는 클라이언트 메시지 한 건의 상한이다. 넘으면 연결이 끊긴다.
const MaxMessageBytes = 2048

// MaxDrops 는 한 판에서 허용하는 연결 끊김 횟수다. 이를 넘기면 패배다.
const MaxDrops = 3

// MaxViolations 는 타당성 검사 위반 허용 횟수다. 이 횟수에 이르면 cheat 패배다.
const MaxViolations = 3

// TickSlack 은 클라이언트 틱이 서버가 잰 실경과보다 앞설 수 있는 여유(틱)다.
const TickSlack = 180

// 대전 방 단계
const (
	phaseReady     = "ready"
	phaseCountdown = "countdown"
	phasePlaying   = "playing"
	phaseOver      = "over"
)

// Timing 은 대전 타이머 묶음이다. 테스트에서는 짧게 바꿔 쓴다.
type Timing struct {
	// Ready 는 매칭 후 양쪽이 ready 를 보내야 하는 시간이다. 넘기면 취소·환불.
	Ready time.Duration
	// Countdown 은 양쪽 ready 후 go 까지의 카운트다운이다.
	Countdown time.Duration
	// Inactivity 동안 굳힌 조각이 없으면 패배다.
	Inactivity time.Duration
	// ReconnectGrace 는 끊김 한 번당 재접속 유예다.
	ReconnectGrace time.Duration
	// ReconnectTotal 은 한 판의 끊김 누적 상한이다.
	ReconnectTotal time.Duration
	// Rematch 는 판이 끝난 뒤 재대결을 받는 시간이다.
	Rematch time.Duration
	// OpponentStateEvery 는 opponent_state 최소 간격이다(초당 4회 상한).
	OpponentStateEvery time.Duration
	// LocksPerSecond 는 1초 창 안에서 받는 lock 상한이다.
	LocksPerSecond int
	// TimeLimitScale 은 규칙 제한시간에 곱하는 배율이다(운영 1, 테스트에서만 줄인다).
	TimeLimitScale float64
}

// DefaultTiming 은 운영 값이다.
func DefaultTiming() Timing {
	return Timing{
		Ready:              10 * time.Second,
		Countdown:          3 * time.Second,
		Inactivity:         20 * time.Second,
		ReconnectGrace:     15 * time.Second,
		ReconnectTotal:     30 * time.Second,
		Rematch:            30 * time.Second,
		OpponentStateEvery: 250 * time.Millisecond,
		LocksPerSecond:     10,
		TimeLimitScale:     1,
	}
}

// Message 는 클라이언트가 보내는 봉투 {type, data} 다.
type Message struct {
	// Type 은 메시지 이름이다.
	Type string `json:"type"`
	// Data 는 메시지별 내용이다. 종류에 따라 따로 해석한다.
	Data json.RawMessage `json:"data,omitempty"`
}

// Client 는 접속 하나다. MbID 는 JWT 로 확정된 값만 쓰고 클라이언트가 보낸 값은 믿지 않는다.
type Client struct {
	// conn 은 웹소켓 연결이다(테스트에서는 nil).
	conn *websocket.Conn
	// send 는 쓰기 펌프로 가는 버퍼다.
	send chan []byte
	// MbID 는 회원 아이디다.
	MbID string
	// Nick 은 닉네임이다.
	Nick string
	// sessionID 는 재접속 복구용 식별자다.
	sessionID string
	// queueKey 는 대기 중인 큐 키다(없으면 빈 문자열). s.mu 아래에서만 다룬다.
	queueKey string
	// alive 는 하트비트 응답 여부다.
	alive atomic.Bool
	// limiter 는 메시지 빈도 제한이다(읽기 펌프 한 곳에서만 쓴다).
	limiter tokenBucket
}

// queueEntry 는 대기열 한 칸이다. 슬라이스로 관리해 선착순(FIFO)을 지킨다.
type queueEntry struct {
	// client 는 대기자다.
	client *Client
	// rule 은 대전 규칙이다.
	rule string
	// inviteCode 는 초대 대전 코드다(random 은 빈 문자열).
	inviteCode string
	// joinedAt 은 줄 선 시각이다.
	joinedAt time.Time
}

// garbageChunk 는 한 번의 공격으로 생긴 방해 줄 묶음이다. 묶음 안의 줄은 구멍 열이 같다.
type garbageChunk struct {
	// lines 는 줄 수다.
	lines int
	// hole 은 구멍 열(0~8)이다.
	hole int
}

// playerState 는 방 안의 한 사람 상태다.
type playerState struct {
	// mbID 는 회원 아이디다(공개 응답에는 싣지 않는다).
	mbID string
	// nick 은 닉네임이다.
	nick string
	// ready 는 ready 를 보냈는지다.
	ready bool
	// ratingBefore 는 판 시작 때의 레이팅이다.
	ratingBefore int
	// seq 는 마지막으로 받은 lock 번호다.
	seq int64
	// tick 은 마지막으로 받은 엔진 틱이다.
	tick int64
	// locks 는 굳힌 조각 수다.
	locks int
	// lines 는 서버가 받아들인 지운 줄 합이다(판정 기준).
	lines int
	// garbageIn 은 실제로 넣으라고 보낸 방해 줄 합이다.
	garbageIn int
	// pending 은 아직 넣지 않은 방해 줄 묶음이다(오래된 것부터).
	pending garbageQueue
	// board 는 마지막으로 받은 판이다.
	board string
	// score 는 클라이언트가 알린 점수다(표시용).
	score int64
	// violations 는 타당성 검사 위반 횟수다.
	violations int
	// lockTimes 는 최근 1초 lock 시각이다(빈도 제한).
	lockTimes []time.Time
	// afkTimer 는 무입력 패배 타이머다.
	afkTimer *time.Timer
	// afkGen 은 무입력 타이머 세대다. 이미 울려 락을 기다리던 옛 타이머를 무시하는 데 쓴다.
	afkGen int
	// drops 는 이 판의 끊김 횟수다.
	drops int
	// disconnectedAt 은 끊긴 시각이다(연결 중이면 영값).
	disconnectedAt time.Time
	// disconnectedTotal 은 끊김 누적 시간이다.
	disconnectedTotal time.Duration
	// graceTimer 는 재접속 유예 타이머다.
	graceTimer *time.Timer
	// graceGen 은 유예 타이머 세대다(옛 타이머 무시용).
	graceGen int
	// rematch 는 재대결을 신청(수락)했는지다.
	rematch bool
	// oppStateLast 는 이 사람에게 opponent_state 를 마지막으로 보낸 시각이다.
	oppStateLast time.Time
	// oppStateTimer 는 몰아서 보낼 opponent_state 예약이다.
	oppStateTimer *time.Timer
}

// Room 은 대전 방 하나다.
type Room struct {
	// id 는 방 식별자(예측 불가)다.
	id string
	// rule 은 대전 규칙이다.
	rule string
	// mode 는 매칭 모드다.
	mode string
	// inviteCode 는 초대 대전 코드다.
	inviteCode string
	// fee 는 이 판에 걷은 1인 참가비다(초대 대전은 0).
	fee int
	// dbGameID 는 angple_stack_games 행 id 다.
	dbGameID int64
	// seed 는 두 사람이 함께 쓰는 엔진 시드다.
	seed uint32
	// players 는 두 사람이다.
	players [2]*playerState
	// phase 는 ready → countdown → playing → over 다.
	phase string
	// goAt 은 go 를 보낸 시각이다.
	goAt time.Time
	// finished 는 finishGame 이 이미 돌았는지다(한 번만 돌게 한다).
	finished bool
	// readyTimer 는 ready 시간 초과 타이머다.
	readyTimer *time.Timer
	// countdownTimer 는 go 예약이다.
	countdownTimer *time.Timer
	// capTimer 는 제한시간 타이머다.
	capTimer *time.Timer
	// rematchTimer 는 재대결 대기 만료 타이머다.
	rematchTimer *time.Timer
	// rematchStarting 은 재대결이 성립해 새 판을 여는 중인지다.
	rematchStarting bool
}

// Server 는 접속·방·큐를 들고 있는 허브다. 대전 상태는 메모리, 결과·참가비는 store 에 남는다.
type Server struct {
	// mu 는 아래 맵 전부를 지킨다.
	mu sync.Mutex
	// clients 는 회원별 접속이다(회원당 연결 1).
	clients map[string]*Client
	// sessions 는 세션 id → 회원 아이디다(재접속 복구용).
	sessions map[string]string
	// rooms 는 방 id → 방이다.
	rooms map[string]*Room
	// playerRoom 은 회원 아이디 → 방 id 다(진행 중이거나 재대결 대기 중인 방).
	playerRoom map[string]string
	// queues 는 (mode, rule) 별 순서 있는 대기열이다.
	queues map[string][]*queueEntry
	// starting 은 매칭이 성립해 방을 여는 중(참가비 차감 중)인 회원이다. 그 사이 다시 줄 서지 못하게 한다.
	starting map[string]bool
	// store 는 기록·참가비 저장소다.
	store GameStore
	// verifyToken 은 JWT → (회원 아이디, 닉네임) 검증기다.
	verifyToken func(string) (string, string, error)
	// upgrader 는 웹소켓 업그레이더다.
	upgrader websocket.Upgrader
	// timing 은 타이머 값이다.
	timing Timing
	// now 는 시계다(테스트에서 바꾼다).
	now func() time.Time
	// lobby 는 공개 대기 현황 캐시다.
	lobby jsonCache
}
