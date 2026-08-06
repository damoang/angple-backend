package omok

import (
	"encoding/json"
	"log"
	"time"
)

const boardSize = 15

func (s *Server) createGame(roomID, blackMbID, whiteMbID string) {
	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil {
		s.mu.Unlock()
		return
	}
	board := make([][]int, boardSize)
	for i := range board {
		board[i] = make([]int, boardSize)
	}
	room.GameState = &GameState{
		Board: board, CurrentPlayer: 1,
		BlackMbID: blackMbID, WhiteMbID: whiteMbID,
		MoveHistory: []Move{}, StartTime: time.Now(), LastMoveTime: time.Now(),
	}
	clients := s.clientsInRoomLocked(room)
	s.mu.Unlock()

	for _, c := range clients {
		s.sendToClient(c, map[string]interface{}{
			"type": "game_start", "roomId": roomID,
			"playerColor": colorOf(room, c.MbID),
			"turnSeconds": int(TurnTimeout.Seconds()),
			"gameState": map[string]interface{}{
				"board": board, "currentPlayer": 1,
			},
		})
	}
	s.armTurnTimer(roomID)
	log.Printf("[omok] game start room=%s black=%s white=%s db=%d", roomID, blackMbID, whiteMbID, room.DBGameID)
}

// armTurnTimer 는 현재 턴의 제한시간을 건다. 시간이 다하면 그 사람이 패배한다
// (이탈=패배 — 질 것 같을 때 창을 닫아 무효로 만드는 악용을 막는다).
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
	turnOwner := gs.BlackMbID
	if gs.CurrentPlayer == 2 {
		turnOwner = gs.WhiteMbID
	}
	gs.turnTimer = time.AfterFunc(TurnTimeout, func() {
		s.finishGame(roomID, opponentOf(room, turnOwner), "timeout",
			"시간 초과로 대국이 종료되었습니다.")
	})
	s.mu.Unlock()
}

func (s *Server) handleMove(client *Client, data map[string]interface{}) {
	roomID, _ := data["roomId"].(string)
	xf, okx := data["x"].(float64)
	yf, oky := data["y"].(float64)
	if !okx || !oky {
		return
	}
	x, y := int(xf), int(yf)

	s.mu.Lock()
	room := s.rooms[roomID]
	if room == nil || room.GameState == nil || room.GameState.finished {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("게임을 찾을 수 없습니다."))
		return
	}
	gs := room.GameState
	color := room.PlayerColors[client.MbID]
	if color == 0 {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("이 대국의 참가자가 아닙니다."))
		return
	}
	if gs.CurrentPlayer != color {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("차례가 아닙니다."))
		return
	}
	if x < 0 || x >= boardSize || y < 0 || y >= boardSize || gs.Board[y][x] != 0 {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("둘 수 없는 자리입니다."))
		return
	}

	gs.Board[y][x] = color
	gs.LastMoveTime = time.Now()
	gs.MoveHistory = append(gs.MoveHistory, Move{MbID: client.MbID, X: x, Y: y, Time: time.Now()})
	won := checkWin(gs.Board, x, y, color)
	full := isBoardFull(gs.Board)
	if !won {
		if gs.CurrentPlayer == 1 {
			gs.CurrentPlayer = 2
		} else {
			gs.CurrentPlayer = 1
		}
	}
	clients := s.clientsInRoomLocked(room)
	s.mu.Unlock()

	for _, c := range clients {
		s.sendToClient(c, map[string]interface{}{
			"type": "move", "x": x, "y": y, "player": color,
			"currentPlayer": gs.CurrentPlayer,
		})
	}

	switch {
	case won:
		s.finishGame(roomID, client.MbID, "five", "")
	case full:
		s.finishGame(roomID, "", "draw", "")
	default:
		s.armTurnTimer(roomID)
	}
}

func (s *Server) handleSurrender(client *Client, data map[string]interface{}) {
	roomID, _ := data["roomId"].(string)
	s.mu.RLock()
	room := s.rooms[roomID]
	s.mu.RUnlock()
	if room == nil || room.PlayerColors[client.MbID] == 0 {
		return
	}
	s.finishGame(roomID, opponentOf(room, client.MbID), "resign", "상대방이 기권했습니다.")
}

// finishGame 은 대국을 끝내고 결과·전적을 기록한다.
// winnerMbID 가 빈 문자열이면 무승부. 어떤 경로로 들어와도 한 번만 실행된다.
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
	black, white := gs.BlackMbID, gs.WhiteMbID
	moves := append([]Move(nil), gs.MoveHistory...)
	dbID := room.DBGameID
	clients := s.clientsInRoomLocked(room)
	room.Status = "finished"
	s.mu.Unlock()

	if s.store != nil && dbID > 0 {
		if err := s.store.FinishGame(dbID, black, white, winnerMbID, reason, moves); err != nil {
			log.Printf("[omok] finish game persist failed room=%s db=%d: %v", roomID, dbID, err)
		}
	}

	winnerColor := 0
	if winnerMbID == black {
		winnerColor = 1
	} else if winnerMbID == white {
		winnerColor = 2
	}
	for _, c := range clients {
		wins, losses, draws, rating := s.store.Stats(c.MbID)
		s.sendToClient(c, map[string]interface{}{
			"type": "game_over", "winner": winnerColor, "winnerMbId": winnerMbID,
			"reason": reason, "message": note,
			"youWon": winnerMbID != "" && winnerMbID == c.MbID,
			"stats":  map[string]int{"wins": wins, "losses": losses, "draws": draws, "rating": rating},
		})
	}

	// 방은 잠시 뒤 정리한다(클라이언트가 결과를 받을 시간).
	time.AfterFunc(10*time.Second, func() {
		s.mu.Lock()
		delete(s.rooms, roomID)
		s.mu.Unlock()
	})
	log.Printf("[omok] game over room=%s winner=%s reason=%s", roomID, winnerMbID, reason)
}

// ── 판정 로직 (서버 단독 권한 — 클라이언트는 좌표만 보낸다) ──

func checkWin(board [][]int, x, y, player int) bool {
	dirs := [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		count := 1 + countStones(board, x, y, d[0], d[1], player) +
			countStones(board, x, y, -d[0], -d[1], player)
		if count >= 5 {
			return true
		}
	}
	return false
}

func countStones(board [][]int, x, y, dx, dy, player int) int {
	count := 0
	nx, ny := x+dx, y+dy
	for nx >= 0 && nx < boardSize && ny >= 0 && ny < boardSize && board[ny][nx] == player {
		count++
		nx += dx
		ny += dy
	}
	return count
}

func isBoardFull(board [][]int) bool {
	for y := 0; y < boardSize; y++ {
		for x := 0; x < boardSize; x++ {
			if board[y][x] == 0 {
				return false
			}
		}
	}
	return true
}

func colorOf(room *Room, mbID string) int { return room.PlayerColors[mbID] }

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
