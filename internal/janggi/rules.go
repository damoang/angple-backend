// Package janggi 는 장기 규칙 엔진이다.
//
// ⛔ 원본 = 웹 클라이언트 janggi-game.svelte 의 TS 구현 (getMoves/inPalace/
// onPalaceDiag/isCheck/applyMove). 회원들이 이미 그 규칙으로 AI 와 두고 있으므로
// 서버는 **같은 규칙**을 판정 기준으로 삼아야 한다. 함수·분기 구조를 의도적으로
// TS 와 1:1 로 맞춰 두었다 — 규칙 수정 시 양쪽을 같이 고치고, 대조 테스트
// (rules_test.go)로 어긋남을 잡는다.
//
// 오목과 동일 원칙: 착수 유효성·승패 판정은 전부 서버가 한다. 클라이언트는
// 좌표만 보낸다.
package janggi

// 판 크기 — 가로 9줄(파일), 세로 10줄(랭크).
const (
	Cols = 9
	Rows = 10
)

// 말 종류. 값은 TS KIND 와 동일하게 유지한다(대조 테스트·직렬화 호환).
const (
	KindGung = 1 // 궁(왕)
	KindCha  = 2 // 차
	KindSang = 3 // 상
	KindMa   = 4 // 마
	KindSa   = 5 // 사
	KindPo   = 6 // 포
	KindJol  = 7 // 졸/병
)

// 진영. 초(Cho)=하단(y 큰 쪽), 한(Han)=상단. TS 의 USER/COM 과 같은 값이다.
const (
	TeamCho = 1
	TeamHan = 2
)

// Piece 는 말 하나다. 좌표는 x∈[0,9), y∈[0,10).
type Piece struct {
	Kind  int  `json:"kind"`
	Team  int  `json:"team"`
	X     int  `json:"x"`
	Y     int  `json:"y"`
	Alive bool `json:"alive"`
}

// Point 는 판 위 좌표다.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Board 는 좌표→말 조회용 격자다. b[y][x], 빈 칸은 nil.
type Board [Rows][Cols]*Piece

// InitPieces 는 초기 배치를 돌려준다. 배치 좌표는 TS initPieces 와 동일하다.
// (마상상마 변형 배치는 1차 범위 밖 — 클라이언트도 고정 배치다.)
func InitPieces() []Piece {
	p := make([]Piece, 0, 32)
	add := func(kind, team, x, y int) {
		p = append(p, Piece{Kind: kind, Team: team, X: x, Y: y, Alive: true})
	}
	// 한(상단)
	add(KindCha, TeamHan, 0, 0)
	add(KindSang, TeamHan, 1, 0)
	add(KindMa, TeamHan, 2, 0)
	add(KindSa, TeamHan, 3, 0)
	add(KindSa, TeamHan, 5, 0)
	add(KindSang, TeamHan, 6, 0)
	add(KindMa, TeamHan, 7, 0)
	add(KindCha, TeamHan, 8, 0)
	add(KindGung, TeamHan, 4, 1)
	add(KindPo, TeamHan, 1, 2)
	add(KindPo, TeamHan, 7, 2)
	for i := 0; i < 5; i++ {
		add(KindJol, TeamHan, i*2, 3)
	}
	// 초(하단)
	add(KindCha, TeamCho, 0, 9)
	add(KindSang, TeamCho, 1, 9)
	add(KindMa, TeamCho, 2, 9)
	add(KindSa, TeamCho, 3, 9)
	add(KindSa, TeamCho, 5, 9)
	add(KindSang, TeamCho, 6, 9)
	add(KindMa, TeamCho, 7, 9)
	add(KindCha, TeamCho, 8, 9)
	add(KindGung, TeamCho, 4, 8)
	add(KindPo, TeamCho, 1, 7)
	add(KindPo, TeamCho, 7, 7)
	for i := 0; i < 5; i++ {
		add(KindJol, TeamCho, i*2, 6)
	}
	return p
}

// MakeBoard 는 말 목록을 격자로 바꾼다 (TS getBoard).
func MakeBoard(pcs []Piece) *Board {
	var b Board
	for i := range pcs {
		if pcs[i].Alive {
			b[pcs[i].Y][pcs[i].X] = &pcs[i]
		}
	}
	return &b
}

// inPalace 는 (x,y)가 team 의 궁성 안인지 본다.
func inPalace(x, y, team int) bool {
	if x < 3 || x > 5 {
		return false
	}
	if team == TeamHan {
		return y >= 0 && y <= 2
	}
	return y >= 7 && y <= 9
}

// onPalaceDiag 는 (x,y)가 궁성 대각선 위 점인지 본다.
// 대각선 = 네 모서리와 중심을 잇는 X 자. 중심(4,1)·(4,8)은 별도 취급 지점이
// 있어 TS 와 같은 식을 그대로 쓴다.
func onPalaceDiag(x, y int) bool {
	if x >= 3 && x <= 5 && y >= 0 && y <= 2 {
		return x-3 == y || 5-x == y
	}
	if x >= 3 && x <= 5 && y >= 7 && y <= 9 {
		return x-3 == y-7 || 5-x == y-7
	}
	return false
}

// palaceDiagOrCenter 는 궁성 대각 경로로 통과 가능한 점인지 본다
// (대각선 위이거나 중심점). 차·포의 대각 이동 경로 판정에 쓴다.
func palaceDiagOrCenter(x, y int) bool {
	return onPalaceDiag(x, y) || (x == 4 && (y == 1 || y == 8))
}

// Moves 는 말 하나의 의사 합법수(pseudo-legal)를 돌려준다 — 자살수(장군 방치)
// 필터는 LegalMoves 가 한다. 분기 구조는 TS getMoves 와 1:1 이다.
func Moves(p Piece, b *Board) []Point {
	moves := []Point{}
	addIf := func(nx, ny int) {
		if nx < 0 || nx >= Cols || ny < 0 || ny >= Rows {
			return
		}
		if t := b[ny][nx]; t != nil && t.Team == p.Team {
			return
		}
		moves = append(moves, Point{nx, ny})
	}

	switch p.Kind {
	case KindGung, KindSa:
		// 궁·사 — 궁성 안 한 칸. 대각선 위에 있을 때만 대각 이동.
		dirs := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
		if onPalaceDiag(p.X, p.Y) {
			dirs = append(dirs, [2]int{-1, -1}, [2]int{1, -1}, [2]int{-1, 1}, [2]int{1, 1})
		}
		for _, d := range dirs {
			nx, ny := p.X+d[0], p.Y+d[1]
			if inPalace(nx, ny, p.Team) {
				addIf(nx, ny)
			}
		}

	case KindCha:
		// 차 — 직선 활주 + 궁성 대각 활주.
		for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			nx, ny := p.X+d[0], p.Y+d[1]
			for nx >= 0 && nx < Cols && ny >= 0 && ny < Rows {
				if t := b[ny][nx]; t != nil {
					if t.Team != p.Team {
						moves = append(moves, Point{nx, ny})
					}
					break
				}
				moves = append(moves, Point{nx, ny})
				nx += d[0]
				ny += d[1]
			}
		}
		if onPalaceDiag(p.X, p.Y) {
			for _, d := range [][2]int{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				nx, ny := p.X+d[0], p.Y+d[1]
				for nx >= 0 && nx < Cols && ny >= 0 && ny < Rows {
					if !palaceDiagOrCenter(nx, ny) {
						break
					}
					if t := b[ny][nx]; t != nil {
						if t.Team != p.Team {
							moves = append(moves, Point{nx, ny})
						}
						break
					}
					moves = append(moves, Point{nx, ny})
					nx += d[0]
					ny += d[1]
				}
			}
		}

	case KindMa:
		// 마 — 직선 한 칸(멱) + 대각 한 칸. 멱이 막히면 불가.
		steps := [][4]int{
			{0, -1, -1, -2}, {0, -1, 1, -2}, {0, 1, -1, 2}, {0, 1, 1, 2},
			{-1, 0, -2, -1}, {-1, 0, -2, 1}, {1, 0, 2, -1}, {1, 0, 2, 1},
		}
		for _, s := range steps {
			bx, by := p.X+s[0], p.Y+s[1]
			if bx < 0 || bx >= Cols || by < 0 || by >= Rows {
				continue
			}
			if b[by][bx] != nil {
				continue // 멱 막힘
			}
			addIf(p.X+s[2], p.Y+s[3])
		}

	case KindSang:
		// 상 — 직선 한 칸 + 대각 두 칸. 경로 두 점 중 하나라도 막히면 불가.
		steps := [][6]int{
			{0, -1, -1, -2, -2, -3}, {0, -1, 1, -2, 2, -3},
			{0, 1, -1, 2, -2, 3}, {0, 1, 1, 2, 2, 3},
			{-1, 0, -2, -1, -3, -2}, {-1, 0, -2, 1, -3, 2},
			{1, 0, 2, -1, 3, -2}, {1, 0, 2, 1, 3, 2},
		}
		for _, s := range steps {
			b1x, b1y := p.X+s[0], p.Y+s[1]
			b2x, b2y := p.X+s[2], p.Y+s[3]
			if b1x < 0 || b1x >= Cols || b1y < 0 || b1y >= Rows {
				continue
			}
			if b2x < 0 || b2x >= Cols || b2y < 0 || b2y >= Rows {
				continue
			}
			if b[b1y][b1x] != nil || b[b2y][b2x] != nil {
				continue
			}
			addIf(p.X+s[4], p.Y+s[5])
		}

	case KindPo:
		// 포 — 정확히 하나를 넘어 활주. 포는 넘지도, 잡지도 못한다.
		for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			nx, ny := p.X+d[0], p.Y+d[1]
			jumped := false
			for nx >= 0 && nx < Cols && ny >= 0 && ny < Rows {
				t := b[ny][nx]
				if !jumped {
					if t != nil {
						if t.Kind == KindPo {
							break // 포는 포를 못 넘는다
						}
						jumped = true
					}
				} else {
					if t != nil {
						if t.Team != p.Team && t.Kind != KindPo {
							moves = append(moves, Point{nx, ny})
						}
						break
					}
					moves = append(moves, Point{nx, ny})
				}
				nx += d[0]
				ny += d[1]
			}
		}
		// 궁성 대각 — 중심을 넘는 두 칸 이동(모서리↔모서리).
		if onPalaceDiag(p.X, p.Y) {
			for _, d := range [][2]int{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				mx, my := p.X+d[0], p.Y+d[1]
				tx, ty := p.X+d[0]*2, p.Y+d[1]*2
				if mx < 0 || mx >= Cols || my < 0 || my >= Rows {
					continue
				}
				if tx < 0 || tx >= Cols || ty < 0 || ty >= Rows {
					continue
				}
				if !palaceDiagOrCenter(mx, my) || !palaceDiagOrCenter(tx, ty) {
					continue
				}
				mid := b[my][mx]
				if mid == nil || mid.Kind == KindPo {
					continue // 넘을 기물(포 제외)이 있어야 한다
				}
				t := b[ty][tx]
				if t == nil {
					moves = append(moves, Point{tx, ty})
				} else if t.Team != p.Team && t.Kind != KindPo {
					moves = append(moves, Point{tx, ty})
				}
			}
		}

	case KindJol:
		// 졸·병 — 앞·좌·우 한 칸. 후퇴 없음. 궁성 대각선 위에선 전방 대각 허용.
		forward := 1
		if p.Team == TeamCho {
			forward = -1
		}
		addIf(p.X, p.Y+forward)
		addIf(p.X-1, p.Y)
		addIf(p.X+1, p.Y)
		if onPalaceDiag(p.X, p.Y) {
			addIf(p.X-1, p.Y+forward)
			addIf(p.X+1, p.Y+forward)
		}
	}

	return moves
}

// ApplyMove 는 pcs 를 복사해 idx 번 말을 to 로 옮긴 결과를 돌려준다.
// 도착 칸의 상대 말은 잡힌다(Alive=false). 원본은 바꾸지 않는다.
func ApplyMove(pcs []Piece, idx int, to Point) []Piece {
	out := make([]Piece, len(pcs))
	copy(out, pcs)
	mover := out[idx]
	for i := range out {
		if i != idx && out[i].Alive && out[i].X == to.X && out[i].Y == to.Y && out[i].Team != mover.Team {
			out[i].Alive = false
		}
	}
	out[idx].X = to.X
	out[idx].Y = to.Y
	return out
}

// IsCheck 는 team 의 궁이 공격받고 있는지 본다 (TS isCheck).
// 궁이 없으면(잡힘) true — 이미 진 국면이다.
func IsCheck(pcs []Piece, team int) bool {
	b := MakeBoard(pcs)
	var king *Piece
	for i := range pcs {
		if pcs[i].Alive && pcs[i].Kind == KindGung && pcs[i].Team == team {
			king = &pcs[i]
			break
		}
	}
	if king == nil {
		return true
	}
	for i := range pcs {
		if !pcs[i].Alive || pcs[i].Team == team {
			continue
		}
		for _, m := range Moves(pcs[i], b) {
			if m.X == king.X && m.Y == king.Y {
				return true
			}
		}
	}
	return false
}

// Move 는 한 수다. PieceIdx 는 pcs 슬라이스 인덱스.
type Move struct {
	PieceIdx int   `json:"pieceIdx"`
	To       Point `json:"to"`
}

// LegalMoves 는 team 의 합법수 전부를 돌려준다.
// ⛔ 클라이언트 TS 와 다른 유일한 지점: 여기서는 **자살수(두고 나면 자기 궁이
// 장군인 수)를 거른다.** 클라는 화면 안내만 하고 실제 판정은 서버가 하므로,
// 서버가 거르지 않으면 장군 방치가 성립해 버린다.
func LegalMoves(pcs []Piece, team int) []Move {
	b := MakeBoard(pcs)
	out := []Move{}
	for i := range pcs {
		if !pcs[i].Alive || pcs[i].Team != team {
			continue
		}
		for _, m := range Moves(pcs[i], b) {
			next := ApplyMove(pcs, i, m)
			if !IsCheck(next, team) {
				out = append(out, Move{PieceIdx: i, To: m})
			}
		}
	}
	return out
}

// IsLegal 은 특정 수가 합법인지 본다 (서버 착수 검증용).
func IsLegal(pcs []Piece, team, idx int, to Point) bool {
	if idx < 0 || idx >= len(pcs) || !pcs[idx].Alive || pcs[idx].Team != team {
		return false
	}
	b := MakeBoard(pcs)
	found := false
	for _, m := range Moves(pcs[idx], b) {
		if m == to {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	return !IsCheck(ApplyMove(pcs, idx, to), team)
}

// IsCheckmate 는 외통(장군 + 피할 수 없음)인지 본다.
// 합법수가 없어도 장군이 아니면 외통이 아니다 — 장기는 그 경우 한 수 쉼(패스)이
// 가능하다. 패스 처리 자체는 서버(P2)의 턴 관리에서 한다.
func IsCheckmate(pcs []Piece, team int) bool {
	return IsCheck(pcs, team) && len(LegalMoves(pcs, team)) == 0
}
