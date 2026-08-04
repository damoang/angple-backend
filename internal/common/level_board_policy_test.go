package common

import "testing"

func TestIsBoardWriteBlockedByLevel(t *testing.T) {
	cases := []struct {
		name  string
		level int
		board string
		want  bool
		why   string
	}{
		// 광고앙 — 이번 변경의 본체
		{"광고앙은 직접홍보에 쓸 수 있다", AdvertiserLevel, "promotion", false,
			"유일한 허용 게시판이다. 여기까지 막으면 광고앙이 아무것도 못 한다"},
		{"광고앙은 자유게시판에 못 쓴다", AdvertiserLevel, "free", true,
			"2026년에만 글 30·댓글 84건이 들어온 실제 유출 경로"},
		{"광고앙은 가입인사에 못 쓴다", AdvertiserLevel, "hello", true,
			"제보된 건. write_level=1 이라 사다리로는 절대 안 막힌다"},
		{"광고앙은 인증면제 게시판에도 못 쓴다", AdvertiserLevel, "verification", true,
			"cert 면제와 등급 제한은 별개다. 면제라고 뚫리면 안 된다"},

		// 다른 등급 — 회귀가 없어야 한다
		{"일반회원(3)은 영향 없다", 3, "free", false, "기존 사다리 그대로"},
		{"일반회원(3)도 promotion 은 사다리가 막는다", 3, "promotion", false,
			"여기서 false = '이 규칙은 관여 안 함'. 실제 차단은 write_level 5 가 한다"},
		{"신규회원(1)은 영향 없다", 1, "hello", false, ""},

		// ⛔ 경계 — >= 로 잘못 쓰면 여기서 깨진다
		{"등급 4 는 규칙 대상이 아니다", 4, "free", false, "5 미만"},
		{"등급 6 은 규칙 대상이 아니다", 6, "free", false, "5 초과 — >= 로 쓰면 여기서 실패"},
		{"관리자(10)는 절대 막히면 안 된다", 10, "free", false,
			"운영 마비를 부른다. == 비교의 존재 이유"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsBoardWriteBlockedByLevel(tc.level, tc.board)
			if got != tc.want {
				t.Errorf("IsBoardWriteBlockedByLevel(%d, %q) = %v, want %v — %s",
					tc.level, tc.board, got, tc.want, tc.why)
			}
		})
	}
}

// 광고앙 허용 목록은 의도적으로 하나뿐이다. 늘어나면 그 게시판은 광고 지면이 된다.
func TestAdvertiserAllowedBoardsIsIntentionallyMinimal(t *testing.T) {
	if len(advertiserAllowedBoards) != 1 {
		t.Fatalf("허용 게시판이 %d개다. 늘릴 때는 광고 목적 지면인지 확인하고 이 테스트도 함께 고쳐라",
			len(advertiserAllowedBoards))
	}
	if !advertiserAllowedBoards["promotion"] {
		t.Error("promotion 이 빠졌다. 광고앙이 글을 쓸 곳이 없어진다")
	}
}

func TestBlockedMessageIsActionable(t *testing.T) {
	// 어디에 쓰면 되는지 알려주지 않는 차단 문구는 문의를 만든다.
	if LevelBoardBlockedMessage == "" {
		t.Fatal("차단 문구가 비어 있다")
	}
	for _, must := range []string{"직접홍보"} {
		if !contains(LevelBoardBlockedMessage, must) {
			t.Errorf("차단 문구에 %q 가 없다 — 회원이 어디에 써야 할지 모른다", must)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
