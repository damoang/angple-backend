package stacksrv

import (
	"encoding/json"
	"errors"
	"log"
	"time"
)

// errUnauthorized 는 토큰이 없거나 회원을 확정할 수 없을 때다.
var errUnauthorized = errors.New(codeUnauthorized)

// codeUnauthorized 는 인증 실패 코드다.
const codeUnauthorized = "unauthorized"

// handleMessage 는 메시지 종류별로 나눈다. ready 만 roomId 를 싣고, 나머지는 연결의 방을 쓴다.
func (s *Server) handleMessage(c *Client, msg Message) {
	switch msg.Type {
	case "join_matching_queue":
		s.handleJoin(c, msg.Data)
	case "cancel_matching":
		s.handleCancelMatching(c)
	case "ready":
		s.handleReady(c, msg.Data)
	case "lock":
		s.handleLock(c, msg.Data)
	case "topped_out":
		s.handleToppedOut(c, msg.Data)
	case "surrender":
		s.handleSurrender(c)
	case "rematch":
		s.handleRematch(c)
	case "rematch_decline":
		s.handleRematchDecline(c)
	case "reconnect":
		s.handleReconnect(c, msg.Data)
	case "ping", "pong":
		c.alive.Store(true)
	default:
		s.sendError(c, "unknown_type", "알 수 없는 요청입니다.")
	}
}

// 자주 쓰는 메시지 이름·오류 코드
const (
	msgMatchingStatus         = "matching_status"
	msgRematchCanceled        = "rematch_canceled"
	codeServerBusy            = "server_busy"
	codeOpponentPaymentFailed = "opponent_payment_failed"
	statusError               = "error"
)

// sendBadMessage 는 형식이 틀린 요청에 답한다.
func (s *Server) sendBadMessage(c *Client) {
	s.sendError(c, "bad_message", "요청 형식이 올바르지 않습니다.")
}

// decodeData 는 data 를 v 로 푼다. 비어 있으면 빈 객체로 본다.
func decodeData(raw json.RawMessage, v interface{}) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// roomOfLocked 는 회원이 든 방과 자리(0·1)를 돌려준다. 호출자가 s.mu 를 잡고 있어야 한다.
func (s *Server) roomOfLocked(mbID string) (*Room, int) {
	room := s.rooms[s.playerRoom[mbID]]
	if room == nil {
		return nil, -1
	}
	for i, p := range room.players {
		if p != nil && p.mbID == mbID {
			return room, i
		}
	}
	return nil, -1
}

// handleDisconnect 는 끊긴 연결을 정리한다.
// 진행 중인 판이면 재접속 유예(15초)를 건다. 한 판에서 끊김이 3회를 넘거나 누적 30초를 넘기면 패배다.
// 재대결 대기 중인 방이면 재대결을 취소한다.
func (s *Server) handleDisconnect(c *Client) {
	s.mu.Lock()
	s.removeFromQueuesLocked(c)
	delete(s.sessions, c.sessionID)
	if s.clients[c.MbID] != c {
		// 같은 회원의 새 연결이 이미 자리를 잡았다 — 판 상태는 건드리지 않는다.
		s.mu.Unlock()
		return
	}
	delete(s.clients, c.MbID)
	room, idx := s.roomOfLocked(c.MbID)
	if room == nil {
		s.mu.Unlock()
		return
	}
	if room.finished {
		s.cancelRematchLocked(room, "left")
		s.mu.Unlock()
		return
	}
	if room.phase == phaseReady {
		// ready 단계는 ready 타이머가 정리한다(취소·환불).
		s.mu.Unlock()
		return
	}
	finishNow := s.markDisconnectedLocked(room, idx)
	roomID := room.id
	s.mu.Unlock()
	log.Printf("[stack] disconnected room=%s seat=%d", roomID, idx)
	if finishNow {
		s.finishGame(roomID, 1-idx, "disconnect")
	}
}

// markDisconnectedLocked 는 끊김을 기록하고 유예 타이머를 건다. 곧바로 패배면 true.
func (s *Server) markDisconnectedLocked(room *Room, idx int) bool {
	p := room.players[idx]
	now := s.now()
	p.drops++
	p.disconnectedAt = now
	// 끊긴 동안은 무입력 타이머를 멈춘다(돌아오면 새로 건다). 유예와 무입력이 겹쳐 판정이 바뀌지 않게.
	if p.afkTimer != nil {
		p.afkTimer.Stop()
		p.afkTimer = nil
	}
	p.afkGen++
	remaining := s.timing.ReconnectTotal - p.disconnectedTotal
	if p.drops > MaxDrops || remaining <= 0 {
		return true
	}
	grace := s.timing.ReconnectGrace
	if remaining < grace {
		grace = remaining
	}
	if p.graceTimer != nil {
		p.graceTimer.Stop()
	}
	p.graceGen++
	roomID, gen := room.id, p.graceGen
	p.graceTimer = time.AfterFunc(grace, func() { s.onGraceExpired(roomID, idx, gen) })
	opp := room.players[1-idx]
	s.emitToLocked(opp.mbID, "opponent_disconnected", map[string]interface{}{"timeout": int(grace.Seconds())})
	return false
}

// onGraceExpired 는 유예 안에 돌아오지 않은 사람을 패배 처리한다.
func (s *Server) onGraceExpired(roomID string, idx, gen int) {
	s.mu.Lock()
	room := s.rooms[roomID]
	lost := room != nil && !room.finished && room.players[idx].graceGen == gen && !room.players[idx].disconnectedAt.IsZero()
	s.mu.Unlock()
	if lost {
		s.finishGame(roomID, 1-idx, "disconnect")
	}
}

// handleReconnect 는 세션 id 로 진행 중인 판에 다시 붙는다.
// 세션은 같은 회원의 것만 받는다 — 남의 세션 id 로 남의 판에 들어올 수 없다.
func (s *Server) handleReconnect(c *Client, raw json.RawMessage) {
	var d struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeData(raw, &d); err != nil || d.SessionID == "" {
		s.sendBadMessage(c)
		return
	}
	s.mu.Lock()
	owner, known := s.sessions[d.SessionID]
	room, idx := s.roomOfLocked(c.MbID)
	// 끊긴 옛 세션은 이미 지워졌을 수 있다. 회원이 같으면(JWT 로 확정) 복구를 허용한다.
	if (known && owner != c.MbID) || room == nil || room.finished {
		s.mu.Unlock()
		s.sendError(c, "no_game", "복구할 대전이 없습니다.")
		return
	}
	p := room.players[idx]
	if !p.disconnectedAt.IsZero() {
		p.disconnectedTotal += s.now().Sub(p.disconnectedAt)
		p.disconnectedAt = time.Time{}
		if p.graceTimer != nil {
			p.graceTimer.Stop()
			p.graceTimer = nil
		}
		p.graceGen++
		s.emitToLocked(room.players[1-idx].mbID, "opponent_reconnected", nil)
		if room.phase == phasePlaying {
			s.armInactivityLocked(room, idx)
		}
	}
	s.emit(c, "game_restored", s.restoredPayloadLocked(room, idx))
	s.mu.Unlock()
}

// restoredPayloadLocked 는 game_restored 내용이다.
func (s *Server) restoredPayloadLocked(room *Room, idx int) map[string]interface{} {
	p, opp := room.players[idx], room.players[1-idx]
	var elapsed int64
	if !room.goAt.IsZero() {
		elapsed = s.now().Sub(room.goAt).Milliseconds()
	}
	return map[string]interface{}{"roomId": room.id, "rule": room.rule, "seed": room.seed, "phase": room.phase, "pending": p.pending.total(), "opponentBoard": opp.board, "elapsedMs": elapsed, "lines": p.lines, "opponentLines": opp.lines}
}
