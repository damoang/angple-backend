package janggisrv

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/janggi"
)

// 상차림 선택 단계 테스트 — DB 없이(store=nil) 허브만 돌린다. 연결(conn)은 쓰지 않고
// 클라이언트의 send 채널로 나간 메시지를 읽는다.

func newTestServer(setupTimeout time.Duration) *Server {
	s := NewServer(nil, nil)
	s.setupTimeout = setupTimeout
	return s
}

func newTestClient(s *Server, mbID string) *Client {
	c := &Client{MbID: mbID, Nick: mbID, isAlive: true, send: make(chan []byte, 256)}
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	s.createSessionForPlayer(c)
	return c
}

// stopAllTimers 는 테스트가 끝날 때 남은 턴·선택 타이머를 멈춘다.
func stopAllTimers(s *Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rooms {
		stopSetupTimerLocked(r)
		if r.GameState != nil && r.GameState.turnTimer != nil {
			r.GameState.turnTimer.Stop()
		}
	}
}

// nextOfType 는 c 에게 온 메시지 중 typ 인 첫 메시지를 돌려준다(다른 종류는 건너뜀).
func nextOfType(t *testing.T, c *Client, typ string, wait time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case raw := <-c.send:
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("%s: 메시지 해석 실패: %v", c.MbID, err)
			}
			if m["type"] == typ {
				return m
			}
		case <-deadline:
			t.Fatalf("%s: %s 메시지가 %v 안에 오지 않았다", c.MbID, typ, wait)
			return nil
		}
	}
}

// countOfType 는 wait 동안 c 에게 온 typ 메시지 수를 센다.
func countOfType(t *testing.T, c *Client, typ string, wait time.Duration) int {
	t.Helper()
	n := 0
	deadline := time.After(wait)
	for {
		select {
		case raw := <-c.send:
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("%s: 메시지 해석 실패: %v", c.MbID, err)
			}
			if m["type"] == typ {
				n++
			}
		case <-deadline:
			return n
		}
	}
}

// startTestMatch 는 두 명을 매칭시키고 (방 id, 초 클라이언트, 한 클라이언트)를 돌려준다.
func startTestMatch(t *testing.T, s *Server, a, b *Client) (string, *Client, *Client) {
	t.Helper()
	s.startMatch(&queueEntry{client: a}, &queueEntry{client: b}, ModeFavorite)
	ma := nextOfType(t, a, "setup_choose", time.Second)
	mb := nextOfType(t, b, "setup_choose", time.Second)
	roomID, _ := ma["roomId"].(string)
	if roomID == "" || mb["roomId"] != roomID {
		t.Fatalf("setup_choose roomId 불일치: %v / %v", ma["roomId"], mb["roomId"])
	}
	for _, m := range []map[string]interface{}{ma, mb} {
		if m["seconds"] != float64(20) {
			t.Fatalf("seconds=%v, 20 이어야 한다", m["seconds"])
		}
		if ms, _ := m["remainingMs"].(float64); ms <= 0 {
			t.Fatalf("remainingMs=%v 는 양수여야 한다", m["remainingMs"])
		}
		if _, has := m["mySetup"]; has {
			t.Fatalf("아직 고르지 않았는데 mySetup 이 있다: %v", m)
		}
	}
	// 매칭 직후엔 game_start 가 아니라 선택 단계다.
	s.mu.RLock()
	room := s.rooms[roomID]
	inSetup, gsNil := room.setupPhase, room.GameState == nil
	s.mu.RUnlock()
	if !inSetup || !gsNil {
		t.Fatalf("매칭 직후 선택 단계여야 한다: setupPhase=%v GameState nil=%v", inSetup, gsNil)
	}
	if ma["playerTeam"] == float64(janggi.TeamCho) {
		return roomID, a, b
	}
	return roomID, b, a
}

func selectSetup(s *Server, c *Client, roomID, setup string) {
	s.handleMessage(c, Message{Type: "setup_select", Data: map[string]interface{}{
		"roomId": roomID, "setup": setup,
	}})
}

func piecesOf(t *testing.T, m map[string]interface{}) []janggi.Piece {
	t.Helper()
	gs, _ := m["gameState"].(map[string]interface{})
	b, err := json.Marshal(gs["pieces"])
	if err != nil {
		t.Fatal(err)
	}
	var out []janggi.Piece
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkSetups(t *testing.T, m map[string]interface{}, cho, han string) {
	t.Helper()
	sv, _ := m["setups"].(map[string]interface{})
	if sv["cho"] != cho || sv["han"] != han {
		t.Fatalf("setups=%v, 기대 cho=%s han=%s", sv, cho, han)
	}
}

func samePieceList(t *testing.T, got, want []janggi.Piece) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("말 수 %d != %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestSetupBothSelectStartsGame(t *testing.T) {
	s := newTestServer(10 * time.Second)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	roomID, cho, han := startTestMatch(t, s, a, b)

	selectSetup(s, cho, roomID, "마상상마")
	for _, c := range []*Client{cho, han} {
		st := nextOfType(t, c, "setup_status", time.Second)
		if st["choSelected"] != true || st["hanSelected"] != false {
			t.Fatalf("setup_status=%v", st)
		}
		// 상대 상차림은 시작 전 공개하지 않는다.
		for k := range st {
			if k != "type" && k != "roomId" && k != "choSelected" && k != "hanSelected" {
				t.Fatalf("setup_status 에 예상 밖 필드 %q", k)
			}
		}
	}
	// 선택은 1회 확정 — 두 번째 선택은 무시된다.
	selectSetup(s, cho, roomID, "상마마상")
	if n := countOfType(t, han, "setup_status", 50*time.Millisecond); n != 0 {
		t.Fatalf("재선택이 무시되지 않았다(setup_status %d건)", n)
	}

	selectSetup(s, han, roomID, "상마마상")
	want := janggi.InitPiecesWithSetup(janggi.SetupMaSangSangMa, janggi.SetupSangMaMaSang)
	for _, c := range []*Client{cho, han} {
		st := nextOfType(t, c, "setup_status", time.Second)
		if st["choSelected"] != true || st["hanSelected"] != true {
			t.Fatalf("setup_status=%v", st)
		}
		gsMsg := nextOfType(t, c, "game_start", time.Second)
		checkSetups(t, gsMsg, "마상상마", "상마마상")
		samePieceList(t, piecesOf(t, gsMsg), want)
	}

	s.mu.RLock()
	room := s.rooms[roomID]
	phase, timer := room.setupPhase, room.setupTimer
	gs := room.GameState
	s.mu.RUnlock()
	if phase || timer != nil {
		t.Fatalf("시작 후 선택 단계·타이머가 남았다: phase=%v timer=%v", phase, timer != nil)
	}
	if gs == nil || gs.ChoSetup != janggi.SetupMaSangSangMa || gs.HanSetup != janggi.SetupSangMaMaSang {
		t.Fatalf("GameState 상차림 기록이 틀렸다: %+v", gs)
	}
}

func TestSetupTimeoutUsesPerTeamDefaults(t *testing.T) {
	s := newTestServer(300 * time.Millisecond)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	roomID, cho, han := startTestMatch(t, s, a, b)

	selectSetup(s, han, roomID, "마상상마") // 한만 고르고 초는 미선택
	gsMsg := nextOfType(t, cho, "game_start", 2*time.Second)
	checkSetups(t, gsMsg, "상마상마", "마상상마") // 초=기본 상마상마
	samePieceList(t, piecesOf(t, gsMsg),
		janggi.InitPiecesWithSetup(janggi.DefaultChoSetup, janggi.SetupMaSangSangMa))
	nextOfType(t, han, "game_start", time.Second)

	// 마감 뒤 늦게 온 선택은 조용히 무시되고, 대국은 두 번 시작되지 않는다.
	selectSetup(s, cho, roomID, "마상마상")
	if n := countOfType(t, cho, "game_start", 150*time.Millisecond); n != 0 {
		t.Fatalf("game_start 가 %d번 더 왔다", n)
	}
	s.mu.RLock()
	gs := s.rooms[roomID].GameState
	s.mu.RUnlock()
	if gs.ChoSetup != janggi.DefaultChoSetup {
		t.Fatalf("늦은 선택이 반영됐다: %s", gs.ChoSetup)
	}
}

func TestSetupNoChoiceEqualsLegacyLayout(t *testing.T) {
	s := newTestServer(30 * time.Millisecond)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	_, cho, _ := startTestMatch(t, s, a, b)
	gsMsg := nextOfType(t, cho, "game_start", 2*time.Second)
	checkSetups(t, gsMsg, "상마상마", "마상마상")
	samePieceList(t, piecesOf(t, gsMsg), janggi.InitPieces())
}

func TestSetupInvalidSelectRejected(t *testing.T) {
	s := newTestServer(10 * time.Second)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	roomID, cho, _ := startTestMatch(t, s, a, b)

	selectSetup(s, cho, roomID, "마마상상")
	if e := nextOfType(t, cho, "error", time.Second); e["message"] == "" {
		t.Fatalf("error 메시지가 비었다: %v", e)
	}
	// 참가자가 아닌 사람의 선택
	outsider := newTestClient(s, "mallory")
	selectSetup(s, outsider, roomID, "상마상마")
	nextOfType(t, outsider, "error", time.Second)
	// 없는 방은 조용히 무시
	selectSetup(s, cho, "no-such-room", "상마상마")
	if n := countOfType(t, cho, "error", 50*time.Millisecond); n != 0 {
		t.Fatalf("없는 방 선택에 error %d건", n)
	}

	s.mu.RLock()
	n := len(s.rooms[roomID].setups)
	phase := s.rooms[roomID].setupPhase
	s.mu.RUnlock()
	if n != 0 || !phase {
		t.Fatalf("잘못된 선택이 기록됐다: setups=%d phase=%v", n, phase)
	}
}

// TestSetupFinishRaceStartsOnce — 타임아웃과 「둘 다 선택」이 겹쳐도 대국은 한 번만 시작한다.
func TestSetupFinishRaceStartsOnce(t *testing.T) {
	s := newTestServer(time.Millisecond)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	s.startMatch(&queueEntry{client: a}, &queueEntry{client: b}, ModeFavorite)
	s.mu.RLock()
	roomID := a.roomID
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.finishSetup(roomID)
		}()
	}
	wg.Wait()
	if n := countOfType(t, a, "game_start", 200*time.Millisecond); n != 1 {
		t.Fatalf("game_start %d번 — 정확히 1번이어야 한다", n)
	}
}

func TestSetupReconnectRestoresPhaseAndGame(t *testing.T) {
	s := newTestServer(10 * time.Second)
	defer stopAllTimers(s)
	a, b := newTestClient(s, "alice"), newTestClient(s, "bob")
	roomID, cho, han := startTestMatch(t, s, a, b)

	selectSetup(s, cho, roomID, "상마마상")
	nextOfType(t, cho, "setup_status", time.Second)
	nextOfType(t, han, "setup_status", time.Second)

	// 선택 단계 중 끊김 → 상대에게 알림
	oldSession := cho.sessionID
	s.handleDisconnect(cho)
	nextOfType(t, han, "opponent_disconnected", time.Second)

	// 같은 회원의 새 연결로 복원 — setup_choose(남은 시간·mySetup) + setup_status
	cho2 := newTestClient(s, cho.MbID)
	s.handleReconnect(cho2, map[string]interface{}{"sessionId": oldSession})
	sc := nextOfType(t, cho2, "setup_choose", time.Second)
	if sc["roomId"] != roomID || sc["playerTeam"] != float64(janggi.TeamCho) || sc["mySetup"] != "상마마상" {
		t.Fatalf("복원 setup_choose=%v", sc)
	}
	if ms, _ := sc["remainingMs"].(float64); ms <= 0 || ms > 10000 {
		t.Fatalf("remainingMs=%v", sc["remainingMs"])
	}
	st := nextOfType(t, cho2, "setup_status", time.Second)
	if st["choSelected"] != true || st["hanSelected"] != false {
		t.Fatalf("복원 setup_status=%v", st)
	}
	nextOfType(t, han, "opponent_reconnected", time.Second)

	// 상대가 고르면 시작 — 복원된 연결이 game_start 를 받는다.
	selectSetup(s, han, roomID, "마상마상")
	gsMsg := nextOfType(t, cho2, "game_start", time.Second)
	checkSetups(t, gsMsg, "상마마상", "마상마상")

	// 대국 중 재접속 → game_restored 의 gameState 에 setups 포함
	sess2 := cho2.sessionID
	s.handleDisconnect(cho2)
	cho3 := newTestClient(s, cho.MbID)
	s.handleReconnect(cho3, map[string]interface{}{"sessionId": sess2})
	gr := nextOfType(t, cho3, "game_restored", time.Second)
	gs, _ := gr["gameState"].(map[string]interface{})
	checkSetups(t, gs, "상마마상", "마상마상")
}

func TestResolveSetups(t *testing.T) {
	cho, han := resolveSetups(map[int]janggi.Setup{})
	if cho != janggi.DefaultChoSetup || han != janggi.DefaultHanSetup {
		t.Fatalf("미선택 기본값 %s/%s", cho, han)
	}
	cho, han = resolveSetups(map[int]janggi.Setup{
		janggi.TeamCho: janggi.SetupMaSangSangMa, janggi.TeamHan: "bogus",
	})
	if cho != janggi.SetupMaSangSangMa || han != janggi.DefaultHanSetup {
		t.Fatalf("선택·무효 혼합 %s/%s", cho, han)
	}
}

func TestSetupRemainingMs(t *testing.T) {
	now := time.Now()
	if got := setupRemainingMs(now.Add(1500*time.Millisecond), now); got != 1500 {
		t.Fatalf("got %d", got)
	}
	if got := setupRemainingMs(now.Add(-time.Second), now); got != 0 {
		t.Fatalf("지난 마감은 0이어야 한다: %d", got)
	}
}
