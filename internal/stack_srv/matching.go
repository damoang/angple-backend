package stacksrv

import (
	"encoding/json"
	"log"
	"strconv"
	"time"
)

// participant 는 매칭이 성립한 한 사람이다.
type participant struct {
	// mbID 는 회원 아이디다.
	mbID string
	// nick 은 닉네임이다.
	nick string
}

// matchPlan 은 방을 열 계획이다. 첫 매칭과 재대결이 같은 경로를 탄다.
type matchPlan struct {
	// mode 는 매칭 모드다.
	mode string
	// rule 은 대전 규칙이다.
	rule string
	// invite 는 초대 코드다.
	invite string
	// players 는 두 사람이다.
	players [2]participant
	// rematchOf 는 재대결이면 직전 판의 DB id 다.
	rematchOf int64
	// rematch 는 재대결인지다(실패 알림 형식이 다르다).
	rematch bool
}

// queueKey 는 (mode, rule) 대기열 키다.
func queueKey(mode, rule string) string {
	return mode + ":" + rule
}

// entryFeeFor 는 모드별 1인 참가비다. random 만 유료다.
func entryFeeFor(mode string) int {
	if mode == ModeRandom {
		return EntryFee
	}
	return 0
}

// sanitizeInviteCode 는 초대 코드를 영숫자 4~32자로 조인다. 그 외는 빈 문자열.
func sanitizeInviteCode(c string) string {
	if len(c) < 4 || len(c) > 32 {
		return ""
	}
	for _, r := range c {
		if !isAlnum(r) {
			return ""
		}
	}
	return c
}

// isAlnum 은 ASCII 영숫자인지다.
func isAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// matchingError 는 matching_status error 를 보낸다.
func (s *Server) matchingError(c *Client, rule, code, message string) {
	s.emit(c, msgMatchingStatus, map[string]interface{}{"status": statusError, "rule": rule, "code": code, "message": message})
}

// handleJoin 은 join_matching_queue {mode, rule, invite?} 다.
// 큐는 (mode, rule) 별로 나뉜다. 초대 대전은 코드와 규칙이 모두 같아야 만난다.
func (s *Server) handleJoin(c *Client, raw json.RawMessage) {
	var d struct {
		Mode   string `json:"mode"`
		Rule   string `json:"rule"`
		Invite string `json:"invite"`
	}
	if err := decodeData(raw, &d); err != nil {
		s.matchingError(c, "", "bad_message", "요청 형식이 올바르지 않습니다.")
		return
	}
	if d.Mode != ModeRandom && d.Mode != ModeFavorite {
		s.matchingError(c, d.Rule, "invalid_mode", "알 수 없는 매칭 방식입니다.")
		return
	}
	if _, ok := LookupRule(d.Rule); !ok {
		s.matchingError(c, d.Rule, "invalid_rule", "아직 열리지 않은 대전 방식입니다.")
		return
	}
	invite := ""
	if d.Mode == ModeFavorite {
		invite = sanitizeInviteCode(d.Invite)
		if invite == "" {
			s.matchingError(c, d.Rule, "invalid_invite", "초대 링크가 올바르지 않습니다. 링크를 다시 확인해 주세요.")
			return
		}
	}
	// 유료 모드는 잔액을 미리 본다(확정 차감은 매칭 시점의 FOR UPDATE 가 한다).
	if fee := entryFeeFor(d.Mode); fee > 0 && s.store != nil {
		if bal := s.store.Balance(c.MbID); bal < fee {
			s.matchingError(c, d.Rule, "insufficient_point", "참가비 1,000P가 부족합니다. (보유 "+strconv.Itoa(bal)+"P)")
			return
		}
	}

	s.mu.Lock()
	code, msg := s.joinBlockedLocked(c, d.Mode, d.Rule, invite)
	if code != "" {
		s.mu.Unlock()
		s.matchingError(c, d.Rule, code, msg)
		return
	}
	key := queueKey(d.Mode, d.Rule)
	s.queues[key] = append(s.queues[key], &queueEntry{client: c, rule: d.Rule, inviteCode: invite, joinedAt: s.now()})
	c.queueKey = key
	position := len(s.queues[key])
	pair := s.popMatchLocked(key)
	if pair != nil {
		s.starting[pair[0].client.MbID] = true
		s.starting[pair[1].client.MbID] = true
	}
	s.mu.Unlock()

	s.emit(c, msgMatchingStatus, map[string]interface{}{"status": "waiting", "rule": d.Rule, "mode": d.Mode, "position": position, "entryFee": entryFeeFor(d.Mode)})
	if pair != nil {
		plan := matchPlan{mode: d.Mode, rule: d.Rule, invite: invite}
		plan.players[0] = participant{mbID: pair[0].client.MbID, nick: pair[0].client.Nick}
		plan.players[1] = participant{mbID: pair[1].client.MbID, nick: pair[1].client.Nick}
		s.startMatch(plan)
	}
}

// joinBlockedLocked 는 줄을 설 수 없는 이유(code, message)다. 설 수 있으면 빈 문자열.
func (s *Server) joinBlockedLocked(c *Client, mode, rule, invite string) (string, string) {
	if room, _ := s.roomOfLocked(c.MbID); room != nil {
		if !room.finished {
			return "already_in_game", "이미 진행 중인 대전이 있습니다."
		}
		// 재대결 대기 중에 새로 줄 서면 재대결은 거절한 것으로 본다.
		s.cancelRematchLocked(room, "left")
	}
	if s.starting[c.MbID] {
		return "already_in_game", "대전을 준비하고 있습니다."
	}
	if c.queueKey != "" {
		return "already_queued", "이미 매칭 대기 중입니다."
	}
	if len(s.rooms) >= MaxRooms {
		return codeServerBusy, "지금은 대전이 많아 잠시 후 다시 시도해 주세요."
	}
	if mode == ModeFavorite {
		// 같은 초대 코드로 다른 규칙을 기다리는 사람이 있으면 알려 준다(서로 영영 못 만난다).
		for key, q := range s.queues {
			if key == queueKey(ModeFavorite, rule) {
				continue
			}
			for _, e := range q {
				if e.inviteCode == invite {
					return "rule_mismatch", "초대한 사람과 대전 방식이 다릅니다. 같은 방식을 골라 주세요."
				}
			}
		}
	}
	return "", ""
}

// popMatchLocked 는 매칭 가능한 두 명을 큐에서 빼서 돌려준다(없으면 nil). 가장 오래 기다린 사람부터 본다.
func (s *Server) popMatchLocked(key string) []*queueEntry {
	q := s.queues[key]
	for i := 0; i < len(q); i++ {
		for j := i + 1; j < len(q); j++ {
			if q[i].inviteCode != q[j].inviteCode || q[i].client.MbID == q[j].client.MbID {
				continue
			}
			a, b := q[i], q[j]
			rest := make([]*queueEntry, 0, len(q))
			for k, e := range q {
				if k != i && k != j {
					rest = append(rest, e)
				}
			}
			s.queues[key] = rest
			a.client.queueKey = ""
			b.client.queueKey = ""
			return []*queueEntry{a, b}
		}
	}
	return nil
}

// removeFromQueuesLocked 는 접속 하나를 모든 대기열에서 뺀다.
func (s *Server) removeFromQueuesLocked(c *Client) {
	for key, q := range s.queues {
		rest := make([]*queueEntry, 0, len(q))
		for _, e := range q {
			if e.client != c {
				rest = append(rest, e)
			}
		}
		s.queues[key] = rest
	}
	c.queueKey = ""
}

// handleCancelMatching 은 대기열에서 빠진다.
func (s *Server) handleCancelMatching(c *Client) {
	s.mu.Lock()
	s.removeFromQueuesLocked(c)
	s.mu.Unlock()
}

// startMatch 는 대전 행을 만들고 참가비를 걷은 뒤 방을 연다(ready 단계).
// DB 트랜잭션이 들어가므로 락 밖에서 부른다.
// 한쪽 차감이 실패하면 이미 낸 쪽은 즉시 환불하고 판을 취소한다.
func (s *Server) startMatch(plan matchPlan) {
	defer func() {
		s.mu.Lock()
		delete(s.starting, plan.players[0].mbID)
		delete(s.starting, plan.players[1].mbID)
		s.mu.Unlock()
	}()
	s.mu.Lock()
	busy := len(s.rooms) >= MaxRooms
	s.mu.Unlock()
	if busy {
		s.notifyStartFailed(plan, "", codeServerBusy, false)
		return
	}
	fee := entryFeeFor(plan.mode)
	seed := randomUint32()
	var gameID int64
	if s.store != nil {
		id, err := s.store.CreateGame(NewGame{Rule: plan.rule, Mode: plan.mode, P1: plan.players[0].mbID, P2: plan.players[1].mbID, EntryFee: fee, Seed: seed, RematchOf: plan.rematchOf})
		if err != nil {
			log.Printf("[stack] create game failed: %v", err)
			s.notifyStartFailed(plan, "", "start_failed", false)
			return
		}
		gameID = id
		if fee > 0 {
			if failed, ok := s.chargeBoth(gameID, plan, fee); !ok {
				s.notifyStartFailed(plan, failed, "insufficient_point", failed == plan.players[1].mbID)
				return
			}
		}
	}
	s.openRoom(plan, gameID, fee, seed)
}

// chargeBoth 는 두 사람의 참가비를 차례로 걷는다. 실패하면 낸 쪽을 환불하고 판을 취소한 뒤
// 실패한 회원 아이디와 false 를 돌려준다.
func (s *Server) chargeBoth(gameID int64, plan matchPlan, fee int) (string, bool) {
	charged := make([]string, 0, 2)
	for _, p := range plan.players {
		if err := s.store.ChargeEntryFee(gameID, p.mbID, fee); err != nil {
			log.Printf("[stack] entry fee failed game=%d seat=%d: %v", gameID, len(charged), err)
			for _, mb := range charged {
				if rerr := s.store.RefundEntryFee(gameID, mb, fee); rerr != nil {
					log.Printf("[stack] refund failed game=%d: %v", gameID, rerr)
				}
			}
			if aerr := s.store.AbortGame(gameID, "entry_fee_failed"); aerr != nil {
				log.Printf("[stack] abort game failed id=%d: %v", gameID, aerr)
			}
			return p.mbID, false
		}
		charged = append(charged, p.mbID)
	}
	return "", true
}

// notifyStartFailed 는 판을 열지 못했음을 두 사람에게 알린다. failedMbID 는 참가비가 부족했던 쪽이다.
func (s *Server) notifyStartFailed(plan matchPlan, failedMbID, code string, refundNotice bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range plan.players {
		c := s.clients[p.mbID]
		myCode, message := code, "대전을 시작하지 못했습니다. 잠시 후 다시 시도해 주세요."
		switch {
		case code == codeServerBusy:
			message = "지금은 대전이 많아 잠시 후 다시 시도해 주세요."
		case failedMbID != "" && p.mbID == failedMbID:
			message = "참가비 1,000P가 부족해 대전이 취소되었습니다."
		case failedMbID != "":
			myCode = codeOpponentPaymentFailed
			message = "상대방의 참가비 결제가 되지 않아 대전이 취소되었습니다."
			if refundNotice {
				message += " 낸 참가비는 돌려드렸습니다."
			}
		}
		if plan.rematch {
			s.emit(c, msgRematchCanceled, map[string]interface{}{"reason": myCode, "message": message})
			continue
		}
		s.matchingError(c, plan.rule, myCode, message)
	}
}

// openRoom 은 방을 만들고 matched 를 보낸 뒤 ready 타이머를 건다.
func (s *Server) openRoom(plan matchPlan, gameID int64, fee int, seed uint32) {
	var ratings [2]int
	for i, p := range plan.players {
		ratings[i] = DefaultRating
		if s.store != nil {
			if st, ok := s.store.Stats(p.mbID, plan.rule); ok {
				ratings[i] = st.Rating
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room := &Room{id: generateID(12), rule: plan.rule, mode: plan.mode, inviteCode: plan.invite, fee: fee, dbGameID: gameID, seed: seed, phase: phaseReady}
	for i, p := range plan.players {
		room.players[i] = &playerState{mbID: p.mbID, nick: p.nick, ratingBefore: ratings[i]}
		s.playerRoom[p.mbID] = room.id
	}
	s.rooms[room.id] = room
	roomID := room.id
	room.readyTimer = time.AfterFunc(s.timing.Ready, func() { s.onReadyTimeout(roomID) })
	for i, p := range plan.players {
		opp := plan.players[1-i]
		s.emitToLocked(p.mbID, msgMatchingStatus, map[string]interface{}{"status": "matched", "roomId": roomID, "rule": plan.rule, "mode": plan.mode, "opponent": map[string]interface{}{"nickname": opp.nick, "rating": ratings[1-i]}, "entryFeeCharged": fee, "readyMs": s.timing.Ready.Milliseconds()})
	}
	log.Printf("[stack] room open room=%s rule=%s mode=%s db=%d", roomID, plan.rule, plan.mode, gameID)
}
