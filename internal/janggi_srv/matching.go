package janggisrv

import (
	"log"
	"time"
)

// handleMatching 은 대기열에 넣고 매칭을 시도한다.
//
// 원본과의 차이:
//   - 큐가 슬라이스라 **선착순(FIFO)** 이 지켜진다. 맵 순회는 순서가 무작위여서
//     오래 기다린 사람이 계속 밀렸다.
//   - rating 모드가 실제로 레이팅을 본다(원본은 이름만 달랐다).
//   - 매칭 성립(=참가비 차감)은 락 밖에서 한다 — DB 트랜잭션을 허브 락 안에서
//     돌리면 전체 서버가 그 시간만큼 멈춘다.
func (s *Server) handleMatching(client *Client, data map[string]interface{}) {
	mode := ModeRandom
	if m, ok := data["mode"].(string); ok && (m == ModeRandom || m == ModeRating || m == ModeFavorite) {
		mode = m
	}

	// 초대 대국은 같은 코드끼리만 만난다. 코드 없는 favorite 진입은 거부 —
	// 열어 두면 "친구와 두기" 링크로 모르는 둘이 붙는 사고가 난다(무료 큐라
	// 무한정 공짜 대국 통로가 되기도 한다). 코드는 클라이언트가 만들어 URL 로
	// 전달하므로 서버는 형식만 조인다(영숫자 4~32자).
	inviteCode := ""
	if mode == ModeFavorite {
		if c, ok := data["invite"].(string); ok {
			inviteCode = sanitizeInviteCode(c)
		}
		if inviteCode == "" {
			s.sendToClient(client, map[string]interface{}{
				"type": "matching_status", "status": "error",
				"message": "초대 링크가 올바르지 않습니다. 링크를 다시 확인해 주세요.",
				"code":    "invalid_invite",
			})
			return
		}
	}

	// 유료 모드는 잔액을 미리 본다 — 큐에서 오래 기다린 뒤 "잔액 부족"으로
	// 튕기면 상대까지 헛걸음한다. (확정 차감은 매칭 시점의 FOR UPDATE 가 한다)
	if mode != ModeFavorite && s.store != nil {
		if bal := s.balanceOf(client.MbID); bal < EntryFee {
			s.sendToClient(client, map[string]interface{}{
				"type": "matching_status", "status": "error",
				"message": "참가비 1,000P가 부족합니다. (보유 " + itoa(bal) + "P)",
				"code":    "insufficient_point",
			})
			return
		}
	}

	s.mu.Lock()
	for _, q := range s.matchingQueue {
		for _, e := range q {
			if e.client.MbID == client.MbID {
				s.mu.Unlock()
				s.sendToClient(client, map[string]interface{}{
					"type": "matching_status", "status": "error", "message": "이미 매칭 대기 중입니다.",
				})
				return
			}
		}
	}
	rating := DefaultRating
	if s.store != nil {
		rating = s.store.Rating(client.MbID)
	}
	entry := &queueEntry{client: client, rating: rating, joinedAt: time.Now(), inviteCode: inviteCode}
	s.matchingQueue[mode] = append(s.matchingQueue[mode], entry)
	client.matchingMode = mode
	position := len(s.matchingQueue[mode]) // 슬라이스라 이 값이 실제 순번이다
	pair := s.popMatchLocked(mode)
	s.mu.Unlock()

	s.sendToClient(client, map[string]interface{}{
		"type": "matching_status", "status": "queued", "queue": mode,
		"position": position, "entryFee": entryFeeFor(mode),
	})

	if pair != nil {
		s.startMatch(pair[0], pair[1], mode)
	}
}

// popMatchLocked 는 매칭 가능한 두 명을 큐에서 빼서 돌려준다(없으면 nil).
// 호출자가 s.mu 를 잡고 있어야 한다.
func (s *Server) popMatchLocked(mode string) []*queueEntry {
	q := s.matchingQueue[mode]
	if len(q) < 2 {
		return nil
	}
	// 가장 오래 기다린 사람(head)부터 상대를 찾는다 = FIFO
	for i := 0; i < len(q); i++ {
		for j := i + 1; j < len(q); j++ {
			if !s.compatible(mode, q[i], q[j]) {
				continue
			}
			a, b := q[i], q[j]
			rest := make([]*queueEntry, 0, len(q)-2)
			for k, e := range q {
				if k != i && k != j {
					rest = append(rest, e)
				}
			}
			s.matchingQueue[mode] = rest
			a.client.matchingMode = ""
			b.client.matchingMode = ""
			return []*queueEntry{a, b}
		}
	}
	return nil
}

// sanitizeInviteCode 는 초대 코드를 영숫자 4~32자로 조인다. 그 외는 빈 문자열.
func sanitizeInviteCode(c string) string {
	if len(c) < 4 || len(c) > 32 {
		return ""
	}
	for _, r := range c {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return c
}

// compatible 은 레이팅 밴드를 본다. 기다린 시간이 길수록 밴드를 넓혀
// "실력이 비슷한 상대"와 "언젠가는 매칭됨"을 함께 만족시킨다.
func (s *Server) compatible(mode string, a, b *queueEntry) bool {
	// 초대 대국은 같은 코드끼리만.
	if mode == ModeFavorite {
		return a.inviteCode != "" && a.inviteCode == b.inviteCode
	}
	if mode != ModeRating {
		return true
	}
	waited := time.Since(a.joinedAt)
	if waited > time.Since(b.joinedAt) {
		waited = time.Since(b.joinedAt)
	}
	band := ratingBandInit
	switch {
	case waited >= 2*ratingBandWiden:
		return true // 60초 넘게 기다렸으면 아무나
	case waited >= ratingBandWiden:
		band = ratingBandSecond
	}
	diff := a.rating - b.rating
	if diff < 0 {
		diff = -diff
	}
	return diff <= band
}

// startMatch 는 방을 만들고 참가비를 걷은 뒤 대국을 시작한다.
// ⛔ 락 밖에서 호출된다(DB 트랜잭션 포함).
func (s *Server) startMatch(a, b *queueEntry, mode string) {
	paid := mode != ModeFavorite
	var gameID int64
	blackFirst := time.Now().UnixNano()%2 == 0
	black, white := a.client, b.client
	if !blackFirst {
		black, white = b.client, a.client
	}

	if s.store != nil {
		id, err := s.store.CreateGame(black.MbID, white.MbID, entryFeeFor(mode))
		if err != nil {
			log.Printf("[janggi] create game failed: %v", err)
			s.notifyMatchFailed(a.client, b.client, "대국을 시작하지 못했습니다. 잠시 후 다시 시도해 주세요.")
			return
		}
		gameID = id

		if paid {
			charged := make([]*Client, 0, 2)
			failed := ""
			for _, c := range []*Client{black, white} {
				if err := s.store.ChargeEntryFee(gameID, c.MbID, EntryFee); err != nil {
					failed = c.MbID
					log.Printf("[janggi] entry fee failed game=%d mb=%s: %v", gameID, c.MbID, err)
					break
				}
				charged = append(charged, c)
			}
			if failed != "" {
				// 이미 낸 쪽은 즉시 환불하고 대국을 취소한다.
				for _, c := range charged {
					if err := s.store.RefundEntryFee(gameID, c.MbID, EntryFee); err != nil {
						log.Printf("[janggi] ⛔ refund failed game=%d mb=%s: %v", gameID, c.MbID, err)
					}
				}
				if aerr := s.store.AbortGame(gameID, "entry_fee_failed"); aerr != nil {
					log.Printf("[janggi] abort game failed id=%d: %v", gameID, aerr)
				}
				for _, c := range []*Client{black, white} {
					msg := "상대방의 참가비 결제가 되지 않아 대국이 취소되었습니다. 낸 참가비는 돌려드렸습니다."
					if c.MbID == failed {
						msg = "참가비 1,000P가 부족해 대국이 취소되었습니다."
					}
					s.sendToClient(c, map[string]interface{}{
						"type": "matching_status", "status": "error", "message": msg,
					})
				}
				return
			}
		}
	}

	roomID := generateID()
	s.mu.Lock()
	room := &Room{
		ID: roomID, Players: []string{black.MbID, white.MbID},
		Status: "playing", Created: time.Now(),
		PlayerColors: map[string]int{black.MbID: 1, white.MbID: 2},
		DBGameID:     gameID, Paid: paid,
	}
	s.rooms[roomID] = room
	black.roomID, white.roomID = roomID, roomID
	s.mu.Unlock()

	for _, c := range []*Client{black, white} {
		opponent := white
		if c == white {
			opponent = black
		}
		s.sendToClient(c, map[string]interface{}{
			"type": "matching_status", "status": "matched",
			"roomId": roomID, "opponent": s.playerInfo(opponent),
			"entryFeeCharged": paid,
		})
	}

	s.createGame(roomID, black.MbID, white.MbID)
}

func (s *Server) notifyMatchFailed(a, b *Client, msg string) {
	for _, c := range []*Client{a, b} {
		s.sendToClient(c, map[string]interface{}{
			"type": "matching_status", "status": "error", "message": msg,
		})
	}
}

func (s *Server) handleCancelMatching(client *Client) {
	s.mu.Lock()
	mode := client.matchingMode
	if mode != "" {
		q := s.matchingQueue[mode]
		rest := make([]*queueEntry, 0, len(q))
		for _, e := range q {
			if e.client.MbID != client.MbID {
				rest = append(rest, e)
			}
		}
		s.matchingQueue[mode] = rest
		client.matchingMode = ""
	}
	s.mu.Unlock()
	s.sendToClient(client, map[string]interface{}{"type": "matching_canceled"})
}

func (s *Server) playerInfo(c *Client) map[string]interface{} {
	if c == nil {
		return map[string]interface{}{}
	}
	wins, losses, draws, rating := s.store.Stats(c.MbID)
	return map[string]interface{}{
		"mbId": c.MbID, "nickname": c.Nick,
		"rating": rating, "wins": wins, "losses": losses, "draws": draws,
	}
}

func (s *Server) balanceOf(mbID string) int {
	if s.store == nil || s.store.db == nil {
		return EntryFee
	}
	var p int
	_ = s.store.db.Raw("SELECT mb_point FROM g5_member WHERE mb_id = ?", mbID).Scan(&p).Error
	return p
}

func entryFeeFor(mode string) int {
	if mode == ModeFavorite {
		return 0
	}
	return EntryFee
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
