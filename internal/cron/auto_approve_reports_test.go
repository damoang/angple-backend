package cron

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseAutoExecuteMode(t *testing.T) {
	cases := map[string]string{
		"":         autoExecuteModeOff,
		"off":      autoExecuteModeOff,
		"on":       autoExecuteModeOn,
		" ON ":     autoExecuteModeOn,
		"dry_run":  autoExecuteModeDryRun,
		"Dry_Run":  autoExecuteModeDryRun,
		"true":     autoExecuteModeOff,
		"dryrun":   autoExecuteModeOff,
		"enabled":  autoExecuteModeOff,
		"on;drop":  autoExecuteModeOff,
		"\ton\n":   autoExecuteModeOn,
		"unknown!": autoExecuteModeOff,
	}
	for in, want := range cases {
		if got := parseAutoExecuteMode(in); got != want {
			t.Errorf("parseAutoExecuteMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAutoExecuteMinOpinions(t *testing.T) {
	cases := map[string]int{"": 2, "x": 2, "0": 2, "1": 2, "2": 2, "3": 3, " 4 ": 4, "-5": 2}
	for in, want := range cases {
		if got := parseAutoExecuteMinOpinions(in); got != want {
			t.Errorf("parseAutoExecuteMinOpinions(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseHo23Reasons(t *testing.T) {
	ok := []struct {
		in   string
		want []int
	}{
		{"22", []int{22}},
		{"23", []int{23}},
		{"22,23", []int{22, 23}},
		{"23, 22", []int{23, 22}},
		{"[22,23]", []int{22, 23}},
		{`["22","23"]`, []int{22, 23}},
		{"22,22", []int{22}},
	}
	for _, c := range ok {
		got, valid := parseHo23Reasons(c.in)
		if !valid || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseHo23Reasons(%q) = %v,%v want %v,true", c.in, got, valid, c.want)
		}
	}
	bad := []string{"", " ", "[]", "21", "22,21", "23,24", "2", "3", "no_manner", "22,no_manner", "22,abc"}
	for _, in := range bad {
		if _, valid := parseHo23Reasons(in); valid {
			t.Errorf("parseHo23Reasons(%q) should be rejected", in)
		}
	}
}

func TestEvaluateAutoExecUnit(t *testing.T) {
	a := func(r, reasons string, days int) autoExecOpinion {
		return autoExecOpinion{Reviewer: r, Type: "action", Reasons: reasons, Days: days}
	}

	t.Run("두 명 일치는 대상", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("r2", "22,23", 5)}, 2)
		if !ev.OK || !reflect.DeepEqual(ev.Reasons, []int{22, 23}) || ev.AllZero {
			t.Fatalf("unexpected: %+v", ev)
		}
		if !reflect.DeepEqual(ev.Reviewers, []string{"r1", "r2"}) {
			t.Fatalf("reviewers: %v", ev.Reviewers)
		}
	})
	t.Run("한 명뿐이면 제외", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1)}, 2)
		if ev.OK || ev.Skip != "not_enough_opinions" {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
	t.Run("같은 담당의 중복 의견은 한 명", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("r1", "22", 1)}, 2)
		if ev.OK {
			t.Fatalf("should not pass: %+v", ev)
		}
	})
	t.Run("미처리 의견이 하나라도 있으면 제외", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("r2", "22", 1),
			{Reviewer: "r3", Type: "dismiss"}}, 2)
		if ev.OK || ev.Skip != "has_dismiss" {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
	t.Run("다른 사유가 섞이면 제외", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("r2", "21,22", 1)}, 2)
		if ev.OK || ev.Skip != "other_reason" {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
	t.Run("사유 없는 처분 의견은 제외", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("r2", "", 1)}, 2)
		if ev.OK {
			t.Fatalf("should not pass: %+v", ev)
		}
	})
	t.Run("담당 ID 를 알 수 없으면 제외", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("", "22", 1)}, 2)
		if ev.OK || ev.Skip != "unknown_reviewer" {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
	t.Run("자동화 계정은 담당 수에 세지 않는다", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 1), a("ai", "22", 1)}, 2)
		if ev.OK {
			t.Fatalf("should not pass: %+v", ev)
		}
	})
	t.Run("모두 0일이면 AllZero", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 0), a("r2", "22", 0)}, 2)
		if !ev.OK || !ev.AllZero {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
	t.Run("하나라도 0일이 아니면 AllZero 아님", func(t *testing.T) {
		ev := evaluateAutoExecUnit([]autoExecOpinion{a("r1", "22", 0), a("r2", "22", -1)}, 2)
		if !ev.OK || ev.AllZero {
			t.Fatalf("unexpected: %+v", ev)
		}
	})
}

func TestHasReviewerConflict(t *testing.T) {
	if !hasReviewerConflict([]string{"r1", "r2"}, []string{"x", "r2"}) {
		t.Error("스레드 작성자인 담당을 잡지 못했다")
	}
	if hasReviewerConflict([]string{"r1", "r2"}, []string{"x", "y", ""}) {
		t.Error("무관한 작성자를 충돌로 판정했다")
	}
	if hasReviewerConflict([]string{"r1"}, nil) {
		t.Error("작성자 목록이 비었는데 충돌")
	}
	if !hasReviewerConflict([]string{"r1"}, []string{" r1 "}) {
		t.Error("공백이 붙은 작성자 ID 를 놓쳤다")
	}
}

func TestComputeAutoExecuteDays(t *testing.T) {
	cases := []struct {
		name    string
		prior   int
		reasons []int
		allZero bool
		want    int
	}{
		{"모두 0일이면 주의", 10, []int{23}, true, 0},
		{"2호 1회차", 0, []int{22}, false, 1},
		{"2호 2회차도 상한 1일", 1, []int{22}, false, 1},
		{"2호 7회차 이상도 상한 1일", 8, []int{22}, false, 1},
		{"3호 1회차", 0, []int{23}, false, 1},
		{"3호 2회차", 1, []int{23}, false, 5},
		{"3호 3회차는 상한 5일", 2, []int{23}, false, 5},
		{"2·3호 4회차는 상한 5일", 3, []int{22, 23}, false, 5},
		{"2·3호 7회차(원칙 영구)도 상한 5일", 6, []int{22, 23}, false, 5},
		{"2·3호 1회차", 0, []int{22, 23}, false, 1},
	}
	for _, c := range cases {
		if got := computeAutoExecuteDays(c.prior, c.reasons, c.allZero); got != c.want {
			t.Errorf("%s: computeAutoExecuteDays(%d,%v,%v) = %d, want %d", c.name, c.prior, c.reasons, c.allZero, got, c.want)
		}
	}
}

func TestAutoExecuteLadderDays(t *testing.T) {
	want := map[int]int{1: 1, 2: 5, 3: 10, 4: 30, 5: 180, 6: 365, 7: permanentDays, 12: permanentDays}
	for round, days := range want {
		if got := autoExecuteLadderDays(round); got != days {
			t.Errorf("round %d = %d, want %d", round, got, days)
		}
	}
}

func TestBuildAutoExecuteMemberReason(t *testing.T) {
	const q2 = "「반말 등 예의를 갖추지 않은 행위」"
	const q3 = "「욕설, 비속어, 혐오표현 등 부적절한 표현을 사용하는 행위」"
	const honorific = "저희 운영정책은 모든 게시물과 댓글에서 경어체를 사용하도록 정하고 있습니다."

	t.Run("2호만·주의", func(t *testing.T) {
		got := buildAutoExecuteMemberReason([]int{22}, 0)
		want := "회원님께서 작성하신 게시물에 대해 안내드립니다.\n\n" +
			"신고된 게시물이 운영정책 제9조 제1항 제2호(예의없음) 「반말 등 예의를 갖추지 않은 행위」에 해당하는 것으로 확인되었습니다.\n\n" +
			honorific + "\n\n" +
			"이에 따라 주의 조치되었으며, 이용에는 제한이 없습니다.\n\n" +
			"이 처분에 이의가 있으시면 소명게시판을 이용하실 수 있습니다."
		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	})
	t.Run("2·3호·5일", func(t *testing.T) {
		got := buildAutoExecuteMemberReason([]int{22, 23}, 5)
		for _, s := range []string{"제2호(예의없음) " + q2, "제3호(부적절한 표현) " + q3, honorific, "이에 따라 이용제한 5일이 적용되었습니다."} {
			if !strings.Contains(got, s) {
				t.Errorf("missing %q in:\n%s", s, got)
			}
		}
	})
	t.Run("3호만은 2호를 언급하지 않는다", func(t *testing.T) {
		got := buildAutoExecuteMemberReason([]int{23}, 1)
		if strings.Contains(got, "제2호") || strings.Contains(got, honorific) {
			t.Errorf("3호만인데 2호 문구가 들어갔다:\n%s", got)
		}
		if !strings.Contains(got, q3) || !strings.Contains(got, "이용제한 1일") {
			t.Errorf("unexpected:\n%s", got)
		}
	})
}

func TestReasonsJSON(t *testing.T) {
	if got := reasonsJSON([]int{22, 23}); got != "[22,23]" {
		t.Errorf("got %q", got)
	}
	// 본 처리(parseDisciplineReasons)가 그대로 읽을 수 있어야 한다.
	s := reasonsJSON([]int{22})
	if got := parseDisciplineReasons(&s); !reflect.DeepEqual(got, []int{22}) {
		t.Errorf("round-trip failed: %v", got)
	}
}
