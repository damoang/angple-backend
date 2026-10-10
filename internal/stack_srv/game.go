package stacksrv

import (
	"encoding/json"
	"log"
	"time"
)

// lockData 는 lock {seq, tick, cleared, board, score, lines} 다. 조각이 굳을 때만 온다.
type lockData struct {
	// Seq 는 1부터 하나씩 오르는 번호다.
	Seq int64 `json:"seq"`
	// Tick 은 굳은 순간의 엔진 틱이다.
	Tick int64 `json:"tick"`
	// Cleared 는 이번에 지운 줄 수(0~4)다.
	Cleared int `json:"cleared"`
	// Board 는 줄을 지운 뒤, 방해 줄을 넣기 전의 판이다(162자).
	Board string `json:"board"`
	// Score 는 클라이언트 점수다(표시용).
	Score int64 `json:"score"`
	// Lines 는 클라이언트 줄 합이다(표시용 — 판정은 서버 합산).
	Lines int `json:"lines"`
}

// lockOutcome 은 lock 처리 결과다.
type lockOutcome struct {
	// finish 는 이 lock 으로 판이 끝나는지다.
	finish bool
	// winner 는 끝날 때 이긴 자리다.
	winner int
	// reason 은 끝난 사유다.
	reason string
}

// onReadyTimeout 은 ready 시간(10초) 안에 둘 다 준비하지 않은 방을 취소하고 참가비를 돌려준다.
func (s *Server) onReadyTimeout(roomID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.finished || room.phase != phaseReady {
		s.mu.Unlock()
		return
	}
	room.finished = true
	room.phase = phaseOver
	s.closeRoomLocked(room)
	players := room.players
	s.mu.Unlock()

	refunded := false
	if s.store != nil && room.dbGameID > 0 {
		if room.fee > 0 {
			for _, p := range players {
				if err := s.store.RefundEntryFee(room.dbGameID, p.mbID, room.fee); err != nil {
					log.Printf("[stack] refund failed game=%d: %v", room.dbGameID, err)
				}
			}
			refunded = true
		}
		if err := s.store.AbortGame(room.dbGameID, "ready_timeout"); err != nil {
			log.Printf("[stack] abort game failed id=%d: %v", room.dbGameID, err)
		}
	}
	message := "상대방이 준비하지 않아 대전이 취소되었습니다."
	if refunded {
		message += " 낸 참가비는 돌려드렸습니다."
	}
	s.mu.Lock()
	for _, p := range players {
		s.emitToLocked(p.mbID, msgMatchingStatus, map[string]interface{}{"status": "error", "rule": room.rule, "code": "ready_timeout", "message": message, "refunded": refunded})
	}
	s.mu.Unlock()
	log.Printf("[stack] ready timeout room=%s", roomID)
}

// handleReady 는 ready {roomId} 다. 둘 다 준비되면 game_start(카운트다운 3초) 후 go 를 보낸다.
func (s *Server) handleReady(c *Client, raw json.RawMessage) {
	var d struct {
		RoomID string `json:"roomId"`
	}
	if err := decodeData(raw, &d); err != nil {
		s.sendBadMessage(c)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, idx := s.roomOfLocked(c.MbID)
	if room == nil || room.id != d.RoomID || room.phase != phaseReady || room.finished {
		s.sendError(c, "no_game", "준비할 대전이 없습니다.")
		return
	}
	room.players[idx].ready = true
	if !room.players[1-idx].ready {
		return
	}
	if room.readyTimer != nil {
		room.readyTimer.Stop()
		room.readyTimer = nil
	}
	room.phase = phaseCountdown
	spec, _ := LookupRule(room.rule)
	for _, p := range room.players {
		s.emitToLocked(p.mbID, "game_start", map[string]interface{}{"roomId": room.id, "rule": room.rule, "ruleSpec": spec, "seed": room.seed, "countdownMs": s.timing.Countdown.Milliseconds(), "entryFeeCharged": room.fee})
	}
	roomID := room.id
	room.countdownTimer = time.AfterFunc(s.timing.Countdown, func() { s.goRoom(roomID) })
}

// goRoom 은 카운트다운이 끝난 방을 시작한다. 이때부터 제한시간·무입력·틱 검사의 기준 시각이 된다.
func (s *Server) goRoom(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[roomID]
	if room == nil || room.finished || room.phase != phaseCountdown {
		return
	}
	room.phase = phasePlaying
	room.goAt = s.now()
	spec, _ := LookupRule(room.rule)
	limit := time.Duration(float64(spec.TimeLimitSec) * s.timing.TimeLimitScale * float64(time.Second))
	room.capTimer = time.AfterFunc(limit, func() { s.onTimeLimit(roomID) })
	for i, p := range room.players {
		if p.disconnectedAt.IsZero() {
			s.armInactivityLocked(room, i)
		}
		s.emitToLocked(p.mbID, "go", map[string]interface{}{"roomId": roomID, "timeLimitMs": limit.Milliseconds()})
	}
}

// armInactivityLocked 는 무입력(20초) 패배 타이머를 (다시) 건다.
func (s *Server) armInactivityLocked(room *Room, idx int) {
	p := room.players[idx]
	if p.afkTimer != nil {
		p.afkTimer.Stop()
	}
	p.afkGen++
	roomID, gen := room.id, p.afkGen
	p.afkTimer = time.AfterFunc(s.timing.Inactivity, func() { s.onInactivity(roomID, idx, gen) })
}

// onInactivity 는 무입력 시간이 다 된 사람을 패배 처리한다. 다시 걸린 뒤의 옛 타이머는 무시한다.
func (s *Server) onInactivity(roomID string, idx, gen int) {
	s.mu.Lock()
	room := s.rooms[roomID]
	active := room != nil && !room.finished && room.phase == phasePlaying && room.players[idx].afkGen == gen
	s.mu.Unlock()
	if active {
		s.finishGame(roomID, 1-idx, "inactivity")
	}
}

// onTimeLimit 은 제한시간 판정이다. attack 은 무승부, sprint40 은 줄이 많은 쪽 승(같으면 무승부).
func (s *Server) onTimeLimit(roomID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.finished || room.phase != phasePlaying {
		s.mu.Unlock()
		return
	}
	winner := judgeTimeLimit(room.rule, [2]int{room.players[0].lines, room.players[1].lines})
	s.mu.Unlock()
	reason := "time"
	if winner < 0 {
		reason = "draw"
	}
	s.finishGame(roomID, winner, reason)
}

// handleLock 은 lock 을 검사하고 반영한다.
// 검사: seq 단조 증가 · lock 빈도 · 판 형식(162자 '0'~'8') · cleared 0~4 · 틱 단조·실경과 이내 · 칸 보존식.
// 형식·틱·칸 보존 위반은 받아들이지 않고 위반 횟수를 센다. 3회면 cheat 패배다.
func (s *Server) handleLock(c *Client, raw json.RawMessage) {
	var d lockData
	if err := decodeData(raw, &d); err != nil {
		s.sendBadMessage(c)
		return
	}
	s.mu.Lock()
	room, idx := s.roomOfLocked(c.MbID)
	if room == nil || room.finished || room.phase != phasePlaying {
		s.mu.Unlock()
		s.sendError(c, "not_playing", "진행 중인 대전이 없습니다.")
		return
	}
	out, code := s.applyLockLocked(room, idx, d)
	roomID := room.id
	s.mu.Unlock()
	if code != "" {
		s.sendError(c, code, "받아들일 수 없는 입력입니다.")
	}
	if out.finish {
		s.finishGame(roomID, out.winner, out.reason)
	}
}

// applyLockLocked 는 lock 하나를 반영한다. 거부하면 오류 코드를 돌려준다.
func (s *Server) applyLockLocked(room *Room, idx int, d lockData) (lockOutcome, string) {
	p := room.players[idx]
	now := s.now()
	if d.Seq <= p.seq {
		return lockOutcome{}, "bad_seq"
	}
	if !p.allowLock(now, s.timing.LocksPerSecond) {
		return lockOutcome{}, "rate_limited"
	}
	p.seq = d.Seq
	// 굳힌 입력이 왔다 — 손을 놓은 것이 아니다.
	s.armInactivityLocked(room, idx)

	maxTick := now.Sub(room.goAt).Milliseconds()*TicksPerSecond/1000 + TickSlack
	switch {
	case !ValidBoard(d.Board) || d.Cleared < 0 || d.Cleared > 4 || d.Score < 0:
		return s.violationLocked(room, idx, "bad_lock")
	case d.Tick < p.tick || d.Tick > maxTick:
		return s.violationLocked(room, idx, "bad_tick")
	}
	p.tick = d.Tick
	p.locks++
	if !CellBalanceOK(p.locks, p.garbageIn, p.lines+d.Cleared) {
		// 조각은 굳은 것으로 세되 지운 줄·공격은 인정하지 않는다.
		return s.violationLocked(room, idx, "cell_balance")
	}
	p.lines += d.Cleared
	p.board = d.Board
	p.score = d.Score

	spec, _ := LookupRule(room.rule)
	if spec.Garbage {
		s.applyGarbageLocked(room, idx, d.Cleared)
	}
	s.queueOpponentStateLocked(room, 1-idx)
	if spec.GoalLines != nil {
		s.sendProgressLocked(room)
		if goalReached(spec, p.lines) {
			return lockOutcome{finish: true, winner: idx, reason: "goal"}, ""
		}
	}
	return lockOutcome{}, ""
}

// allowLock 은 최근 1초 lock 수가 상한 안인지 보고, 허용하면 기록한다.
func (p *playerState) allowLock(now time.Time, perSecond int) bool {
	if perSecond <= 0 {
		return true
	}
	recent := p.lockTimes[:0]
	for _, t := range p.lockTimes {
		if now.Sub(t) < time.Second {
			recent = append(recent, t)
		}
	}
	p.lockTimes = recent
	if len(recent) >= perSecond {
		return false
	}
	p.lockTimes = append(p.lockTimes, now)
	return true
}

// violationLocked 는 위반을 세고, 3회째면 cheat 패배를 돌려준다.
func (s *Server) violationLocked(room *Room, idx int, code string) (lockOutcome, string) {
	p := room.players[idx]
	p.violations++
	log.Printf("[stack] violation room=%s seat=%d code=%s count=%d", room.id, idx, code, p.violations)
	if p.violations >= MaxViolations {
		return lockOutcome{finish: true, winner: 1 - idx, reason: "cheat"}, code
	}
	return lockOutcome{}, code
}

// applyGarbageLocked 는 attack 규칙의 방해 줄을 처리한다.
// 줄을 지우면 공격표(1→0, 2→1, 3→2, 4→4)만큼 먼저 내 대기 줄을 상쇄하고 남은 만큼 상대에게 쌓는다(대기 ≤ 12).
// 줄을 못 지운 lock 뒤에는 내 대기 줄을 최대 8줄 넣으라고 보낸다(묶음마다 구멍 열이 같다).
func (s *Server) applyGarbageLocked(room *Room, idx, cleared int) {
	p, opp := room.players[idx], room.players[1-idx]
	if cleared > 0 {
		before := p.pending.total()
		rest, send := p.pending.cancel(AttackFor(cleared))
		p.pending = rest
		if p.pending.total() != before {
			s.emitToLocked(p.mbID, "garbage_queued", map[string]interface{}{"pending": p.pending.total()})
		}
		if send > 0 {
			opp.pending = opp.pending.push(send, randomHole())
			s.emitToLocked(opp.mbID, "garbage_queued", map[string]interface{}{"pending": opp.pending.total()})
		}
		return
	}
	if len(p.pending) == 0 {
		return
	}
	chunks, rest := p.pending.take(GarbagePerLockMax)
	p.pending = rest
	// 묶음마다 garbage_apply 한 통. 받은 순서대로 넣는다. pending 은 그 묶음을 넣고 남은 대기 줄 수다.
	after := p.pending.total()
	for _, ch := range chunks {
		after += ch.lines
	}
	for _, ch := range chunks {
		after -= ch.lines
		p.garbageIn += ch.lines
		s.emitToLocked(p.mbID, "garbage_apply", map[string]interface{}{"lines": ch.lines, "hole": ch.hole, "pending": after})
	}
}

// queueOpponentStateLocked 는 seat 에게 상대 판을 보낸다. 초당 4회를 넘지 않게, 너무 잦으면
// 한 번만 예약해 그때의 최신 판을 보낸다.
func (s *Server) queueOpponentStateLocked(room *Room, seat int) {
	to := room.players[seat]
	wait := to.oppStateLast.Add(s.timing.OpponentStateEvery).Sub(s.now())
	if wait <= 0 {
		s.sendOpponentStateLocked(room, seat)
		return
	}
	if to.oppStateTimer != nil {
		return
	}
	roomID := room.id
	to.oppStateTimer = time.AfterFunc(wait, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		r := s.rooms[roomID]
		if r == nil || r.finished {
			return
		}
		r.players[seat].oppStateTimer = nil
		s.sendOpponentStateLocked(r, seat)
	})
}

// sendOpponentStateLocked 는 opponent_state {board, lines, score, pending} 를 보낸다.
func (s *Server) sendOpponentStateLocked(room *Room, seat int) {
	to, from := room.players[seat], room.players[1-seat]
	to.oppStateLast = s.now()
	s.emitToLocked(to.mbID, "opponent_state", map[string]interface{}{"board": from.board, "lines": from.lines, "score": from.score, "pending": from.pending.total()})
}

// sendProgressLocked 는 sprint40 진행률 progress {you, opp, elapsedMs} 를 두 사람에게 보낸다.
func (s *Server) sendProgressLocked(room *Room) {
	elapsed := s.now().Sub(room.goAt).Milliseconds()
	for i, p := range room.players {
		opp := room.players[1-i]
		s.emitToLocked(p.mbID, "progress", map[string]interface{}{"you": map[string]int{"lines": p.lines}, "opp": map[string]int{"lines": opp.lines}, "elapsedMs": elapsed})
	}
}

// handleToppedOut 은 topped_out {seq, tick} 다. 판이 넘친 쪽이 진다.
func (s *Server) handleToppedOut(c *Client, raw json.RawMessage) {
	var d struct {
		Seq  int64 `json:"seq"`
		Tick int64 `json:"tick"`
	}
	if err := decodeData(raw, &d); err != nil {
		s.sendBadMessage(c)
		return
	}
	s.mu.Lock()
	room, idx := s.roomOfLocked(c.MbID)
	ok := room != nil && !room.finished && room.phase == phasePlaying
	s.mu.Unlock()
	if ok {
		s.finishGame(room.id, 1-idx, "topout")
	}
}

// handleSurrender 는 기권이다(참가비 환불 없음). ready 단계 기권도 기권으로 본다.
func (s *Server) handleSurrender(c *Client) {
	s.mu.Lock()
	room, idx := s.roomOfLocked(c.MbID)
	ok := room != nil && !room.finished
	s.mu.Unlock()
	if ok {
		s.finishGame(room.id, 1-idx, "resign")
	}
}

// finishGame 은 어떤 경로(목표·넘침·시간·기권·무입력·끊김·cheat)로 와도 한 번만 돈다.
// winner 는 이긴 자리(0·1), 무승부는 -1. 결과·전적을 남기고 game_over 를 보낸 뒤 재대결을 30초 받는다.
func (s *Server) finishGame(roomID string, winner int, reason string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.finished {
		s.mu.Unlock()
		return
	}
	room.finished = true
	room.phase = phaseOver
	stopRoomTimersLocked(room)
	var elapsedMs int64
	if !room.goAt.IsZero() {
		elapsedMs = s.now().Sub(room.goAt).Milliseconds()
	}
	res := GameResult{GameID: room.dbGameID, Rule: room.rule, P1: room.players[0].mbID, P2: room.players[1].mbID, Reason: reason, P1Lines: room.players[0].lines, P2Lines: room.players[1].lines, DurationMs: elapsedMs}
	if winner >= 0 {
		res.Winner = room.players[winner].mbID
	}
	s.mu.Unlock()

	var stats [2]PlayerStats
	if s.store != nil {
		if room.dbGameID > 0 {
			if err := s.store.FinishGame(res); err != nil {
				log.Printf("[stack] finish persist failed room=%s db=%d: %v", roomID, room.dbGameID, err)
			}
		}
		for i, mb := range []string{res.P1, res.P2} {
			st, ok := s.store.Stats(mb, room.rule)
			if !ok {
				st = PlayerStats{Rating: DefaultRating}
			}
			stats[i] = st
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	winnerNick := interface{}(nil)
	if winner >= 0 {
		winnerNick = room.players[winner].nick
	}
	notice := rematchNotice(room.fee)
	for i, p := range room.players {
		result := map[string]interface{}{"lines": p.lines, "score": p.score}
		if reason == "goal" && i == winner {
			result["timeMs"] = elapsedMs
		}
		s.emitToLocked(p.mbID, "game_over", map[string]interface{}{"winner": winnerNick, "youWon": i == winner, "reason": reason, "result": result, "stats": stats[i], "ratingDelta": stats[i].Rating - p.ratingBefore, "rematchSeconds": int(s.timing.Rematch.Seconds()), "rematchFee": room.fee, "feeNotice": notice})
	}
	if s.rooms[roomID] == room {
		room.rematchTimer = time.AfterFunc(s.timing.Rematch, func() { s.onRematchExpired(roomID) })
	}
	log.Printf("[stack] game over room=%s winnerSeat=%d reason=%s", roomID, winner, reason)
}

// stopRoomTimersLocked 는 방의 진행 타이머를 모두 멈춘다.
func stopRoomTimersLocked(room *Room) {
	for _, t := range []*time.Timer{room.readyTimer, room.countdownTimer, room.capTimer} {
		if t != nil {
			t.Stop()
		}
	}
	room.readyTimer, room.countdownTimer, room.capTimer = nil, nil, nil
	for _, p := range room.players {
		for _, t := range []*time.Timer{p.afkTimer, p.graceTimer, p.oppStateTimer} {
			if t != nil {
				t.Stop()
			}
		}
		p.afkTimer, p.graceTimer, p.oppStateTimer = nil, nil, nil
	}
}

// rematchNotice 는 재대결 참가비 사전 안내 문구다.
func rematchNotice(fee int) string {
	if fee <= 0 {
		return "초대 대전 재대결은 무료입니다."
	}
	return "재대결도 판마다 참가비 1,000P가 차감됩니다."
}

// handleRematch 는 재대결 신청이자 수락이다. 30초 안에 둘 다 누르면 새 판을 연다(random 은 다시 1,000P).
func (s *Server) handleRematch(c *Client) {
	s.mu.Lock()
	room, idx := s.roomOfLocked(c.MbID)
	if room == nil || !room.finished || room.rematchStarting || room.phase != phaseOver {
		s.mu.Unlock()
		s.emit(c, msgRematchCanceled, map[string]interface{}{"reason": "unavailable"})
		return
	}
	room.players[idx].rematch = true
	opp := room.players[1-idx]
	if !opp.rematch {
		s.emitToLocked(opp.mbID, "rematch_offer", map[string]interface{}{"feeNotice": rematchNotice(room.fee), "fee": room.fee})
		s.mu.Unlock()
		return
	}
	room.rematchStarting = true
	s.closeRoomLocked(room)
	plan := matchPlan{mode: room.mode, rule: room.rule, invite: room.inviteCode, rematchOf: room.dbGameID, rematch: true}
	for i, p := range room.players {
		plan.players[i] = participant{mbID: p.mbID, nick: p.nick}
		s.starting[p.mbID] = true
	}
	s.mu.Unlock()
	s.startMatch(plan)
}

// handleRematchDecline 은 재대결 거절이다.
func (s *Server) handleRematchDecline(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if room, _ := s.roomOfLocked(c.MbID); room != nil && room.finished {
		s.cancelRematchLocked(room, "declined")
	}
}

// onRematchExpired 는 재대결 대기(30초)가 끝난 방을 닫는다.
func (s *Server) onRematchExpired(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if room := s.rooms[roomID]; room != nil && room.finished && !room.rematchStarting {
		s.cancelRematchLocked(room, "expired")
	}
}

// cancelRematchLocked 는 재대결 대기 중인 방을 닫고 남은 사람에게 알린다.
func (s *Server) cancelRematchLocked(room *Room, reason string) {
	if room.rematchStarting {
		return
	}
	s.closeRoomLocked(room)
	for _, p := range room.players {
		s.emitToLocked(p.mbID, msgRematchCanceled, map[string]interface{}{"reason": reason})
	}
}

// closeRoomLocked 는 방을 허브에서 지운다.
func (s *Server) closeRoomLocked(room *Room) {
	if room.rematchTimer != nil {
		room.rematchTimer.Stop()
		room.rematchTimer = nil
	}
	if s.rooms[room.id] == room {
		delete(s.rooms, room.id)
	}
	for _, p := range room.players {
		if s.playerRoom[p.mbID] == room.id {
			delete(s.playerRoom, p.mbID)
		}
	}
}
