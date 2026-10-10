package stacksrv

import "time"

// 이 파일의 표와 식은 웹 클라이언트(versus.ts·engine.ts)와 같은 값이어야 한다.
// 한쪽을 바꾸면 다른 쪽도 함께 바꾼다.

// Cols 는 판의 열 수다.
const Cols = 9

// Rows 는 판의 행 수다.
const Rows = 18

// BoardCells 는 판 칸 수(9×18=162)이자 board 문자열 길이다.
const BoardCells = Cols * Rows

// GarbageCell 은 방해 줄 칸 값이다(조각 칸 1~7 과 겹치지 않는다).
const GarbageCell = 8

// TicksPerSecond 는 엔진 고정 틱 수(초당)다.
const TicksPerSecond = 60

// LinesPerLevel 마다 레벨이 하나 오른다.
const LinesPerLevel = 8

// GarbagePerLockMax 는 한 번에 넣는 방해 줄 상한이다.
const GarbagePerLockMax = 8

// GarbagePendingMax 는 쌓여 대기할 수 있는 방해 줄 상한이다. 넘치는 공격은 버린다.
const GarbagePendingMax = 12

// RuleAttack 은 공격 모드다. 줄을 지워 상대에게 방해 줄을 보내고, 먼저 넘친 쪽이 진다.
const RuleAttack = "attack"

// RuleSprint40 은 40줄 스프린트다. 서버가 합산한 줄 수로 40줄을 먼저 채운 쪽이 이긴다.
const RuleSprint40 = "sprint40"

// SpeedUp 은 시간이 지나면 최소 레벨이 오르는 규칙이다.
type SpeedUp struct {
	// StartSec 부터 최소 레벨이 오른다.
	StartSec int `json:"startSec"`
	// EverySec 마다 최소 레벨이 1 오른다.
	EverySec int `json:"everySec"`
}

// RuleSpec 은 규칙 하나의 수치다. 웹 versus.ts 의 RuleDef 와 같은 모양으로 game_start 에 실린다.
type RuleSpec struct {
	// ID 는 규칙 이름이다.
	ID string `json:"id"`
	// Label 은 화면 표시 이름이다.
	Label string `json:"label"`
	// Garbage 는 줄을 지우면 상대에게 방해 줄을 보내는지다.
	Garbage bool `json:"garbage"`
	// GoalLines 는 먼저 채우면 이기는 줄 수다(없으면 null).
	GoalLines *int `json:"goalLines"`
	// TimeLimitSec 은 제한시간(초)이다.
	TimeLimitSec int `json:"timeLimitSec"`
	// SpeedUp 은 시간 경과 가속 규칙이다(없으면 null).
	SpeedUp *SpeedUp `json:"speedUp"`
}

// LookupRule 은 v1 에서 열린 규칙만 돌려준다. 그 외(score120·survival 등)는 거부한다.
func LookupRule(id string) (RuleSpec, bool) {
	switch id {
	case RuleAttack:
		return RuleSpec{ID: RuleAttack, Label: "공격 모드", Garbage: true, TimeLimitSec: 600, SpeedUp: &SpeedUp{StartSec: 120, EverySec: 30}}, true
	case RuleSprint40:
		goal := 40
		return RuleSpec{ID: RuleSprint40, Label: "40줄 스프린트", GoalLines: &goal, TimeLimitSec: 300}, true
	}
	return RuleSpec{}, false
}

// OpenRules 는 v1 에서 열린 규칙 목록이다(로비·전적 표시 순서).
func OpenRules() []string {
	return []string{RuleAttack, RuleSprint40}
}

// AttackFor 는 한 번에 지운 줄 수(0~4)에 대한 공격 줄 수다. 1→0, 2→1, 3→2, 4→4.
func AttackFor(cleared int) int {
	switch cleared {
	case 2:
		return 1
	case 3:
		return 2
	case 4:
		return 4
	}
	return 0
}

// CellBalanceOK 는 대전 칸 보존식이다. 굳힌 조각 4칸 + 넣은 방해 줄 8칸 − 지운 줄 9칸은
// 판 안의 칸 수이므로 0 이상 162 이하여야 한다. 지운 줄을 부풀리면 음수가 된다.
func CellBalanceOK(locks, garbageLines, cleared int) bool {
	return CellBalanceWithin(locks, garbageLines, cleared, 0)
}

// CellBalanceWithin 은 칸 보존식의 위쪽 한도에 slackLines×8 칸 여유를 준 판정이다.
// 서버는 방해 줄을 보낸 순간 넣은 것으로 세지만, 클라이언트가 그 줄을 받기 전에 다음 조각을 굳히면
// 그 판에는 아직 없다. 판이 거의 찬 상태에서 이 시차로 162 를 넘겨 cheat 로 오판하지 않게
// 방금 보낸 줄과 대기 줄만큼은 봐준다. 아래쪽 한도(지운 줄 부풀리기 방지)는 그대로다.
func CellBalanceWithin(locks, garbageLines, cleared, slackLines int) bool {
	cells := 4*locks + 8*garbageLines - 9*cleared
	return cells >= 0 && cells <= BoardCells+8*slackLines
}

// GarbageAckWindow 는 보낸 방해 줄을 아직 판에 반영하지 못했을 수 있다고 보는 시간이다.
const GarbageAckWindow = 2 * time.Second

// ValidBoard 는 board 가 162자 '0'~'8' 인지 본다.
func ValidBoard(b string) bool {
	if len(b) != BoardCells {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] < '0' || b[i] > '8' {
			return false
		}
	}
	return true
}

// garbageQueue 는 한 사람에게 쌓인 방해 줄 묶음이다(오래된 것부터).
type garbageQueue []garbageChunk

// total 은 대기 줄 합이다.
func (q garbageQueue) total() int {
	n := 0
	for _, c := range q {
		n += c.lines
	}
	return n
}

// cancel 은 상쇄다. 내가 보낼 공격 n 줄로 내게 쌓인 줄을 오래된 것부터 지우고,
// 남은 대기열과 상대에게 보낼 줄 수를 돌려준다.
func (q garbageQueue) cancel(n int) (garbageQueue, int) {
	out := make(garbageQueue, 0, len(q))
	for _, c := range q {
		if n > 0 {
			use := c.lines
			if use > n {
				use = n
			}
			c.lines -= use
			n -= use
		}
		if c.lines > 0 {
			out = append(out, c)
		}
	}
	return out, n
}

// push 는 공격 lines 줄을 구멍 열 hole 로 쌓는다. 대기 상한(12)을 넘는 만큼은 버린다.
func (q garbageQueue) push(lines, hole int) garbageQueue {
	room := GarbagePendingMax - q.total()
	if lines > room {
		lines = room
	}
	if lines <= 0 {
		return q
	}
	return append(q, garbageChunk{lines: lines, hole: hole})
}

// take 는 앞에서부터 최대 limit 줄을 꺼낸다. 묶음이 잘리면 남은 쪽이 같은 구멍 열을 유지한다.
func (q garbageQueue) take(limit int) ([]garbageChunk, garbageQueue) {
	var got []garbageChunk
	rest := make(garbageQueue, 0, len(q))
	for _, c := range q {
		if limit <= 0 {
			rest = append(rest, c)
			continue
		}
		use := c.lines
		if use > limit {
			use = limit
		}
		got = append(got, garbageChunk{lines: use, hole: c.hole})
		limit -= use
		if c.lines > use {
			rest = append(rest, garbageChunk{lines: c.lines - use, hole: c.hole})
		}
	}
	return got, rest
}

// goalReached 는 목표 줄 수 규칙에서 lines 가 목표에 닿았는지다.
func goalReached(spec RuleSpec, lines int) bool {
	return spec.GoalLines != nil && lines >= *spec.GoalLines
}

// judgeTimeLimit 은 제한시간이 다 됐을 때의 승자 자리(0·1)다. 무승부면 -1.
// attack 은 무승부, sprint40 은 줄을 더 많이 지운 쪽이 이긴다(같으면 무승부).
func judgeTimeLimit(rule string, lines [2]int) int {
	if rule != RuleSprint40 {
		return -1
	}
	switch {
	case lines[0] > lines[1]:
		return 0
	case lines[1] > lines[0]:
		return 1
	}
	return -1
}
