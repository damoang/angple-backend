package janggisrv

import (
	"log"
	"time"

	"github.com/damoang/angple-backend/internal/janggi"
)

// 상차림 선택 단계 — 매칭 성사 후 game_start 전에 양쪽이 동시에 자기 상차림을 고른다.
//
//	서버 → 양쪽  setup_choose {roomId, playerTeam, seconds, remainingMs[, mySetup]}
//	클라 → 서버  setup_select {roomId, setup}
//	서버 → 양쪽  setup_status {roomId, choSelected, hanSelected} (상대 상차림은 비공개)
//
// 둘 다 고르거나 제한시간이 지나면 고르지 않은 진영은 기본값으로 game_start 한다.
// 구 클라이언트는 setup_choose 를 모르므로 무시하고, 제한시간 뒤 기본값으로 시작된다.

// openSetupLocked 는 방을 상차림 선택 단계로 연다. 호출자가 s.mu 를 잡고 있어야 한다.
// 타이머 콜백은 락을 다시 잡고 setupPhase 를 확인하므로, 이미 시작된 대국이나
// 사라진 방에는 아무 일도 하지 않는다.
func (s *Server) openSetupLocked(room *Room) {
	timeout := s.setupTimeout
	if timeout <= 0 {
		timeout = SetupTimeout
	}
	roomID := room.ID
	room.setupPhase = true
	room.setups = map[int]janggi.Setup{}
	room.setupDeadline = time.Now().Add(timeout)
	room.setupTimer = time.AfterFunc(timeout, func() {
		s.finishSetup(roomID)
	})
}

// stopSetupTimerLocked 는 선택 단계 타이머를 멈춘다. 호출자가 s.mu 를 잡고 있어야 한다.
func stopSetupTimerLocked(room *Room) {
	if room.setupTimer != nil {
		room.setupTimer.Stop()
		room.setupTimer = nil
	}
}

// setupRemainingMs 는 선택 마감까지 남은 밀리초다(지났으면 0).
func setupRemainingMs(deadline, now time.Time) int64 {
	d := deadline.Sub(now)
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

// resolveSetups 는 고른 값을 확정한다. 고르지 않은(또는 잘못된) 진영은 그 진영의
// 기본 상차림이 된다 — 둘 다 미선택이면 상차림 도입 전 고정 배치와 같다.
func resolveSetups(chosen map[int]janggi.Setup) (cho, han janggi.Setup) {
	cho, han = janggi.DefaultChoSetup, janggi.DefaultHanSetup
	if s, ok := chosen[janggi.TeamCho]; ok && janggi.IsValidSetup(s) {
		cho = s
	}
	if s, ok := chosen[janggi.TeamHan]; ok && janggi.IsValidSetup(s) {
		han = s
	}
	return cho, han
}

// setupChooseMsgLocked 는 한 명에게 보낼 setup_choose 메시지다(재접속 복원에도 쓴다).
// 호출자가 s.mu 를 잡고 있어야 한다.
func setupChooseMsgLocked(room *Room, mbID string, now time.Time) map[string]interface{} {
	team := room.PlayerColors[mbID]
	msg := map[string]interface{}{
		"type": "setup_choose", "roomId": room.ID,
		"playerTeam":  team,
		"seconds":     int(SetupTimeout.Seconds()),
		"remainingMs": setupRemainingMs(room.setupDeadline, now),
	}
	if s, ok := room.setups[team]; ok {
		msg["mySetup"] = string(s)
	}
	return msg
}

// setupStatusMsgLocked 는 선택 현황 메시지다. 상대가 무엇을 골랐는지는 담지 않는다.
// 호출자가 s.mu 를 잡고 있어야 한다.
func setupStatusMsgLocked(room *Room) map[string]interface{} {
	_, cho := room.setups[janggi.TeamCho]
	_, han := room.setups[janggi.TeamHan]
	return map[string]interface{}{
		"type": "setup_status", "roomId": room.ID,
		"choSelected": cho, "hanSelected": han,
	}
}

// sendSetupChoose 는 방의 접속 중인 참가자에게 setup_choose 를 보낸다.
func (s *Server) sendSetupChoose(roomID string) {
	now := time.Now()
	type out struct {
		c   *Client
		msg map[string]interface{}
	}
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || !room.setupPhase {
		s.mu.Unlock()
		return
	}
	clients := s.clientsInRoomLocked(room)
	outs := make([]out, 0, len(clients))
	for _, c := range clients {
		outs = append(outs, out{c, setupChooseMsgLocked(room, c.MbID, now)})
	}
	s.mu.Unlock()
	for _, o := range outs {
		s.sendToClient(o.c, o.msg)
	}
}

// handleSetupSelect — 선택은 1회 확정. 선택 단계가 아니거나 이미 고른 경우는
// 조용히 무시한다(타임아웃 직후 늦게 도착한 선택 등 정상 경합이 있어서).
func (s *Server) handleSetupSelect(client *Client, data map[string]interface{}) {
	roomID, _ := data["roomId"].(string) //nolint:errcheck // 없으면 방 조회 실패로 처리
	raw, _ := data["setup"].(string)     //nolint:errcheck // 없으면 빈 문자열 → 무효
	setup := janggi.Setup(raw)
	if !janggi.IsValidSetup(setup) {
		s.sendToClient(client, errMsg("올바르지 않은 상차림입니다."))
		return
	}

	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || !room.setupPhase {
		s.mu.Unlock()
		return
	}
	team := room.PlayerColors[client.MbID]
	if team == 0 {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("이 대국의 참가자가 아닙니다."))
		return
	}
	if _, done := room.setups[team]; done {
		s.mu.Unlock()
		return
	}
	room.setups[team] = setup
	bothChosen := len(room.setups) >= 2
	status := setupStatusMsgLocked(room)
	clients := s.clientsInRoomLocked(room)
	s.mu.Unlock()

	for _, c := range clients {
		s.sendToClient(c, status)
	}
	if bothChosen {
		s.finishSetup(roomID)
	}
}

// finishSetup 은 선택 단계를 닫고 대국을 시작한다. 「둘 다 선택」과 「타임아웃」이
// 동시에 와도 락 안에서 setupPhase 를 먼저 내린 한 쪽만 createGame 까지 간다.
func (s *Server) finishSetup(roomID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || !room.setupPhase {
		s.mu.Unlock()
		return
	}
	room.setupPhase = false
	stopSetupTimerLocked(room)
	choSetup, hanSetup := resolveSetups(room.setups)
	var choMbID, hanMbID string
	for mbID, team := range room.PlayerColors {
		switch team {
		case janggi.TeamCho:
			choMbID = mbID
		case janggi.TeamHan:
			hanMbID = mbID
		}
	}
	s.mu.Unlock()

	log.Printf("[janggi] setup done room=%s cho=%s han=%s", roomID, choSetup, hanSetup)
	s.createGame(roomID, choMbID, hanMbID, choSetup, hanSetup)
}
