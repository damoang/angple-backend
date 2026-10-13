package stacksrv

import "math"

// 뱀 게임 규칙 — 웹 lib/games/snake/engine.ts 를 그대로 옮겼다(결정적).
// 같은 seed·같은 입력 기록이면 웹과 같은 먹이 위치·같은 점수가 나온다.
// 두 쪽이 같은지는 공용 테스트 벡터(testdata/snake_vectors.json, 웹 저장소에 같은 파일)로 확인한다.
//
// 입력 기록 = [[틱, 방향], …]. [t, d] 는 t 번째 이동(0부터)을 하기 직전에 방향이 d 로 바뀐다는 뜻이다.
// 한 틱에 하나만, 틱은 엄격히 증가, 방향은 그 틱에 실제로 돌 수 있는 값(같은 방향·반대 방향 아님)이어야 한다.

// SnakeGrid 는 판 한 변의 칸 수다.
const SnakeGrid = 20

// SnakeMaxTicks 는 판 하나의 최대 틱이다(넘으면 판이 끝난다). 입력 기록 길이 상한도 같다.
const SnakeMaxTicks = 20000

// 틱 간격(ms) — max(60, 150 − floor(점수/3)×5).
const (
	snakeBaseSpeedMs = 150
	snakeMinSpeedMs  = 60
)

// 방향 — 시계 방향 순서라 (d+2)%4 가 반대 방향이다.
const (
	snakeUp = iota
	snakeRight
	snakeDown
	snakeLeft
)

// 끝난 이유
const (
	snakeEndWall  = "wall"
	snakeEndSelf  = "self"
	snakeEndFull  = "full"
	snakeEndLimit = "limit"
)

// 입력 기록 오류 코드(웹 replay 와 같다)
const (
	snakeErrShape    = "shape"
	snakeErrOrder    = "input_order"
	snakeErrDir      = "input_dir"
	snakeErrAfterEnd = "input_after_end"
)

var (
	snakeDX = [4]int{0, 1, 0, -1}
	snakeDY = [4]int{-1, 0, 1, 0}
)

// SnakePos 는 칸 좌표다.
type SnakePos struct {
	// X 는 가로(0~19)다.
	X int
	// Y 는 세로(0~19)다.
	Y int
}

// snakeGame 은 진행 중인 판이다.
type snakeGame struct {
	// body 는 머리가 0번인 몸이다.
	body []SnakePos
	// dir 는 지금 방향이다.
	dir int
	// food 는 먹이다(hasFood 가 false 면 없음).
	food SnakePos
	// hasFood 는 먹이가 있는지다.
	hasFood bool
	// score 는 점수다.
	score int
	// tick 은 지금까지 한 이동 수다.
	tick int
	// simMs 는 지금까지의 틱 간격 합(ms)이다.
	simMs int64
	// over 는 판이 끝났는지다.
	over bool
	// reason 은 끝난 이유다.
	reason string
	// rng 는 난수 상태다.
	rng uint32
}

// snakeSpeedFor 는 점수에 따른 틱 간격(ms)이다.
func snakeSpeedFor(score int) int64 {
	v := snakeBaseSpeedMs - (score/3)*5
	if v < snakeMinSpeedMs {
		return snakeMinSpeedMs
	}
	return int64(v)
}

// mulberry32 는 웹 엔진과 같은 32비트 난수기다. 다음 상태와 값을 돌려준다.
func mulberry32(state uint32) (next, value uint32) {
	next = state + 0x6d2b79f5
	t := next
	t = (t ^ (t >> 15)) * (t | 1)
	t ^= t + (t^(t>>7))*(t|61)
	return next, t ^ (t >> 14)
}

// placeFood 는 빈칸을 행 우선 순서로 세우고 floor(u×빈칸 수/2³²) 번째에 먹이를 놓는다. 빈칸이 없으면 false.
func (g *snakeGame) placeFood() bool {
	var occupied [SnakeGrid * SnakeGrid]bool
	for _, p := range g.body {
		occupied[p.Y*SnakeGrid+p.X] = true
	}
	free := make([]int, 0, SnakeGrid*SnakeGrid)
	for i, taken := range occupied {
		if !taken {
			free = append(free, i)
		}
	}
	if len(free) == 0 {
		g.hasFood = false
		return false
	}
	next, u := mulberry32(g.rng)
	g.rng = next
	cell := free[(uint64(u)*uint64(len(free)))>>32]
	g.food = SnakePos{X: cell % SnakeGrid, Y: cell / SnakeGrid}
	g.hasFood = true
	return true
}

// newSnakeGame 은 새 판이다. 시작 뱀은 (10,10)(9,10)(8,10), 오른쪽.
func newSnakeGame(seed uint32) *snakeGame {
	mid := SnakeGrid / 2
	g := &snakeGame{
		body: []SnakePos{{X: mid, Y: mid}, {X: mid - 1, Y: mid}, {X: mid - 2, Y: mid}},
		dir:  snakeRight,
		rng:  seed,
	}
	g.placeFood()
	return g
}

// canTurn 은 지금 d 로 돌 수 있는지다(같은 방향·반대 방향은 아니다).
func (g *snakeGame) canTurn(d int) bool {
	return d >= snakeUp && d <= snakeLeft && d != g.dir && d != (g.dir+2)%4
}

// hits 는 p 가 몸(꼬리 끝 칸 포함)에 닿는지다.
func (g *snakeGame) hits(p SnakePos) bool {
	for _, s := range g.body {
		if s == p {
			return true
		}
	}
	return false
}

// step 은 한 틱이다. want 로 돌 수 있으면 돌고 한 칸 간다. 반대 방향 want 는 무시한다.
func (g *snakeGame) step(want int) {
	if g.over {
		return
	}
	if g.canTurn(want) {
		g.dir = want
	}
	g.simMs += snakeSpeedFor(g.score)
	g.tick++
	head := SnakePos{X: g.body[0].X + snakeDX[g.dir], Y: g.body[0].Y + snakeDY[g.dir]}
	if head.X < 0 || head.X >= SnakeGrid || head.Y < 0 || head.Y >= SnakeGrid {
		g.end(snakeEndWall)
		return
	}
	if g.hits(head) {
		g.end(snakeEndSelf)
		return
	}
	g.body = append([]SnakePos{head}, g.body...)
	if g.hasFood && head == g.food {
		g.score++
		if !g.placeFood() {
			g.end(snakeEndFull)
			return
		}
	} else {
		g.body = g.body[:len(g.body)-1]
	}
	if g.tick >= SnakeMaxTicks {
		g.end(snakeEndLimit)
	}
}

// end 는 판을 끝낸다.
func (g *snakeGame) end(reason string) {
	g.over = true
	g.reason = reason
}

// SnakeReplay 는 재연 결과다.
type SnakeReplay struct {
	// Error 는 입력 기록 오류 코드다(통과면 빈 문자열).
	Error string
	// Score 는 점수다.
	Score int
	// Ticks 는 이동 수다.
	Ticks int
	// SimMs 는 이 판을 사람이 하는 데 걸리는 최소 시간(틱 간격 합, ms)이다.
	SimMs int64
	// Reason 은 끝난 이유다.
	Reason string
	// Foods 는 놓인 먹이 위치 순서다(처음 먹이 포함).
	Foods []SnakePos
}

// isIntIn 은 v 가 [lo, hi] 안의 정수인지다.
func isIntIn(v float64, lo, hi float64) bool {
	return v >= lo && v <= hi && v == math.Trunc(v)
}

// CheckSnakeInputs 는 입력 기록 모양(길이·정수·방향 값·틱 순서)을 본다. 통과면 빈 문자열.
func CheckSnakeInputs(inputs [][]float64) string {
	if inputs == nil || len(inputs) > SnakeMaxTicks {
		return snakeErrShape
	}
	last := -1.0
	for _, it := range inputs {
		if len(it) != 2 || !isIntIn(it[0], 0, SnakeMaxTicks-1) {
			return snakeErrShape
		}
		if !isIntIn(it[1], snakeUp, snakeLeft) {
			return snakeErrDir
		}
		if it[0] <= last {
			return snakeErrOrder
		}
		last = it[0]
	}
	return ""
}

// ReplaySnake 는 seed 와 입력 기록으로 판을 처음부터 끝까지 다시 돌린다(웹 replay 와 같다).
// 기록된 방향이 그 틱에 돌 수 없는 방향이면 input_dir, 판이 끝난 뒤의 입력이 남으면 input_after_end.
func ReplaySnake(seed uint32, inputs [][]float64) SnakeReplay {
	g := newSnakeGame(seed)
	foods := make([]SnakePos, 0, 8)
	if g.hasFood {
		foods = append(foods, g.food)
	}
	result := func(code string) SnakeReplay {
		return SnakeReplay{Error: code, Score: g.score, Ticks: g.tick, SimMs: g.simMs, Reason: g.reason, Foods: foods}
	}
	if code := CheckSnakeInputs(inputs); code != "" {
		return result(code)
	}
	i := 0
	for !g.over {
		want := g.dir
		if i < len(inputs) && int(inputs[i][0]) == g.tick {
			want = int(inputs[i][1])
			i++
			if !g.canTurn(want) {
				return result(snakeErrDir)
			}
		}
		before := g.score
		g.step(want)
		if g.score != before && g.hasFood {
			foods = append(foods, g.food)
		}
	}
	if i < len(inputs) {
		return result(snakeErrAfterEnd)
	}
	return result("")
}
