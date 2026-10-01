package v2

import (
	"math"
	"math/rand"
	"testing"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약 (럭키 상품 표 선택):
//
//   - pickPrize 는 weight 비율로 고른다(대량 시뮬레이션에서 허용 오차 안, C15). weight<=0 줄은 절대 나오지 않는다.
//   - 상품 표가 비었거나 가중치 합이 0 이면 고르지 않는다 — 호출부는 레거시 points 로 v4 와 똑같이 준다(C19).
//   - 상품 줄의 포인트는 1..Points 균등, 경험치는 고정값이다.
//   - 고른 상품의 포인트·경험치가 지급(GrantWithOptions)에 그대로 넘어간다.
//
// 테스트의 가중치·금액은 임의 값이다.

// seededRandN 은 결정적 [0, n) 난수다(시뮬레이션 재현용).
func seededRandN(seed int64) func(int) int {
	r := rand.New(rand.NewSource(seed)) // #nosec G404 -- 테스트용 결정적 난수
	return r.Intn
}

// TestPickPrize_Ratio — C15: weight 3/5/2 로 대량 추첨하면 비율이 0.3/0.5/0.2 에 수렴한다. weight<=0 줄은 0회.
func TestPickPrize_Ratio(t *testing.T) {
	prizes := []v2repo.LuckyPrize{
		{Weight: 3, Points: 17},
		{Weight: 0, Points: 99999, Exp: 99999}, // 무시돼야 한다
		{Weight: 5, Exp: 37},
		{Weight: -4, Exp: 88888}, // 무시돼야 한다
		{Weight: 2, Points: 17, Exp: 41},
	}
	const n = 200000
	rng := seededRandN(20261001)
	counts := map[int]int{} // prizes 인덱스별
	for i := 0; i < n; i++ {
		p, ok := pickPrize(prizes, rng)
		if !ok {
			t.Fatal("유효한 줄이 있으면 항상 골라야 한다")
		}
		idx := -1
		for j := range prizes {
			if prizes[j] == p {
				idx = j
				break
			}
		}
		counts[idx]++
	}
	if counts[1] != 0 || counts[3] != 0 {
		t.Fatalf("weight<=0 줄이 나왔다: %v", counts)
	}
	want := map[int]float64{0: 0.3, 2: 0.5, 4: 0.2}
	for idx, w := range want {
		got := float64(counts[idx]) / n
		if math.Abs(got-w) > 0.01 {
			t.Errorf("줄 %d 비율 %.4f, 기대 %.2f(±0.01)", idx, got, w)
		}
	}
}

// TestPickPrize_Boundaries — rng 경계값이 정확한 줄로 떨어지는지(누적 가중치 구간) 본다.
func TestPickPrize_Boundaries(t *testing.T) {
	prizes := []v2repo.LuckyPrize{{Weight: 3, Points: 1}, {Weight: 0, Points: 2}, {Weight: 5, Points: 3}, {Weight: 2, Points: 4}}
	cases := []struct {
		r          int
		wantPoints int
	}{
		{0, 1}, {2, 1}, {3, 3}, {7, 3}, {8, 4}, {9, 4},
	}
	for _, c := range cases {
		var gotN int
		p, ok := pickPrize(prizes, func(n int) int { gotN = n; return c.r })
		if !ok || p.Points != c.wantPoints {
			t.Errorf("r=%d: got %+v ok=%v, want points=%d", c.r, p, ok, c.wantPoints)
		}
		if gotN != 10 {
			t.Errorf("rng 상한은 가중치 합(10)이어야 한다, got %d", gotN)
		}
	}
}

// TestPickPrize_EmptyFallsBack — 빈 표·가중치 합 0·rng 없음이면 고르지 않는다(호출부가 레거시 points 사용).
func TestPickPrize_EmptyFallsBack(t *testing.T) {
	called := false
	rng := func(int) int { called = true; return 0 }
	for name, prizes := range map[string][]v2repo.LuckyPrize{
		"nil":     nil,
		"빈 표":     {},
		"가중치 합 0": {{Weight: 0, Points: 5}, {Weight: -1, Exp: 7}},
	} {
		if _, ok := pickPrize(prizes, rng); ok {
			t.Errorf("%s: 고르면 안 된다", name)
		}
	}
	if called {
		t.Error("고를 줄이 없으면 rng 를 쓰지 않아야 한다")
	}
	if _, ok := pickPrize([]v2repo.LuckyPrize{{Weight: 1, Points: 5}}, nil); ok {
		t.Error("rng 가 없으면 고르지 않는다")
	}
}

// TestPrizeAmount_Uniform — 포인트는 1..Points 균등(0·초과 없음), Points<=0 이면 0.
func TestPrizeAmount_Uniform(t *testing.T) {
	rng := seededRandN(7)
	const maxPts = 13
	seen := make(map[int]int)
	for i := 0; i < 50000; i++ {
		a := prizeAmount(v2repo.LuckyPrize{Weight: 1, Points: maxPts}, rng)
		if a < 1 || a > maxPts {
			t.Fatalf("범위 밖 금액 %d", a)
		}
		seen[a]++
	}
	if len(seen) != maxPts {
		t.Errorf("1..%d 가 모두 나와야 한다, got %d 종류", maxPts, len(seen))
	}
	if a := prizeAmount(v2repo.LuckyPrize{Weight: 1, Exp: 37}, rng); a != 0 {
		t.Errorf("Points 없으면 0, got %d", a)
	}
}

// TestLuckyLive_PrizePassesPointsAndExp — 게시판 상품 표에서 고른 줄의 포인트·경험치가 지급으로 넘어간다.
func TestLuckyLive_PrizePassesPointsAndExp(t *testing.T) {
	prizes := []v2repo.LuckyPrize{{Weight: 3, Points: 17}, {Weight: 5, Exp: 37}, {Weight: 2, Points: 17, Exp: 41}}
	cases := []struct {
		name       string
		pick       int // 가중치 구간 값(0..9)
		wantAmount int
		wantExp    int
	}{
		{"포인트만", 0, 5, 0},
		{"경험치만", 4, 0, 37},
		{"둘 다", 9, 5, 41},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90, boardPrize: prizes}
			roller := &fakeRoller{win: true}
			l := newTestLive(store, roller, nil, testNow)
			call := 0
			l.randN = func(n int) int {
				call++
				if call == 1 { // 상품 줄 선택
					if n != 10 {
						t.Errorf("상품 선택 rng 상한=가중치 합 10, got %d", n)
					}
					return c.pick
				}
				if n != 17 { // 포인트 금액 1..17
					t.Errorf("포인트 rng 상한=17, got %d", n)
				}
				return 4 // → 5P
			}
			res := l.Process("member_a", "free", 7, false, 0)
			if len(store.grants) != 1 {
				t.Fatalf("지급 1회, got %d", len(store.grants))
			}
			g := store.grants[0]
			if g.amount != c.wantAmount || g.opt.Exp != c.wantExp || g.kind != v2repo.LuckyKindPost {
				t.Errorf("지급 값: amount=%d exp=%d kind=%s, want %d %d", g.amount, g.opt.Exp, g.kind, c.wantAmount, c.wantExp)
			}
			if !res.Won || res.Amount != c.wantAmount || res.Exp != c.wantExp {
				t.Errorf("결과: %+v", res)
			}
		})
	}
}

// TestLuckyLive_NoPrizesIsV4 — C19: 상품 표가 없으면 v4 와 같다 — 주사위 금액 그대로·경험치 0·상품 rng 미사용.
func TestLuckyLive_NoPrizesIsV4(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90}
	roller := &fakeRoller{win: true}
	l := newTestLive(store, roller, nil, testNow)
	l.randN = func(int) int { t.Fatal("상품 표가 없으면 상품 rng 를 쓰면 안 된다"); return 0 }
	res := l.Process("member_a", "free", 42, false, 0)
	if len(store.grants) != 1 {
		t.Fatalf("지급 1회, got %d", len(store.grants))
	}
	g := store.grants[0]
	if g.amount != 90 || g.opt.Exp != 0 || roller.points != 90 {
		t.Errorf("v4: 주사위 금액(90)·경험치 0, got amount=%d exp=%d", g.amount, g.opt.Exp)
	}
	if res.Amount != 90 || res.Exp != 0 {
		t.Errorf("결과: %+v", res)
	}

	// 가중치 합 0 인 표도 「없음」과 같다.
	store2 := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90, boardPrize: []v2repo.LuckyPrize{{Weight: 0, Exp: 37}}}
	l2 := newTestLive(store2, &fakeRoller{win: true}, nil, testNow)
	l2.randN = func(int) int { t.Fatal("가중치 합 0 이면 상품 rng 를 쓰면 안 된다"); return 0 }
	l2.Process("member_a", "free", 43, false, 0)
	if len(store2.grants) != 1 || store2.grants[0].amount != 90 || store2.grants[0].opt.Exp != 0 {
		t.Errorf("가중치 합 0 → 레거시, got %+v", store2.grants)
	}
}

// TestLuckyLive_PrizeOnlyBoard — 레거시 points 없이 상품 표만 있는 게시판도 발동한다.
func TestLuckyLive_PrizeOnlyBoard(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPrize: []v2repo.LuckyPrize{{Weight: 1, Exp: 37}}}
	l := newTestLive(store, &fakeRoller{win: true}, nil, testNow)
	l.randN = func(int) int { return 0 }
	res := l.Process("member_a", "free", 8, false, 0)
	if !res.Rolled || len(store.grants) != 1 || store.grants[0].amount != 0 || store.grants[0].opt.Exp != 37 {
		t.Errorf("상품 표만으로 발동·경험치만 지급, res=%+v grants=%+v", res, store.grants)
	}
}

// TestLuckyLive_BlankPrizeRowGrantsNothing — 포인트·경험치 모두 0 인 줄(꽝)이 뽑히면 지급을 부르지 않는다.
func TestLuckyLive_BlankPrizeRowGrantsNothing(t *testing.T) {
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 90,
		boardPrize: []v2repo.LuckyPrize{{Weight: 1}, {Weight: 1, Exp: 37}}}
	l := newTestLive(store, &fakeRoller{win: true}, nil, testNow)
	l.randN = func(int) int { return 0 } // 첫 줄(꽝)
	res := l.Process("member_a", "free", 9, false, 0)
	if res.Won || len(store.grants) != 0 {
		t.Errorf("꽝 줄이면 지급 없음, res=%+v grants=%d", res, len(store.grants))
	}
}

// TestLuckyLive_WindowPrizes — 시간대 안이면 그 단계의 상품 표를 쓴다(게시판 표가 아니라).
func TestLuckyLive_WindowPrizes(t *testing.T) {
	cfg := enabledCfg()
	cfg.WindowStartHour, cfg.WindowEndHour = 0, 24
	cfg.Windows = []v2repo.LuckyWindow{{Name: "온종일", Minutes: 24 * 60, Odds: 1,
		Prizes: []v2repo.LuckyPrize{{Weight: 1, Exp: 41}}}}
	store := &fakeLuckyStore{cfg: cfg, boardOdds: 13, boardPts: 90, boardPrize: []v2repo.LuckyPrize{{Weight: 1, Exp: 37}}}
	l := newTestLive(store, &fakeRoller{win: true}, []byte("k"), testNow)
	l.randN = func(int) int { return 0 }
	res := l.Process("member_a", "free", 10, false, 0)
	if res.Tier != "온종일" || len(store.grants) != 1 || store.grants[0].opt.Exp != 41 || store.grants[0].opt.TierName != "온종일" {
		t.Errorf("단계 상품 표(41)가 쓰여야 한다, res=%+v grants=%+v", res, store.grants)
	}
}
