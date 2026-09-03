package v2

import "testing"

// hot_feed_test.go — GET /api/v2/feed/hot 쿼리 파라미터 정규화 헬퍼 테스트.
// hours 정규화(NormalizeHotHours)는 리포 패키지 테스트가 담당한다.

func TestHotFeedLimit(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 20},     // 기본
		{"abc", 20},  // 비숫자
		{"0", 20},    // 0 이하
		{"-3", 20},   // 음수
		{"1", 1},     // 하한
		{"20", 20},   // 기본과 동일
		{"30", 30},   // 상한
		{"31", 20},   // 초과는 기본으로 (/feed 와 같은 규칙 — 30 으로 늘리지 않는다)
		{"1000", 20}, // 초과
	}
	for _, tc := range cases {
		if got := hotFeedLimit(tc.in); got != tc.want {
			t.Errorf("hotFeedLimit(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestHotFeedPage(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 1},    // 기본
		{"abc", 1}, // 비숫자
		{"0", 1},   // 1 미만
		{"-1", 1},  // 음수
		{"1", 1},
		{"7", 7},
		{"100", 100}, // 상한 없음 — 바운디드 풀이라 깊은 페이지도 안전하다
	}
	for _, tc := range cases {
		if got := hotFeedPage(tc.in); got != tc.want {
			t.Errorf("hotFeedPage(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
