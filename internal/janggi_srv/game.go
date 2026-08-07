package janggisrv

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/damoang/angple-backend/internal/janggi"
)

// createGame 은 방에 장기 초기 국면을 깐다. 1=초(선공, 하단)·2=한.
// 오목의 createGame 과 같은 골격 — 판정은 전부 서버(engine)가 한다.
func (s *Server) createGame(roomID, choMbID, hanMbID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil {
		s.mu.Unlock()
		return
	}
	room.GameState = &GameState{
		Pieces: janggi.InitPieces(), CurrentTeam: janggi.TeamCho,
		ChoMbID: choMbID, HanMbID: hanMbID,
		MoveHistory: []Move{}, StartTime: time.Now(), LastMoveTime: time.Now(),
	}
	clients := s.clientsInRoomLocked(room)
	gs := room.GameState
	s.mu.Unlock()

	for _, c := range clients {
		s.sendToClient(c, map[string]interface{}{
			"type": "game_start", "roomId": roomID,
			"playerTeam":  room.PlayerColors[c.MbID], // 1=초 2=한 (matching 의 색 배정 재사용)
			"turnSeconds": int(TurnTimeout.Seconds()),
			"gameState": map[string]interface{}{
				"pieces": gs.Pieces, "currentTeam": gs.CurrentTeam,
			},
		})
	}
	s.armTurnTimer(roomID)
	log.Printf("[janggi] game start room=%s cho=%s han=%s db=%d", roomID, choMbID, hanMbID, room.DBGameID)
}

// armTurnTimer — 턴 90초. 시간이 다하면 그 사람이 패배한다(이탈=패배 원칙, 오목과 동일).
func (s *Server) armTurnTimer(roomID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.GameState == nil || room.GameState.finished {
		s.mu.Unlock()
		return
	}
	gs := room.GameState
	if gs.turnTimer != nil {
		gs.turnTimer.Stop()
	}
	turnOwner := gs.ChoMbID
	if gs.CurrentTeam == janggi.TeamHan {
		turnOwner = gs.HanMbID
	}
	gs.turnTimer = time.AfterFunc(TurnTimeout, func() {
		s.finishGame(roomID, opponentOf(room, turnOwner), "timeout",
			"시간 초과로 대국이 종료되었습니다.")
	})
	s.mu.Unlock()
}

// handleMove — 클라이언트는 from/to 좌표만 보낸다. 합법성 판정은 엔진 단독 권한.
func (s *Server) handleMove(client *Client, data map[string]interface{}) {
	roomID, _ := data["roomId"].(string) //nolint:errcheck // 없으면 방 조회 실패로 처리
	fx, ok1 := data["fromX"].(float64)
	fy, ok2 := data["fromY"].(float64)
	tx, ok3 := data["toX"].(float64)
	ty, ok4 := data["toY"].(float64)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return
	}
	from := janggi.Point{X: int(fx), Y: int(fy)}
	to := janggi.Point{X: int(tx), Y: int(ty)}

	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.GameState == nil || room.GameState.finished {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("게임을 찾을 수 없습니다."))
		return
	}
	gs := room.GameState
	team := room.PlayerColors[client.MbID]
	if team == 0 {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("이 대국의 참가자가 아닙니다."))
		return
	}
	if gs.CurrentTeam != team {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("차례가 아닙니다."))
		return
	}
	idx := -1
	for i := range gs.Pieces {
		p := gs.Pieces[i]
		if p.Alive && p.X == from.X && p.Y == from.Y && p.Team == team {
			idx = i
			break
		}
	}
	if idx < 0 || !janggi.IsLegal(gs.Pieces, team, idx, to) {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("둘 수 없는 수입니다."))
		return
	}

	gs.Pieces = janggi.ApplyMove(gs.Pieces, idx, to)
	gs.LastMoveTime = time.Now()
	gs.MoveHistory = append(gs.MoveHistory, Move{
		MbID: client.MbID, FromX: from.X, FromY: from.Y, ToX: to.X, ToY: to.Y, Time: time.Now(),
	})

	opp := janggi.TeamHan
	if team == janggi.TeamHan {
		opp = janggi.TeamCho
	}
	mate := janggi.IsCheckmate(gs.Pieces, opp)
	check := !mate && janggi.IsCheck(gs.Pieces, opp)
	// 한 수 쉼: 상대가 장군이 아닌데 둘 수가 없으면 자동 패스 — 턴이 되돌아온다.
	passed := false
	if !mate && !check && len(janggi.LegalMoves(gs.Pieces, opp)) == 0 {
		passed = true
		gs.passStreak++
	} else if !mate {
		gs.CurrentTeam = opp
		gs.passStreak = 0
	}
	// 빅장(궁 마주봄) — 이번 착수로 생기거나 유지되면 +1, 해소되면 0.
	// 2가 쌓이면(선언 후 상대가 한 수 안에 해소하지 않음) 점수 판정으로 넘어간다.
	bik := janggi.IsBikjang(gs.Pieces)
	if bik {
		gs.bikjangStreak++
	} else {
		gs.bikjangStreak = 0
	}
	settleByBikjang := bik && gs.bikjangStreak >= 2
	// 교착(패스 4회 누적)도 1차의 단순 무승부 대신 점수 판정으로 종료한다(2차 규칙).
	settleByStall := gs.passStreak >= 4
	pieces := gs.Pieces
	cur := gs.CurrentTeam
	clients := s.clientsInRoomLocked(room)
	s.mu.Unlock()

	for _, c := range clients {
		s.sendToClient(c, map[string]interface{}{
			"type": "move",
			"from": map[string]int{"x": from.X, "y": from.Y},
			"to":   map[string]int{"x": to.X, "y": to.Y},
			"team": team, "pieces": pieces, "currentTeam": cur,
			"check": check, "passed": passed, "bikjang": bik,
		})
	}

	switch {
	case mate:
		s.finishGame(roomID, client.MbID, "checkmate", "외통입니다.")
	case settleByBikjang:
		s.settleByScore(roomID, "bikjang", pieces)
	case settleByStall:
		s.settleByScore(roomID, "stall", pieces)
	default:
		s.armTurnTimer(roomID)
	}
}

// settleByScore — 점수제 판정(2차 규칙, 대한장기협회 점수: 차13 포7 마5 상3 사3 졸2).
// 한(후수)은 덤 1.5 를 더한다. 정수+1.5 라 동점이 존재하지 않으므로 항상 승자가 나온다.
func (s *Server) settleByScore(roomID, cause string, pieces []janggi.Piece) {
	s.mu.RLock()
	room := s.rooms[roomID]
	var choMbID, hanMbID string
	if room != nil && room.GameState != nil {
		choMbID, hanMbID = room.GameState.ChoMbID, room.GameState.HanMbID
	}
	s.mu.RUnlock()
	if room == nil {
		return
	}

	choScore := janggi.Score(pieces, janggi.TeamCho)
	hanScore := float64(janggi.Score(pieces, janggi.TeamHan)) + 1.5 // 덤
	winner := choMbID
	if hanScore > float64(choScore) {
		winner = hanMbID
	}
	causeLabel := "빅장"
	if cause == "stall" {
		causeLabel = "쌍방 교착"
	}
	note := fmt.Sprintf("%s — 점수 판정: 초 %d점 vs 한 %.1f점(덤 1.5 포함)", causeLabel, choScore, hanScore)
	s.finishGame(roomID, winner, cause+"_score", note)
}

func (s *Server) handleSurrender(client *Client, data map[string]interface{}) {
	roomID, _ := data["roomId"].(string) //nolint:errcheck // 위와 동일
	s.mu.RLock()
	room := s.rooms[roomID]
	s.mu.RUnlock()
	if room == nil || room.PlayerColors[client.MbID] == 0 {
		return
	}
	s.finishGame(roomID, opponentOf(room, client.MbID), "resign", "상대방이 기권했습니다.")
}

// finishGame — 오목과 동일 골격: 어떤 경로로 와도 한 번만, 결과·전적 영속화.
func (s *Server) finishGame(roomID, winnerMbID, reason, note string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.GameState == nil || room.GameState.finished {
		s.mu.Unlock()
		return
	}
	gs := room.GameState
	gs.finished = true
	if gs.turnTimer != nil {
		gs.turnTimer.Stop()
		gs.turnTimer = nil
	}
	cho, han := gs.ChoMbID, gs.HanMbID
	moves := append([]Move(nil), gs.MoveHistory...)
	dbID := room.DBGameID
	clients := s.clientsInRoomLocked(room)
	room.Status = "finished"
	s.mu.Unlock()

	if s.store != nil && dbID > 0 {
		if err := s.store.FinishGame(dbID, cho, han, winnerMbID, reason, moves); err != nil {
			log.Printf("[janggi] finish game persist failed room=%s db=%d: %v", roomID, dbID, err)
		}
	}

	winnerTeam := 0
	switch winnerMbID {
	case cho:
		winnerTeam = janggi.TeamCho
	case han:
		winnerTeam = janggi.TeamHan
	}
	for _, c := range clients {
		wins, losses, draws, rating := s.store.Stats(c.MbID)
		s.sendToClient(c, map[string]interface{}{
			"type": "game_over", "winner": winnerTeam, "winnerMbId": winnerMbID,
			"reason": reason, "message": note,
			"youWon": winnerMbID != "" && winnerMbID == c.MbID,
			"stats":  map[string]int{"wins": wins, "losses": losses, "draws": draws, "rating": rating},
		})
	}

	time.AfterFunc(10*time.Second, func() {
		s.mu.Lock()
		delete(s.rooms, roomID)
		s.mu.Unlock()
	})
	log.Printf("[janggi] game over room=%s winner=%s reason=%s", roomID, winnerMbID, reason)
}

func opponentOf(room *Room, mbID string) string {
	for _, p := range room.Players {
		if p != mbID {
			return p
		}
	}
	return ""
}

func errMsg(m string) map[string]interface{} {
	return map[string]interface{}{"type": "error", "message": m}
}

func marshalMoves(moves []Move) string {
	b, err := json.Marshal(moves)
	if err != nil {
		return "[]"
	}
	return string(b)
}
