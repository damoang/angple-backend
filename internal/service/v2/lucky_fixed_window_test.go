package v2

import (
	"testing"
	"time"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약 (고정 시간대):
//
//   - F1 시각 판정은 KST 기준 [start, end) 다(시작 포함·끝 제외). 자정을 넘는 구간을 받는다. 입력 시간대와 무관하다.
//   - F2 우선순위는 무작위 window > 고정 시간대 > 앙복타임(게시판). 고정 시간대끼리 겹치면 배열 앞쪽.
//     fixed_windows 가 비면 기존(v6) 동작과 같다. 게시판이 꺼져 있으면 고정 시간대도 발동하지 않는다.
//
// 이름·확률·금액은 모두 임의의 테스트 값이다.

func fixedCfg(fw ...v2repo.LuckyFixedWindow) *v2repo.LuckyConfig {
	c := enabledCfg()
	c.FixedWindows = fw
	return c
}

func kstAt(h, m, s int) time.Time {
	return time.Date(2026, 10, 1, h, m, s, 0, luckyKST)
}

// TestParseLuckyClock — "HH:MM" 두 자리 고정만 받는다.
func TestParseLuckyClock(t *testing.T) {
	ok := map[string]int{"00:00": 0, "09:05": 545, "23:59": 1439, "12:30": 750}
	for s, want := range ok {
		got, valid := ParseLuckyClock(s)
		if !valid || got != want {
			t.Errorf("ParseLuckyClock(%q) = %d,%v want %d,true", s, got, valid, want)
		}
	}
	for _, s := range []string{"", "9:00", "24:00", "12:60", "12-30", "12:3", " 12:30", "12:30 ", "1230", "aa:bb"} {
		if _, valid := ParseLuckyClock(s); valid {
			t.Errorf("ParseLuckyClock(%q) 는 거부돼야 한다", s)
		}
	}
}

// TestFixedWindowContains — F1: 시작 포함·끝 제외, 자정 넘김, 같은 시각은 빈 구간.
func TestFixedWindowContains(t *testing.T) {
	cases := []struct {
		name               string
		start, end, minute int
		want               bool
	}{
		{"보통 구간 시작 포함", 600, 700, 600, true},
		{"보통 구간 안", 600, 700, 650, true},
		{"보통 구간 끝 제외", 600, 700, 700, false},
		{"보통 구간 직전", 600, 700, 599, false},
		{"자정 넘김 시작 포함", 1380, 120, 1380, true},
		{"자정 넘김 자정 전", 1380, 120, 1439, true},
		{"자정 넘김 0시", 1380, 120, 0, true},
		{"자정 넘김 끝 직전", 1380, 120, 119, true},
		{"자정 넘김 끝 제외", 1380, 120, 120, false},
		{"자정 넘김 밖(낮)", 1380, 120, 720, false},
		{"자정 넘김 시작 직전", 1380, 120, 1379, false},
		{"같은 시각은 빈 구간", 600, 600, 600, false},
	}
	for _, c := range cases {
		if got := fixedWindowContains(c.start, c.end, c.minute); got != c.want {
			t.Errorf("%s: fixedWindowContains(%d,%d,%d)=%v want %v", c.name, c.start, c.end, c.minute, got, c.want)
		}
	}
}

// TestLuckyLive_FixedWindowBoundaries — F1: Process 단에서도 경계가 같고, UTC 로 들어온 시각도 KST 로 본다.
func TestLuckyLive_FixedWindowBoundaries(t *testing.T) {
	fw := v2repo.LuckyFixedWindow{Name: "테스트구간", Start: "23:10", End: "01:20", Odds: 4, Points: 33}
	cases := []struct {
		name string
		now  time.Time
		in   bool
	}{
		{"시작 정각(포함)", kstAt(23, 10, 0), true},
		{"시작 1초 전", kstAt(23, 9, 59), false},
		{"자정 넘어 안", kstAt(0, 30, 0), true},
		{"끝 1초 전", kstAt(1, 19, 59), true},
		{"끝 정각(제외)", kstAt(1, 20, 0), false},
		{"UTC 로 준 같은 순간(KST 23:30)", time.Date(2026, 10, 1, 14, 30, 0, 0, time.UTC), true},
		{"UTC 로 준 밖(KST 12:00)", time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC), false},
	}
	for _, c := range cases {
		store := &fakeLuckyStore{cfg: fixedCfg(fw), boardOdds: 50, boardPts: 7}
		roller := &fakeRoller{win: true}
		res := newTestLive(store, roller, nil, c.now).Process("member_a", "free", 1, false, 0)
		wantTier, wantOdds := LuckyBaseTierName, 50
		if c.in {
			wantTier, wantOdds = "테스트구간", 4
		}
		if res.Tier != wantTier || roller.odds != wantOdds {
			t.Errorf("%s: tier=%q odds=%d, want %q/%d", c.name, res.Tier, roller.odds, wantTier, wantOdds)
		}
		if c.in && (len(store.grants) != 1 || store.grants[0].opt.TierName != "테스트구간") {
			t.Errorf("%s: 고정 시간대 이름이 지급 문구 단계로 넘어가야 한다: %+v", c.name, store.grants)
		}
	}
}

// TestLuckyLive_FixedWindowPriority — F2: 무작위 window > 고정 > 앙복타임.
func TestLuckyLive_FixedWindowPriority(t *testing.T) {
	fixed := v2repo.LuckyFixedWindow{Name: "고정A", Start: "00:00", End: "23:59", Odds: 8, Points: 11}

	// 무작위 window 가 하루 전체를 덮으면(시작 0분 하나뿐) 고정 시간대보다 앞선다.
	cfg := fixedCfg(fixed)
	cfg.WindowStartHour, cfg.WindowEndHour = 0, 24
	cfg.Windows = []v2repo.LuckyWindow{{Name: "무작위A", Minutes: 24 * 60, Odds: 3, Points: 22}}
	store := &fakeLuckyStore{cfg: cfg, boardOdds: 50, boardPts: 7}
	roller := &fakeRoller{win: true}
	res := newTestLive(store, roller, []byte("k"), testNow).Process("member_a", "free", 1, false, 0)
	if res.Tier != "무작위A" || roller.odds != 3 {
		t.Fatalf("무작위 window 가 우선이어야 한다: tier=%q odds=%d", res.Tier, roller.odds)
	}

	// 키가 없으면 무작위 window 는 꺼지고 고정 시간대가 적용된다.
	store = &fakeLuckyStore{cfg: cfg, boardOdds: 50, boardPts: 7}
	roller = &fakeRoller{win: true}
	res = newTestLive(store, roller, nil, testNow).Process("member_a", "free", 2, false, 0)
	if res.Tier != "고정A" || roller.odds != 8 || roller.points != 11 {
		t.Fatalf("무작위가 없으면 고정 시간대: tier=%q odds=%d points=%d", res.Tier, roller.odds, roller.points)
	}

	// 고정 시간대끼리 겹치면 배열 앞쪽.
	store = &fakeLuckyStore{cfg: fixedCfg(
		v2repo.LuckyFixedWindow{Name: "앞쪽", Start: "11:00", End: "13:00", Odds: 6, Points: 9},
		v2repo.LuckyFixedWindow{Name: "뒤쪽", Start: "10:00", End: "14:00", Odds: 8, Points: 9},
	), boardOdds: 50, boardPts: 7}
	roller = &fakeRoller{win: true}
	res = newTestLive(store, roller, nil, testNow).Process("member_a", "free", 3, false, 0)
	if res.Tier != "앞쪽" {
		t.Fatalf("겹치면 배열 앞쪽: got %q", res.Tier)
	}

	// 밖이면 앙복타임(게시판 값).
	store = &fakeLuckyStore{cfg: fixedCfg(v2repo.LuckyFixedWindow{Name: "고정B", Start: "04:00", End: "05:00", Odds: 8, Points: 11}), boardOdds: 50, boardPts: 7}
	roller = &fakeRoller{win: true}
	res = newTestLive(store, roller, nil, testNow).Process("member_a", "free", 4, false, 0)
	if res.Tier != LuckyBaseTierName || roller.odds != 50 {
		t.Fatalf("밖이면 앙복타임: tier=%q odds=%d", res.Tier, roller.odds)
	}
}

// TestLuckyLive_FixedWindowEmptyIsV6 — F2: fixed_windows 가 없으면 기존과 똑같다.
func TestLuckyLive_FixedWindowEmptyIsV6(t *testing.T) {
	for _, fw := range [][]v2repo.LuckyFixedWindow{nil, {}} {
		store := &fakeLuckyStore{cfg: fixedCfg(fw...), boardOdds: 50, boardPts: 7}
		roller := &fakeRoller{win: true}
		res := newTestLive(store, roller, nil, testNow).Process("member_a", "free", 1, false, 0)
		if res.Tier != LuckyBaseTierName || roller.odds != 50 || roller.points != 7 {
			t.Errorf("비어 있으면 앙복타임: %+v roller=%+v", res, roller)
		}
	}
}

// TestLuckyLive_FixedWindowRespectsBoardAndComments — 게시판이 꺼져 있으면 고정 시간대도 미발동, 댓글 확률이 없으면 댓글 미발동.
func TestLuckyLive_FixedWindowRespectsBoardAndComments(t *testing.T) {
	fw := v2repo.LuckyFixedWindow{Name: "고정C", Start: "00:00", End: "23:59", Odds: 7, Points: 5}

	store := &fakeLuckyStore{cfg: fixedCfg(fw)} // 게시판 꺼짐
	roller := &fakeRoller{win: true}
	res := newTestLive(store, roller, nil, testNow).Process("member_a", "promotion", 1, false, 0)
	if res.Rolled || roller.calls != 0 {
		t.Fatalf("게시판이 꺼져 있으면 고정 시간대도 미발동: %+v", res)
	}

	cfg := fixedCfg(fw) // comment_odds 0
	cfg.IncludeComments = true
	cfg.MinCommentChars = 0
	store = &fakeLuckyStore{cfg: cfg, boardOdds: 50, boardCOdds: 60, boardPts: 7}
	roller = &fakeRoller{win: true}
	res = newTestLive(store, roller, nil, testNow).Process("member_a", "free", 2, true, 30)
	if res.Rolled || res.Tier != "고정C" {
		t.Fatalf("고정 시간대에 댓글 확률이 없으면 댓글 미발동: %+v", res)
	}

	cfg.FixedWindows[0].CommentOdds = 9
	store = &fakeLuckyStore{cfg: cfg, boardOdds: 50, boardCOdds: 60, boardPts: 7}
	roller = &fakeRoller{win: true}
	res = newTestLive(store, roller, nil, testNow).Process("member_a", "free", 3, true, 30)
	if !res.Rolled || roller.odds != 9 || store.grants[0].kind != v2repo.LuckyKindComment {
		t.Fatalf("댓글 확률이 있으면 그 확률로: %+v roller=%+v", res, roller)
	}
}

// TestActiveFixedWindow_SkipsInvalid — 형식이 틀렸거나 줄 것이 없는 줄은 건너뛴다(DB 직접 수정 방어).
func TestActiveFixedWindow_SkipsInvalid(t *testing.T) {
	cfg := fixedCfg(
		v2repo.LuckyFixedWindow{Name: "형식오류", Start: "9:00", End: "23:00", Odds: 7, Points: 5},
		v2repo.LuckyFixedWindow{Name: "확률없음", Start: "00:00", End: "23:00", Odds: 0, Points: 5},
		v2repo.LuckyFixedWindow{Name: "지급없음", Start: "00:00", End: "23:00", Odds: 7},
		v2repo.LuckyFixedWindow{Name: " ", Start: "00:00", End: "23:00", Odds: 7, Points: 5},
		v2repo.LuckyFixedWindow{Name: "정상", Start: "00:00", End: "23:00", Odds: 7, Prizes: []v2repo.LuckyPrize{{Weight: 1, Exp: 3}}},
	)
	f, ok := activeFixedWindow(cfg, testNow)
	if !ok || f.Name != "정상" {
		t.Fatalf("유효한 줄만 골라야 한다: %+v %v", f, ok)
	}
}
