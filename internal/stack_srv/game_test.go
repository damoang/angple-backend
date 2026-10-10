package stacksrv

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStore 는 GameStore 의 메모리 가짜다.
type fakeStore struct {
	// mu 는 아래를 지킨다.
	mu sync.Mutex
	// nextID 는 다음 대전 id 다.
	nextID int64
	// failCharge 에 든 회원은 참가비 차감이 실패한다.
	failCharge map[string]bool
	// charges 는 "game:mb" 차감 기록이다.
	charges []string
	// refunds 는 "game:mb" 환불 기록이다.
	refunds []string
	// aborts 는 "game:reason" 취소 기록이다.
	aborts []string
	// finishes 는 FinishGame 호출 기록이다.
	finishes []GameResult
	// games 는 CreateGame 호출 기록이다.
	games []NewGame
}

func newFakeStore() *fakeStore {
	return &fakeStore{failCharge: map[string]bool{}}
}

func (f *fakeStore) CreateGame(g NewGame) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.games = append(f.games, g)
	return f.nextID, nil
}

func (f *fakeStore) ChargeEntryFee(gameID int64, mbID string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCharge[mbID] {
		return ErrInsufficientPoint
	}
	f.charges = append(f.charges, fmt.Sprintf("%d:%s", gameID, mbID))
	return nil
}

func (f *fakeStore) RefundEntryFee(gameID int64, mbID string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refunds = append(f.refunds, fmt.Sprintf("%d:%s", gameID, mbID))
	return nil
}

func (f *fakeStore) AbortGame(gameID int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborts = append(f.aborts, fmt.Sprintf("%d:%s", gameID, reason))
	return nil
}

func (f *fakeStore) FinishGame(r GameResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finishes = append(f.finishes, r)
	return nil
}

func (f *fakeStore) Stats(_, _ string) (PlayerStats, bool) {
	return PlayerStats{Rating: DefaultRating}, false
}

func (f *fakeStore) Balance(_ string) int { return 100000 }

func (f *fakeStore) snapshot() (charges, refunds, aborts []string, finishes []GameResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.charges...), append([]string(nil), f.refunds...), append([]string(nil), f.aborts...), append([]GameResult(nil), f.finishes...)
}

// outMsg 는 서버가 보낸 봉투다.
type outMsg struct {
	// Type 은 메시지 이름이다.
	Type string `json:"type"`
	// Data 는 내용이다.
	Data map[string]interface{} `json:"data"`
}

func testTiming() Timing {
	t := DefaultTiming()
	t.Ready = 2 * time.Second
	t.Countdown = time.Millisecond
	t.Inactivity = 5 * time.Second
	t.ReconnectGrace = 2 * time.Second
	t.ReconnectTotal = 4 * time.Second
	t.Rematch = 2 * time.Second
	t.OpponentStateEvery = 0
	t.LocksPerSecond = 0
	return t
}

func newTestServer(store GameStore, timing Timing) *Server {
	return NewServer(store, func(string) (string, string, error) { return "", "", errUnauthorized }, timing)
}

func newTestClient(s *Server, mbID string) *Client {
	c := &Client{send: make(chan []byte, 1024), MbID: mbID, Nick: "nick-" + mbID}
	s.attach(c)
	return c
}

func sendMsg(t *testing.T, s *Server, c *Client, typ string, data interface{}) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	s.handleMessage(c, Message{Type: typ, Data: raw})
}

// nextWhere 는 조건에 맞는 메시지가 올 때까지 읽는다(다른 메시지는 버린다).
func nextWhere(t *testing.T, c *Client, desc string, match func(outMsg) bool) outMsg {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case raw := <-c.send:
			var m outMsg
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			if match(m) {
				return m
			}
		case <-deadline:
			t.Fatalf("%s: %s 를 받지 못했습니다", c.MbID, desc)
		}
	}
}

func next(t *testing.T, c *Client, typ string) outMsg {
	t.Helper()
	return nextWhere(t, c, typ, func(m outMsg) bool { return m.Type == typ })
}

// drain 은 지금까지 쌓인 메시지를 모두 꺼낸다.
func drain(t *testing.T, c *Client) []outMsg {
	t.Helper()
	var out []outMsg
	for {
		select {
		case raw := <-c.send:
			var m outMsg
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			out = append(out, m)
		default:
			return out
		}
	}
}

func countType(msgs []outMsg, typ string) int {
	n := 0
	for _, m := range msgs {
		if m.Type == typ {
			n++
		}
	}
	return n
}

func matched(m outMsg) bool {
	return m.Type == "matching_status" && m.Data["status"] == "matched"
}

// startGame 은 두 사람을 매칭하고 ready 를 보내 go 까지 진행한다. c1 이 0번 자리다.
func startGame(t *testing.T, s *Server, mode, rule string) (*Client, *Client, string) {
	t.Helper()
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	join := map[string]interface{}{"mode": mode, "rule": rule, "invite": "abcd1234"}
	sendMsg(t, s, c1, "join_matching_queue", join)
	sendMsg(t, s, c2, "join_matching_queue", join)
	roomID, _ := nextWhere(t, c1, "matched", matched).Data["roomId"].(string)
	nextWhere(t, c2, "matched", matched)
	sendMsg(t, s, c1, "ready", map[string]interface{}{"roomId": roomID})
	sendMsg(t, s, c2, "ready", map[string]interface{}{"roomId": roomID})
	next(t, c1, "game_start")
	next(t, c1, "go")
	next(t, c2, "go")
	return c1, c2, roomID
}

var emptyBoard = strings.Repeat("0", BoardCells)

func lock(seq, cleared int) map[string]interface{} {
	return map[string]interface{}{"seq": seq, "tick": 0, "cleared": cleared, "board": emptyBoard, "score": 0, "lines": 0}
}

// presetLocks 는 칸 보존식이 넉넉하도록 굳힌 조각 수를 미리 채운다.
func presetLocks(s *Server, roomID string, locks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.rooms[roomID].players {
		p.locks = locks
	}
}

func TestMatchChargesRandomFeeAndStartsWithSharedSeed(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	join := map[string]interface{}{"mode": "random", "rule": "attack"}
	sendMsg(t, s, c1, "join_matching_queue", join)
	sendMsg(t, s, c2, "join_matching_queue", join)
	m := nextWhere(t, c1, "matched", matched)
	if m.Data["entryFeeCharged"] != float64(EntryFee) {
		t.Fatalf("entryFeeCharged = %v", m.Data["entryFeeCharged"])
	}
	opp, _ := m.Data["opponent"].(map[string]interface{})
	if opp["nickname"] != "nick-p2" || opp["mbId"] != nil {
		t.Fatalf("opponent = %v (닉네임만 실어야 한다)", opp)
	}
	roomID, _ := m.Data["roomId"].(string)
	sendMsg(t, s, c1, "ready", map[string]interface{}{"roomId": roomID})
	sendMsg(t, s, c2, "ready", map[string]interface{}{"roomId": roomID})
	g1, g2 := next(t, c1, "game_start"), next(t, c2, "game_start")
	if g1.Data["seed"] != g2.Data["seed"] {
		t.Fatal("두 사람의 시드가 달라서는 안 된다")
	}
	spec, _ := g1.Data["ruleSpec"].(map[string]interface{})
	if spec["id"] != "attack" || spec["timeLimitSec"] != float64(600) {
		t.Fatalf("ruleSpec = %v", spec)
	}
	charges, _, _, _ := store.snapshot()
	if len(charges) != 2 {
		t.Fatalf("charges = %v", charges)
	}
}

func TestAttackSendsGarbageAfterNonClearingLock(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2, _ := startGame(t, s, ModeRandom, RuleAttack)
	for i := 1; i <= 9; i++ {
		sendMsg(t, s, c1, "lock", lock(i, 0))
	}
	// 10번째 lock 에서 4줄: 4×10 − 9×4 = 4 ≥ 0 이라 받아들여진다.
	sendMsg(t, s, c1, "lock", lock(10, 4))
	q := next(t, c2, "garbage_queued")
	if q.Data["pending"] != float64(4) {
		t.Fatalf("pending = %v", q.Data["pending"])
	}
	sendMsg(t, s, c2, "lock", lock(1, 0))
	a := next(t, c2, "garbage_apply")
	if a.Data["lines"] != float64(4) || a.Data["pending"] != float64(0) {
		t.Fatalf("garbage_apply = %v", a.Data)
	}
	if h, _ := a.Data["hole"].(float64); h < 0 || h > 8 {
		t.Fatalf("hole = %v", h)
	}
	st := next(t, c1, "opponent_state")
	if st.Data["board"] != emptyBoard {
		t.Fatalf("opponent_state = %v", st.Data)
	}
}

func TestAttackOffsetCancelsOwnPendingFirst(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c1, c2, roomID := startGame(t, s, ModeRandom, RuleAttack)
	presetLocks(s, roomID, 30)
	sendMsg(t, s, c1, "lock", lock(1, 4))
	next(t, c2, "garbage_queued")
	// 2줄 = 공격 1 → 내 대기 4 중 1 상쇄, 상대에게는 0.
	sendMsg(t, s, c2, "lock", lock(1, 2))
	q := next(t, c2, "garbage_queued")
	if q.Data["pending"] != float64(3) {
		t.Fatalf("pending after offset = %v", q.Data["pending"])
	}
	if n := countType(drain(t, c1), "garbage_queued"); n != 0 {
		t.Fatalf("상쇄로 다 쓴 공격이 상대에게 갔다 (%d)", n)
	}
	// 4줄 = 공격 4 → 남은 대기 3 상쇄, 1 줄만 상대에게.
	sendMsg(t, s, c2, "lock", lock(2, 4))
	q1 := next(t, c1, "garbage_queued")
	if q1.Data["pending"] != float64(1) {
		t.Fatalf("상대 대기 = %v", q1.Data["pending"])
	}
}

func TestAttackPendingCapAndPerLockCap(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c1, c2, roomID := startGame(t, s, ModeRandom, RuleAttack)
	// 35 + 4 = 39 조각·16줄 → 칸 12, 받는 쪽 36 조각 → 칸 144 (둘 다 0~162 안).
	presetLocks(s, roomID, 35)
	for i := 1; i <= 4; i++ {
		sendMsg(t, s, c1, "lock", lock(i, 4))
	}
	s.mu.Lock()
	pending := s.rooms[roomID].players[1].pending.total()
	s.mu.Unlock()
	if pending != GarbagePendingMax {
		t.Fatalf("대기 상한 = %d, want %d", pending, GarbagePendingMax)
	}
	drain(t, c2)
	sendMsg(t, s, c2, "lock", lock(1, 0))
	total, last := 0, -1.0
	for _, m := range drain(t, c2) {
		if m.Type != "garbage_apply" {
			continue
		}
		total += int(m.Data["lines"].(float64))
		last = m.Data["pending"].(float64)
	}
	if total != GarbagePerLockMax || last != float64(GarbagePendingMax-GarbagePerLockMax) {
		t.Fatalf("한 번에 넣은 줄 = %d (남은 대기 %v)", total, last)
	}
}

func TestSprint40DoesNotSendGarbage(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c1, c2, roomID := startGame(t, s, ModeRandom, RuleSprint40)
	presetLocks(s, roomID, 20)
	sendMsg(t, s, c1, "lock", lock(1, 4))
	if n := countType(drain(t, c2), "garbage_queued"); n != 0 {
		t.Fatal("sprint40 에서 방해 줄이 나갔다")
	}
	p := next(t, c1, "progress")
	you, _ := p.Data["you"].(map[string]interface{})
	if you["lines"] != float64(4) {
		t.Fatalf("progress = %v", p.Data)
	}
}

func TestSprint40FirstToFortyWins(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2, _ := startGame(t, s, ModeRandom, RuleSprint40)
	seq := 0
	// 0줄 9번 + 4줄 1번 = 칸 +4. 10회 반복하면 40줄.
	for round := 0; round < 10; round++ {
		for i := 0; i < 9; i++ {
			seq++
			sendMsg(t, s, c1, "lock", lock(seq, 0))
		}
		seq++
		sendMsg(t, s, c1, "lock", lock(seq, 4))
	}
	over := next(t, c1, "game_over")
	if over.Data["reason"] != "goal" || over.Data["youWon"] != true || over.Data["winner"] != "nick-p1" {
		t.Fatalf("game_over = %v", over.Data)
	}
	result, _ := over.Data["result"].(map[string]interface{})
	if result["lines"] != float64(40) || result["timeMs"] == nil {
		t.Fatalf("result = %v", result)
	}
	if lost := next(t, c2, "game_over"); lost.Data["youWon"] != false {
		t.Fatalf("상대 game_over = %v", lost.Data)
	}
	_, _, _, finishes := store.snapshot()
	if len(finishes) != 1 || finishes[0].Winner != "p1" || finishes[0].P1Lines != 40 {
		t.Fatalf("finishes = %+v", finishes)
	}
}

func TestTimeLimitJudgement(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, _, roomID := startGame(t, s, ModeRandom, RuleSprint40)
	s.mu.Lock()
	s.rooms[roomID].players[0].lines = 5
	s.rooms[roomID].players[1].lines = 12
	s.mu.Unlock()
	s.onTimeLimit(roomID)
	over := next(t, c1, "game_over")
	if over.Data["reason"] != "time" || over.Data["winner"] != "nick-p2" {
		t.Fatalf("sprint40 시간 판정 = %v", over.Data)
	}

	s2 := newTestServer(store, testTiming())
	a1, _, attackRoom := startGame(t, s2, ModeRandom, RuleAttack)
	s2.onTimeLimit(attackRoom)
	draw := next(t, a1, "game_over")
	if draw.Data["reason"] != "draw" || draw.Data["winner"] != nil {
		t.Fatalf("attack 10분 = %v", draw.Data)
	}
}

func TestCellBalanceViolationsEndInCheat(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2, _ := startGame(t, s, ModeRandom, RuleAttack)
	// 첫 조각부터 4줄 — 4×1 − 9×4 < 0. 받아들이지 않고 위반을 센다.
	for i := 1; i <= MaxViolations; i++ {
		sendMsg(t, s, c1, "lock", lock(i, 4))
	}
	if n := countType(drain(t, c2), "garbage_queued"); n != 0 {
		t.Fatal("위반 lock 의 공격이 상대에게 갔다")
	}
	over := next(t, c1, "game_over")
	if over.Data["reason"] != "cheat" || over.Data["youWon"] != false {
		t.Fatalf("game_over = %v", over.Data)
	}
	_, _, _, finishes := store.snapshot()
	if len(finishes) != 1 || finishes[0].Winner != "p2" || finishes[0].Reason != "cheat" {
		t.Fatalf("finishes = %+v", finishes)
	}
}

func TestLockValidationRejects(t *testing.T) {
	timing := testTiming()
	timing.LocksPerSecond = 3
	s := newTestServer(newFakeStore(), timing)
	c1, _, _ := startGame(t, s, ModeRandom, RuleAttack)
	errCode := func() string {
		m := next(t, c1, statusError)
		code, _ := m.Data["code"].(string)
		return code
	}

	bad := lock(1, 0)
	bad["board"] = strings.Repeat("9", BoardCells)
	sendMsg(t, s, c1, "lock", bad)
	if code := errCode(); code != "bad_lock" {
		t.Fatalf("잘못된 판 = %s", code)
	}
	ahead := lock(2, 0)
	ahead["tick"] = 100000
	sendMsg(t, s, c1, "lock", ahead)
	if code := errCode(); code != "bad_tick" {
		t.Fatalf("앞선 틱 = %s", code)
	}
	sendMsg(t, s, c1, "lock", lock(3, 0))
	sendMsg(t, s, c1, "lock", lock(3, 0))
	if code := errCode(); code != "bad_seq" {
		t.Fatalf("같은 seq = %s", code)
	}
	// 1초 창 안의 네 번째 lock 은 빈도 제한에 걸린다.
	sendMsg(t, s, c1, "lock", lock(4, 0))
	if code := errCode(); code != "rate_limited" {
		t.Fatalf("빈도 = %s", code)
	}
}

func TestFinishGameRunsOnce(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2, roomID := startGame(t, s, ModeRandom, RuleAttack)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.finishGame(roomID, i%2, "topout")
		}(i)
	}
	wg.Wait()
	s.handleMessage(c1, Message{Type: "surrender"})
	s.onTimeLimit(roomID)
	_, _, _, finishes := store.snapshot()
	if len(finishes) != 1 {
		t.Fatalf("FinishGame 호출 = %d", len(finishes))
	}
	if n := countType(drain(t, c1), "game_over"); n != 1 {
		t.Fatalf("c1 game_over = %d", n)
	}
	if n := countType(drain(t, c2), "game_over"); n != 1 {
		t.Fatalf("c2 game_over = %d", n)
	}
}

func TestReadyTimeoutRefundsBoth(t *testing.T) {
	store := newFakeStore()
	timing := testTiming()
	timing.Ready = 30 * time.Millisecond
	s := newTestServer(store, timing)
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	join := map[string]interface{}{"mode": "random", "rule": "attack"}
	sendMsg(t, s, c1, "join_matching_queue", join)
	sendMsg(t, s, c2, "join_matching_queue", join)
	m := nextWhere(t, c1, "ready_timeout", func(m outMsg) bool { return m.Data["code"] == "ready_timeout" })
	if m.Data["refunded"] != true {
		t.Fatalf("ready_timeout = %v", m.Data)
	}
	_, refunds, aborts, finishes := store.snapshot()
	if len(refunds) != 2 || len(aborts) != 1 || !strings.HasSuffix(aborts[0], "ready_timeout") || len(finishes) != 0 {
		t.Fatalf("refunds=%v aborts=%v finishes=%v", refunds, aborts, finishes)
	}
	nextWhere(t, c2, "ready_timeout", func(m outMsg) bool { return m.Data["code"] == "ready_timeout" })
	// 방이 닫혔으니 다시 줄 설 수 있다.
	sendMsg(t, s, c2, "join_matching_queue", join)
	nextWhere(t, c2, "waiting", func(m outMsg) bool { return m.Data["status"] == "waiting" })
}

func TestMatchChargeFailureRefundsPayer(t *testing.T) {
	store := newFakeStore()
	store.failCharge["p2"] = true
	s := newTestServer(store, testTiming())
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	join := map[string]interface{}{"mode": "random", "rule": "sprint40"}
	sendMsg(t, s, c1, "join_matching_queue", join)
	sendMsg(t, s, c2, "join_matching_queue", join)
	e1 := nextWhere(t, c1, statusError, func(m outMsg) bool { return m.Data["status"] == statusError })
	e2 := nextWhere(t, c2, statusError, func(m outMsg) bool { return m.Data["status"] == statusError })
	if e1.Data["code"] != codeOpponentPaymentFailed || e2.Data["code"] != "insufficient_point" {
		t.Fatalf("codes = %v / %v", e1.Data["code"], e2.Data["code"])
	}
	charges, refunds, aborts, _ := store.snapshot()
	if len(charges) != 1 || len(refunds) != 1 || refunds[0] != charges[0] || !strings.HasSuffix(aborts[0], "entry_fee_failed") {
		t.Fatalf("charges=%v refunds=%v aborts=%v", charges, refunds, aborts)
	}
}

func TestRematchChargesAgainAndRefundsOnFailure(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2, _ := startGame(t, s, ModeRandom, RuleAttack)
	s.handleMessage(c1, Message{Type: "surrender"})
	over := next(t, c1, "game_over")
	if over.Data["rematchFee"] != float64(EntryFee) {
		t.Fatalf("재대결 참가비 사전 안내 = %v", over.Data)
	}

	// 첫 재대결: 신청 → 상대에게 rematch_offer(참가비 안내) → 수락 → 다시 1,000P씩.
	s.handleMessage(c1, Message{Type: "rematch"})
	offer := next(t, c2, "rematch_offer")
	if notice, _ := offer.Data["feeNotice"].(string); !strings.Contains(notice, "1,000P") {
		t.Fatalf("feeNotice = %v", offer.Data)
	}
	s.handleMessage(c2, Message{Type: "rematch"})
	m := nextWhere(t, c1, "matched", matched)
	charges, _, _, _ := store.snapshot()
	if len(charges) != 4 {
		t.Fatalf("재대결도 판마다 차감해야 한다: %v", charges)
	}

	// 두 번째 재대결에서 p2 잔액 부족 → p1 즉시 환불.
	roomID, _ := m.Data["roomId"].(string)
	sendMsg(t, s, c1, "ready", map[string]interface{}{"roomId": roomID})
	sendMsg(t, s, c2, "ready", map[string]interface{}{"roomId": roomID})
	next(t, c1, "go")
	s.handleMessage(c2, Message{Type: "surrender"})
	next(t, c1, "game_over")
	store.mu.Lock()
	store.failCharge["p2"] = true
	store.mu.Unlock()
	s.handleMessage(c1, Message{Type: "rematch"})
	s.handleMessage(c2, Message{Type: "rematch"})
	r1 := next(t, c1, "rematch_canceled")
	r2 := next(t, c2, "rematch_canceled")
	if r1.Data["reason"] != codeOpponentPaymentFailed || r2.Data["reason"] != "insufficient_point" {
		t.Fatalf("rematch_canceled = %v / %v", r1.Data, r2.Data)
	}
	charges, refunds, _, _ := store.snapshot()
	if len(charges) != 5 || len(refunds) != 1 || refunds[0] != charges[4] {
		t.Fatalf("charges=%v refunds=%v", charges, refunds)
	}
}

func TestFavoriteIsFreeAndNeedsSameRule(t *testing.T) {
	store := newFakeStore()
	s := newTestServer(store, testTiming())
	c1, c2 := newTestClient(s, "p1"), newTestClient(s, "p2")
	sendMsg(t, s, c1, "join_matching_queue", map[string]interface{}{"mode": "favorite", "rule": "attack", "invite": "abcd1234"})
	sendMsg(t, s, c2, "join_matching_queue", map[string]interface{}{"mode": "favorite", "rule": "sprint40", "invite": "abcd1234"})
	e := nextWhere(t, c2, "rule_mismatch", func(m outMsg) bool { return m.Data["status"] == statusError })
	if e.Data["code"] != "rule_mismatch" {
		t.Fatalf("code = %v", e.Data["code"])
	}
	sendMsg(t, s, c2, "join_matching_queue", map[string]interface{}{"mode": "favorite", "rule": "attack", "invite": "abcd1234"})
	m := nextWhere(t, c2, "matched", matched)
	if m.Data["entryFeeCharged"] != float64(0) {
		t.Fatalf("초대 대전 참가비 = %v", m.Data["entryFeeCharged"])
	}
	charges, _, _, _ := store.snapshot()
	if len(charges) != 0 {
		t.Fatalf("초대 대전에서 차감됨: %v", charges)
	}
}

func TestJoinRejectsClosedRulesAndBadInvite(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c := newTestClient(s, "p1")
	cases := []struct {
		data map[string]interface{}
		code string
	}{
		{map[string]interface{}{"mode": "random", "rule": "score120"}, "invalid_rule"},
		{map[string]interface{}{"mode": "random", "rule": "survival"}, "invalid_rule"},
		{map[string]interface{}{"mode": "rating", "rule": "attack"}, "invalid_mode"},
		{map[string]interface{}{"mode": "favorite", "rule": "attack"}, "invalid_invite"},
	}
	for _, tc := range cases {
		sendMsg(t, s, c, "join_matching_queue", tc.data)
		m := next(t, c, "matching_status")
		if m.Data["code"] != tc.code {
			t.Fatalf("%v → %v, want %s", tc.data, m.Data["code"], tc.code)
		}
	}
}

func TestDisconnectLimitAndReconnect(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	c1, c2, _ := startGame(t, s, ModeRandom, RuleAttack)
	cur := c1
	for i := 0; i < MaxDrops; i++ {
		s.handleDisconnect(cur)
		next(t, c2, "opponent_disconnected")
		nc := newTestClient(s, "p1")
		sendMsg(t, s, nc, "reconnect", map[string]interface{}{"sessionId": cur.sessionID})
		next(t, nc, "game_restored")
		next(t, c2, "opponent_reconnected")
		cur = nc
	}
	// 네 번째 끊김은 곧바로 패배.
	s.handleDisconnect(cur)
	over := next(t, c2, "game_over")
	if over.Data["reason"] != "disconnect" || over.Data["youWon"] != true {
		t.Fatalf("game_over = %v", over.Data)
	}
}

func TestReconnectGraceExpiryLoses(t *testing.T) {
	timing := testTiming()
	timing.ReconnectGrace = 30 * time.Millisecond
	s := newTestServer(newFakeStore(), timing)
	c1, c2, _ := startGame(t, s, ModeRandom, RuleAttack)
	s.handleDisconnect(c1)
	over := next(t, c2, "game_over")
	if over.Data["reason"] != "disconnect" || over.Data["youWon"] != true {
		t.Fatalf("game_over = %v", over.Data)
	}
}

func TestInactivityLoses(t *testing.T) {
	timing := testTiming()
	timing.Inactivity = 40 * time.Millisecond
	s := newTestServer(newFakeStore(), timing)
	c1, _, _ := startGame(t, s, ModeRandom, RuleAttack)
	over := next(t, c1, "game_over")
	if over.Data["reason"] != "inactivity" {
		t.Fatalf("game_over = %v", over.Data)
	}
}

func TestOneConnectionPerMember(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	a := newTestClient(s, "p1")
	sendMsg(t, s, a, "join_matching_queue", map[string]interface{}{"mode": "random", "rule": "attack"})
	b := newTestClient(s, "p1")
	s.mu.Lock()
	current, queued := s.clients["p1"], len(s.queues[queueKey(ModeRandom, RuleAttack)])
	s.mu.Unlock()
	if current != b || queued != 0 {
		t.Fatalf("새 연결이 자리를 잡고 옛 연결의 대기는 빠져야 한다 (queued=%d)", queued)
	}
	// 옛 연결의 끊김 처리는 새 연결을 건드리지 않는다.
	s.handleDisconnect(a)
	s.mu.Lock()
	current = s.clients["p1"]
	s.mu.Unlock()
	if current != b {
		t.Fatal("옛 연결 정리가 새 연결을 지웠다")
	}
}

func TestRoomLimit(t *testing.T) {
	s := newTestServer(newFakeStore(), testTiming())
	s.mu.Lock()
	for i := 0; i < MaxRooms; i++ {
		id := fmt.Sprintf("r%d", i)
		s.rooms[id] = &Room{id: id}
	}
	s.mu.Unlock()
	c := newTestClient(s, "p1")
	sendMsg(t, s, c, "join_matching_queue", map[string]interface{}{"mode": "random", "rule": "attack"})
	if m := next(t, c, "matching_status"); m.Data["code"] != "server_busy" {
		t.Fatalf("방 상한 = %v", m.Data)
	}
}

func TestTokenBucket(t *testing.T) {
	b := newTokenBucket(2, 1)
	t0 := time.Unix(0, 0)
	got := make([]bool, 0, 3)
	for i := 0; i < 3; i++ {
		got = append(got, b.allow(t0))
	}
	if !got[0] || !got[1] || got[2] {
		t.Fatalf("burst 2 = %v", got)
	}
	if !b.allow(t0.Add(time.Second)) {
		t.Fatal("1초 뒤 충전되지 않았다")
	}
}
