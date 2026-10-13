package janggisrv

import (
	"log"
	"time"
)

func (s *Server) handleMessage(client *Client, msg Message) {
	switch msg.Type {
	case "join_matching_queue":
		s.handleMatching(client, msg.Data)
	case "cancel_matching":
		s.handleCancelMatching(client)
	case "setup_select":
		s.handleSetupSelect(client, msg.Data)
	case "move":
		s.handleMove(client, msg.Data)
	case "surrender":
		s.handleSurrender(client, msg.Data)
	case "get_player_stats":
		s.sendToClient(client, map[string]interface{}{
			"type": "player_stats", "stats": s.playerInfo(client),
		})
	case "reconnect":
		s.handleReconnect(client, msg.Data)
	case "pong", "ping":
		client.isAlive = true
	default:
		s.sendToClient(client, errMsg("알 수 없는 요청입니다: "+msg.Type))
	}
}

func (s *Server) createSessionForPlayer(client *Client) *Session {
	sessionID := generateSessionID()
	session := &Session{ID: sessionID, MbID: client.MbID, Client: client}
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()
	client.sessionID = sessionID
	return session
}

// handleDisconnect 은 끊긴 연결을 유예 상태로 옮긴다.
// 유예(30초) 안에 돌아오지 않으면 그 대국은 **패배**로 확정된다.
func (s *Server) handleDisconnect(client *Client) {
	s.mu.Lock()
	delete(s.clients, client)
	// 대기열에서도 뺀다 — 끊긴 사람이 큐에 남아 상대를 헛매칭시키면 안 된다.
	for mode, q := range s.matchingQueue {
		rest := make([]*queueEntry, 0, len(q))
		for _, e := range q {
			if e.client != client {
				rest = append(rest, e)
			}
		}
		s.matchingQueue[mode] = rest
	}
	session, ok := s.sessions[client.sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	roomID := client.roomID
	var opponents []*Client
	if room := s.rooms[roomID]; roomInPlayLocked(room) {
		session.GameID = roomID
		session.GameState = room.GameState
		for _, pid := range room.Players {
			if pid != client.MbID {
				if c := s.findClientByMbIDLocked(pid); c != nil {
					opponents = append(opponents, c)
				}
			}
		}
	}
	now := time.Now()
	session.DisconnectedAt = &now
	session.Client = nil
	s.disconnectedSessions[session.ID] = session
	delete(s.sessions, session.ID)
	mbID := session.MbID
	s.mu.Unlock()

	for _, c := range opponents {
		s.sendToClient(c, map[string]interface{}{
			"type": "opponent_disconnected", "timeout": int(ReconnectGrace.Seconds()),
			"message": "상대방의 연결이 끊겼습니다. 30초 안에 돌아오지 않으면 회원님의 승리로 처리됩니다.",
		})
	}
	log.Printf("[janggi] disconnected mb_id=%s room=%s", mbID, roomID)

	session.ReconnectTimeout = time.AfterFunc(ReconnectGrace, func() {
		s.mu.Lock()
		delete(s.disconnectedSessions, session.ID)
		gameID := session.GameID
		room := s.rooms[gameID]
		s.mu.Unlock()
		if gameID == "" || room == nil {
			return
		}
		// 이탈 = 패배 (남은 쪽 승리)
		s.finishGame(gameID, opponentOf(room, session.MbID), "disconnect",
			"상대방이 대국을 이탈했습니다.")
	})
}

func (s *Server) handleReconnect(client *Client, data map[string]interface{}) {
	sessionID, _ := data["sessionId"].(string) //nolint:errcheck // 없으면 빈 문자열 → 아래에서 조기 반환
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	session, ok := s.disconnectedSessions[sessionID]
	// ⛔ 세션 소유자 확인 — 남의 세션 id 로 남의 대국에 들어올 수 없다.
	if !ok || session.MbID != client.MbID {
		s.mu.Unlock()
		s.sendToClient(client, errMsg("복구할 대국이 없습니다."))
		return
	}
	if session.ReconnectTimeout != nil {
		session.ReconnectTimeout.Stop()
		session.ReconnectTimeout = nil
	}
	session.Client = client
	client.sessionID = sessionID
	client.roomID = session.GameID
	s.sessions[sessionID] = session
	delete(s.disconnectedSessions, sessionID)

	room := s.rooms[session.GameID]
	restore := restoreMsgsLocked(room, client.MbID)
	var opponents []*Client
	if restore != nil {
		for _, pid := range room.Players {
			if pid != client.MbID {
				if c := s.findClientByMbIDLocked(pid); c != nil {
					opponents = append(opponents, c)
				}
			}
		}
	}
	s.mu.Unlock()

	for _, c := range opponents {
		s.sendToClient(c, map[string]interface{}{
			"type": "opponent_reconnected", "message": "상대방이 다시 접속했습니다.",
		})
	}
	if restore == nil {
		s.sendToClient(client, errMsg("복구할 대국이 없습니다."))
		return
	}
	for _, m := range restore {
		s.sendToClient(client, m)
	}
}

// restoreMsgsLocked 는 재접속한 mbID 에게 보낼 복원 메시지다(복원할 게 없으면 nil).
//   - 상차림 선택 단계: setup_choose(남은 시간·본인 선택값) + setup_status
//   - 대국 중: game_restored(gameState 에 setups 포함)
//
// 호출자가 s.mu 를 잡고 있어야 한다.
func restoreMsgsLocked(room *Room, mbID string) []map[string]interface{} {
	if room == nil {
		return nil
	}
	if room.setupPhase {
		return []map[string]interface{}{
			setupChooseMsgLocked(room, mbID, time.Now()),
			setupStatusMsgLocked(room),
		}
	}
	gs := room.GameState
	if gs == nil || gs.finished {
		return nil
	}
	return []map[string]interface{}{{
		"type": "game_restored", "roomId": room.ID,
		"gameState": map[string]interface{}{
			"pieces": gs.Pieces, "currentTeam": gs.CurrentTeam,
			"playerTeam": room.PlayerColors[mbID],
			"setups":     setupsPayload(gs),
		},
	}}
}

// roomInPlayLocked 는 방이 상차림 선택 단계이거나 대국 중인지 본다.
// 선택 단계도 대국 중으로 본다 — 끊겨도 단계는 기본값으로 그대로 시작하고,
// 유예 안에 돌아오지 않으면 기존 이탈 규칙(패배)을 따른다.
// 호출자가 s.mu 를 잡고 있어야 한다.
func roomInPlayLocked(room *Room) bool {
	if room == nil {
		return false
	}
	return room.setupPhase || room.GameState != nil && !room.GameState.finished
}

// findClientByMbIDLocked — 호출자가 s.mu 를 잡고 있어야 한다.
func (s *Server) findClientByMbIDLocked(mbID string) *Client {
	for c := range s.clients {
		if c.MbID == mbID {
			return c
		}
	}
	return nil
}

func (s *Server) clientsInRoomLocked(room *Room) []*Client {
	out := make([]*Client, 0, len(room.Players))
	for _, pid := range room.Players {
		if c := s.findClientByMbIDLocked(pid); c != nil {
			out = append(out, c)
		}
	}
	return out
}
