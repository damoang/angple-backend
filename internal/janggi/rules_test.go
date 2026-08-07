package janggi

import (
	"sort"
	"testing"
)

// ⛔ 이 테스트의 기대값은 Go 구현이 아니라 **클라이언트 TS(janggi-game.svelte)의
// 규칙 의미론에서 손으로 유도**했다. 각 케이스 주석에 유도 근거를 남긴다 —
// 구현을 보고 기대값을 베끼면 대조가 아니라 동어반복이 된다.

func sorted(ps []Point) []Point {
	out := append([]Point{}, ps...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

func movesOf(t *testing.T, pcs []Piece, x, y int) []Point {
	t.Helper()
	b := MakeBoard(pcs)
	for i := range pcs {
		if pcs[i].Alive && pcs[i].X == x && pcs[i].Y == y {
			return sorted(Moves(pcs[i], b))
		}
	}
	t.Fatalf("(%d,%d)에 말이 없다", x, y)
	return nil
}

func eq(t *testing.T, got, want []Point, label string) {
	t.Helper()
	w := sorted(want)
	if len(got) != len(w) {
		t.Fatalf("%s: 수 개수 %d != 기대 %d — got=%v want=%v", label, len(got), len(w), got, w)
	}
	for i := range w {
		if got[i] != w[i] {
			t.Fatalf("%s: got=%v want=%v", label, got, w)
		}
	}
}

func TestInitialPosition(t *testing.T) {
	pcs := InitPieces()
	if len(pcs) != 32 {
		t.Fatalf("초기 말 수 %d != 32", len(pcs))
	}
	// 궁 위치: 한(4,1)·초(4,8)
	kings := 0
	for _, p := range pcs {
		if p.Kind == KindGung {
			kings++
			if p.Team == TeamHan && (p.X != 4 || p.Y != 1) {
				t.Fatalf("한 궁 위치 (%d,%d)", p.X, p.Y)
			}
			if p.Team == TeamCho && (p.X != 4 || p.Y != 8) {
				t.Fatalf("초 궁 위치 (%d,%d)", p.X, p.Y)
			}
		}
	}
	if kings != 2 {
		t.Fatalf("궁 %d개", kings)
	}
}

func TestChaInitial(t *testing.T) {
	// 초 차(0,9): 위로 (0,8)(0,7) — (0,6)은 아군 졸이라 정지.
	// 오른쪽 (1,9)는 아군 상. 아래·왼쪽은 판 밖. → 정확히 2수.
	eq(t, movesOf(t, InitPieces(), 0, 9),
		[]Point{{0, 8}, {0, 7}}, "초 차(0,9)")
}

func TestMaInitial(t *testing.T) {
	// 초 마(2,9). 멱 규칙 유도:
	//   위 멱(2,8) 비어 있음 → (1,7)=아군 포 제외, (3,7) 빈 칸 허용
	//   좌 멱(1,9)=아군 상 막힘, 우 멱(3,9)=아군 사 막힘, 아래는 판 밖
	// → 정확히 (3,7) 한 수.
	eq(t, movesOf(t, InitPieces(), 2, 9),
		[]Point{{3, 7}}, "초 마(2,9)")
}

func TestSaPalaceDiag(t *testing.T) {
	// 초 사(3,9)는 궁성 대각선 위(5-3 == 9-7). 이동 후보 중 궁성 안만:
	//   직선: (3,8)빈칸, (4,9)빈칸 / (2,9)는 궁성 밖, (3,10) 판 밖
	//   대각: (4,8)=중심 빈칸... 초기 배치에선 (4,8)에 초 궁! → 아군이라 제외
	// → (3,8),(4,9) 두 수.
	eq(t, movesOf(t, InitPieces(), 3, 9),
		[]Point{{3, 8}, {4, 9}}, "초 사(3,9)")
}

func TestPoInitialHasNoMoves(t *testing.T) {
	// 장기 상식: 초기 국면의 포는 둘 곳이 없다.
	// 초 포(1,7): 위쪽 첫 기물이 (1,2)의 상대 포 → 포는 포를 못 넘어 정지.
	// 아래 (1,8)빈칸→(1,9) 아군 상을 넘지만 그 다음이 판 밖. 좌/우도 화면 없음·포.
	got := movesOf(t, InitPieces(), 1, 7)
	if len(got) != 0 {
		t.Fatalf("초기 포는 0수여야 한다: %v", got)
	}
}

func TestJolInitial(t *testing.T) {
	// 초 졸(0,6): 전진(0,5), 우(1,6). 좌는 판 밖, 후퇴 없음.
	eq(t, movesOf(t, InitPieces(), 0, 6),
		[]Point{{0, 5}, {1, 6}}, "초 졸(0,6)")
}

// 맞춤 국면 도우미 — 자살수 필터를 안 쓰는 Moves 검증엔 궁이 없어도 된다.
func pos(pieces ...Piece) []Piece { return pieces }
func pc(kind, team, x, y int) Piece {
	return Piece{Kind: kind, Team: team, X: x, Y: y, Alive: true}
}

func TestPoCannotUsePoAsScreenOrCapturePo(t *testing.T) {
	// 초 포(4,5), 상대 포(4,3)이 화면 후보, 그 뒤 (4,1) 상대 차.
	// 포는 포를 못 넘으므로 위 방향 0수. (좌우·아래는 화면 없음 → 0수)
	pcs := pos(pc(KindPo, TeamCho, 4, 5), pc(KindPo, TeamHan, 4, 3), pc(KindCha, TeamHan, 4, 1))
	got := movesOf(t, pcs, 4, 5)
	if len(got) != 0 {
		t.Fatalf("포가 포를 넘었다: %v", got)
	}
	// 화면을 일반 기물로 바꾸면: (4,4)빈칸(화면 전이라 착점 아님) → 화면(4,3)=졸 →
	// 넘은 뒤 (4,2)빈칸 착점, (4,1) 상대 차 잡기. 단 (4,0)은 차에 막혀 불가.
	pcs2 := pos(pc(KindPo, TeamCho, 4, 5), pc(KindJol, TeamHan, 4, 3), pc(KindCha, TeamHan, 4, 1))
	eq(t, movesOf(t, pcs2, 4, 5), []Point{{4, 2}, {4, 1}}, "포 정상 넘기")
	// 넘은 뒤 첫 기물이 포면 잡지 못하고 정지.
	pcs3 := pos(pc(KindPo, TeamCho, 4, 5), pc(KindJol, TeamHan, 4, 3), pc(KindPo, TeamHan, 4, 1))
	eq(t, movesOf(t, pcs3, 4, 5), []Point{{4, 2}}, "포는 포를 못 잡는다")
}

func TestIsCheckAndBlock(t *testing.T) {
	// 한 궁(4,1) vs 초 차(4,5) — 파일이 비어 있으면 장군.
	base := pos(pc(KindGung, TeamHan, 4, 1), pc(KindCha, TeamCho, 4, 5), pc(KindGung, TeamCho, 4, 8))
	// 주의: 초 궁(4,8)이 같은 파일이지만 차(4,5)가 사이에 있어 상호 영향 없음.
	if !IsCheck(base, TeamHan) {
		t.Fatal("빈 파일의 차 장군을 놓쳤다")
	}
	// (4,3)에 한 졸을 세우면 차가 거기서 정지 → 장군 아님.
	blocked := append(append([]Piece{}, base...), pc(KindJol, TeamHan, 4, 3))
	if IsCheck(blocked, TeamHan) {
		t.Fatal("가로막힌 차를 장군으로 판정했다")
	}
}

func TestPinnedPieceIsFiltered(t *testing.T) {
	// 초 궁(4,8) — 초 차(4,5) — 한 차(4,0): 초 차는 핀 상태.
	// 옆으로 비키는 (3,5)는 자살수 → 불허. 파일 위 (4,4)는 계속 가림 → 허용.
	pcs := pos(pc(KindGung, TeamCho, 4, 8), pc(KindCha, TeamCho, 4, 5),
		pc(KindCha, TeamHan, 4, 0), pc(KindGung, TeamHan, 3, 0))
	chaIdx := 1
	if IsLegal(pcs, TeamCho, chaIdx, Point{3, 5}) {
		t.Fatal("핀된 차의 이탈(자살수)을 허용했다")
	}
	if !IsLegal(pcs, TeamCho, chaIdx, Point{4, 4}) {
		t.Fatal("핀 유지 이동을 거부했다")
	}
}

func TestCheckmate(t *testing.T) {
	// 한 궁(3,0) 궁성 구석. 궁의 후보지 = (3,1),(4,0),(4,1·대각).
	// 초 차 두 대가 3·4 파일을 비운 채 겨눔 → 현 위치 포함 전부 공격 = 외통.
	pcs := pos(pc(KindGung, TeamHan, 3, 0),
		pc(KindCha, TeamCho, 3, 5), pc(KindCha, TeamCho, 4, 5),
		pc(KindGung, TeamCho, 4, 8))
	if !IsCheckmate(pcs, TeamHan) {
		t.Fatalf("외통을 놓쳤다 — 남은 합법수: %v", LegalMoves(pcs, TeamHan))
	}
	// 차 하나를 5파일로 옮기면 (3,1) 등으로 피할 수... (3,1)은 3파일 차 공격권.
	// 4파일 차를 치우면 (4,0)·(4,1)이 열린다 → 외통 아님.
	pcs2 := pos(pc(KindGung, TeamHan, 3, 0),
		pc(KindCha, TeamCho, 3, 5),
		pc(KindGung, TeamCho, 4, 8))
	if IsCheckmate(pcs2, TeamHan) {
		t.Fatal("피할 길이 있는데 외통 판정")
	}
}

func TestApplyMoveCaptures(t *testing.T) {
	pcs := pos(pc(KindCha, TeamCho, 0, 5), pc(KindJol, TeamHan, 0, 3))
	next := ApplyMove(pcs, 0, Point{0, 3})
	if next[0].X != 0 || next[0].Y != 3 {
		t.Fatal("이동이 반영되지 않았다")
	}
	if next[1].Alive {
		t.Fatal("잡힌 말이 살아 있다")
	}
	if !pcs[1].Alive || pcs[0].Y != 5 {
		t.Fatal("원본이 변형됐다 — ApplyMove 는 복사본을 반환해야 한다")
	}
}
