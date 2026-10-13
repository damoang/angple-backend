package stacksrv

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// snakeVector 는 공용 테스트 벡터 한 판이다. 웹 저장소 lib/games/snake/testdata/vectors.json 과 같은 파일이다.
type snakeVector struct {
	// Name 은 벡터 이름이다.
	Name string `json:"name"`
	// Seed 는 시드다.
	Seed uint32 `json:"seed"`
	// Inputs 는 입력 기록이다.
	Inputs [][]float64 `json:"inputs"`
	// Expect 는 웹 엔진이 낸 기대값이다.
	Expect struct {
		Error  string  `json:"error"`
		Score  int     `json:"score"`
		Ticks  int     `json:"ticks"`
		SimMs  int64   `json:"simMs"`
		Reason string  `json:"reason"`
		Foods  [][]int `json:"foods"`
	} `json:"expect"`
}

func loadSnakeVectors(t *testing.T) []snakeVector {
	t.Helper()
	raw, err := os.ReadFile("testdata/snake_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Version int           `json:"version"`
		Vectors []snakeVector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if file.Version != 1 || len(file.Vectors) < 10 {
		t.Fatalf("벡터 파일이 이상하다: version=%d n=%d", file.Version, len(file.Vectors))
	}
	return file.Vectors
}

// 웹 TS 엔진이 낸 기대값(점수·틱·최소 시간·끝난 이유·먹이 위치 순서)과 Go 재연이 똑같아야 한다.
func TestSnakeReplayMatchesWebVectors(t *testing.T) {
	scored, selfEnd := false, false
	for _, v := range loadSnakeVectors(t) {
		got := ReplaySnake(v.Seed, v.Inputs)
		foods := make([][]int, 0, len(got.Foods))
		for _, p := range got.Foods {
			foods = append(foods, []int{p.X, p.Y})
		}
		e := v.Expect
		if got.Error != e.Error || got.Score != e.Score || got.Ticks != e.Ticks || got.SimMs != e.SimMs || got.Reason != e.Reason {
			t.Fatalf("%s: got err=%q score=%d ticks=%d simMs=%d reason=%q, want %+v",
				v.Name, got.Error, got.Score, got.Ticks, got.SimMs, got.Reason, e)
		}
		if !reflect.DeepEqual(foods, e.Foods) {
			t.Fatalf("%s: 먹이 위치가 다르다\n got  %v\n want %v", v.Name, foods, e.Foods)
		}
		scored = scored || e.Score >= 20
		selfEnd = selfEnd || e.Reason == snakeEndSelf
	}
	if !scored || !selfEnd {
		t.Fatal("벡터에 점수가 나는 판·몸에 부딪혀 끝나는 판이 있어야 한다")
	}
}

func TestSnakeSpeedMatchesOriginal(t *testing.T) {
	cases := map[int]int64{0: 150, 2: 150, 3: 145, 53: 65, 54: 60, 300: 60}
	for score, want := range cases {
		if got := snakeSpeedFor(score); got != want {
			t.Fatalf("speed(%d) = %d, want %d", score, got, want)
		}
	}
}

func TestSnakeStepRules(t *testing.T) {
	g := newSnakeGame(1)
	g.step(snakeLeft) // 반대 방향은 무시
	if g.dir != snakeRight || g.body[0] != (SnakePos{X: 11, Y: 10}) {
		t.Fatalf("반대 방향 = dir %d head %v", g.dir, g.body[0])
	}

	// 꼬리 끝 칸에 닿아도 끝난다(기존 화면 규칙).
	g = newSnakeGame(1)
	g.food = SnakePos{X: 0, Y: 0}
	g.body = []SnakePos{{X: 5, Y: 5}, {X: 5, Y: 6}, {X: 6, Y: 6}, {X: 6, Y: 5}}
	g.dir = snakeUp
	g.step(snakeRight)
	if !g.over || g.reason != snakeEndSelf {
		t.Fatalf("꼬리 = over %v reason %q", g.over, g.reason)
	}

	// 최대 틱.
	g = newSnakeGame(1)
	g.food = SnakePos{X: 0, Y: 0}
	g.tick = SnakeMaxTicks - 1
	g.step(snakeDown)
	if !g.over || g.reason != snakeEndLimit {
		t.Fatalf("최대 틱 = over %v reason %q", g.over, g.reason)
	}

	// 빈칸이 없으면 끝(full).
	g = newSnakeGame(1)
	body := make([]SnakePos, 0, SnakeGrid*SnakeGrid)
	body = append(body, SnakePos{X: 1, Y: 0})
	for y := 0; y < SnakeGrid; y++ {
		for x := 0; x < SnakeGrid; x++ {
			if (x == 0 || x == 1) && y == 0 {
				continue
			}
			body = append(body, SnakePos{X: x, Y: y})
		}
	}
	g.body = body
	g.dir = snakeLeft
	g.food = SnakePos{X: 0, Y: 0}
	g.step(snakeLeft)
	if !g.over || g.reason != snakeEndFull || g.score != 1 || g.hasFood {
		t.Fatalf("꽉 참 = over %v reason %q score %d food %v", g.over, g.reason, g.score, g.hasFood)
	}
}

func TestCheckSnakeInputs(t *testing.T) {
	cases := []struct {
		in   [][]float64
		want string
	}{
		{nil, snakeErrShape},
		{[][]float64{}, ""},
		{[][]float64{{0, 0}, {5, 1}}, ""},
		{[][]float64{{1}}, snakeErrShape},
		{[][]float64{{1, 2, 3}}, snakeErrShape},
		{[][]float64{{SnakeMaxTicks, 0}}, snakeErrShape},
		{[][]float64{{0, -1}}, snakeErrDir},
		{[][]float64{{0, 1.5}}, snakeErrDir},
		{[][]float64{{3, 0}, {3, 1}}, snakeErrOrder},
		{[][]float64{{3, 0}, {2, 1}}, snakeErrOrder},
		{make([][]float64, SnakeMaxTicks+1), snakeErrShape},
	}
	for i, c := range cases {
		if got := CheckSnakeInputs(c.in); got != c.want {
			t.Fatalf("#%d CheckSnakeInputs = %q, want %q", i, got, c.want)
		}
	}
}

func TestMulberry32IsUint32Deterministic(t *testing.T) {
	a, b := uint32(0xffffffff), uint32(0xffffffff)
	for i := 0; i < 100; i++ {
		var va, vb uint32
		a, va = mulberry32(a)
		b, vb = mulberry32(b)
		if a != b || va != vb {
			t.Fatal("같은 상태에서 다른 값이 나왔다")
		}
	}
}
