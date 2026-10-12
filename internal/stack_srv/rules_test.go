package stacksrv

import (
	"strings"
	"testing"
)

func TestAttackTable(t *testing.T) {
	want := []int{0, 0, 1, 2, 4}
	for cleared, w := range want {
		if got := AttackFor(cleared); got != w {
			t.Fatalf("AttackFor(%d) = %d, want %d", cleared, got, w)
		}
	}
}

func TestGarbageQueueOffset(t *testing.T) {
	q := garbageQueue{{lines: 2, hole: 1}, {lines: 4, hole: 5}}
	rest, send := q.cancel(3)
	if send != 0 || rest.total() != 3 || len(rest) != 1 || rest[0].hole != 5 {
		t.Fatalf("cancel(3) = %+v, send %d", rest, send)
	}
	rest, send = rest.cancel(4)
	if send != 1 || rest.total() != 0 {
		t.Fatalf("cancel(4) = %+v, send %d", rest, send)
	}
}

func TestGarbageQueueCaps(t *testing.T) {
	var q garbageQueue
	for i := 0; i < 4; i++ {
		q = q.push(4, i)
	}
	if q.total() != GarbagePendingMax {
		t.Fatalf("대기 상한: total = %d", q.total())
	}
	got, rest := q.take(GarbagePerLockMax)
	n := 0
	for _, c := range got {
		n += c.lines
	}
	if n != GarbagePerLockMax || rest.total() != GarbagePendingMax-GarbagePerLockMax {
		t.Fatalf("take = %+v rest %+v", got, rest)
	}
	// 묶음이 잘려도 남은 쪽은 같은 구멍 열을 유지한다.
	q2 := garbageQueue{{lines: 5, hole: 2}, {lines: 5, hole: 7}}
	got, rest = q2.take(8)
	if len(got) != 2 || got[1].lines != 3 || got[1].hole != 7 || rest[0].lines != 2 || rest[0].hole != 7 {
		t.Fatalf("partial take = %+v rest %+v", got, rest)
	}
}

func TestCellBalance(t *testing.T) {
	cases := []struct {
		locks, garbage, cleared int
		ok                      bool
	}{
		{0, 0, 0, true},
		{9, 0, 4, true},
		{1, 0, 4, false},
		{0, 1, 0, true},
		{2, 2, 2, true},
		{41, 0, 0, false},
		{40, 1, 1, true},
	}
	for _, c := range cases {
		if got := CellBalanceOK(c.locks, c.garbage, c.cleared); got != c.ok {
			t.Fatalf("CellBalanceOK(%d, %d, %d) = %v", c.locks, c.garbage, c.cleared, got)
		}
	}
}

func TestValidBoard(t *testing.T) {
	if !ValidBoard(strings.Repeat("0", 161) + "8") {
		t.Fatal("'8'(방해 줄)을 거부했다")
	}
	for _, b := range []string{strings.Repeat("0", 161), strings.Repeat("0", 163), strings.Repeat("0", 161) + "9", strings.Repeat("0", 161) + "a"} {
		if ValidBoard(b) {
			t.Fatalf("받아들이면 안 되는 판 (%d자)", len(b))
		}
	}
}

func TestJudgeTimeLimit(t *testing.T) {
	cases := []struct {
		rule  string
		lines [2]int
		want  int
	}{
		{RuleSprint40, [2]int{20, 31}, 1},
		{RuleSprint40, [2]int{31, 20}, 0},
		{RuleSprint40, [2]int{25, 25}, -1},
		{RuleSurvival, [2]int{48, 12}, 0},
		{RuleSurvival, [2]int{3, 7}, 1},
		{RuleSurvival, [2]int{30, 30}, -1},
		{RuleSurvival, [2]int{0, 0}, -1},
		{RuleAttack, [2]int{30, 1}, -1},
		{RuleAttack, [2]int{1, 30}, -1},
		{RuleAttack, [2]int{10, 10}, -1},
	}
	for _, c := range cases {
		if w := judgeTimeLimit(c.rule, c.lines); w != c.want {
			t.Fatalf("judgeTimeLimit(%s, %v) = %d, want %d", c.rule, c.lines, w, c.want)
		}
	}
	spec, _ := LookupRule(RuleSprint40)
	if goalReached(spec, 39) || !goalReached(spec, 40) {
		t.Fatal("sprint40 목표 40줄")
	}
	attack, _ := LookupRule(RuleAttack)
	if goalReached(attack, 1000) {
		t.Fatal("attack 에는 목표 줄이 없다")
	}
	survival, _ := LookupRule(RuleSurvival)
	if goalReached(survival, 1000) {
		t.Fatal("survival 에는 목표 줄이 없다")
	}
}

func TestOnlyOpenRules(t *testing.T) {
	for _, id := range []string{"score120", "", "ATTACK", "Survival"} {
		if _, ok := LookupRule(id); ok {
			t.Fatalf("%q 는 열리면 안 된다", id)
		}
	}
	open := OpenRules()
	if len(open) != 3 || open[0] != RuleAttack || open[1] != RuleSprint40 || open[2] != RuleSurvival {
		t.Fatalf("OpenRules = %v", open)
	}
	for _, id := range open {
		if spec, ok := LookupRule(id); !ok || spec.ID != id {
			t.Fatalf("OpenRules 의 %q 를 LookupRule 이 거부했다", id)
		}
	}
	attack, _ := LookupRule(RuleAttack)
	if !attack.Garbage || attack.TimeLimitSec != 600 || attack.SpeedUp == nil || attack.SpeedUp.StartSec != 120 || attack.SpeedUp.EverySec != 30 {
		t.Fatalf("attack = %+v", attack)
	}
	sprint, _ := LookupRule(RuleSprint40)
	if sprint.Garbage || sprint.TimeLimitSec != 300 || sprint.GoalLines == nil || *sprint.GoalLines != 40 {
		t.Fatalf("sprint40 = %+v", sprint)
	}
	survival, ok := LookupRule(RuleSurvival)
	if !ok || survival.ID != "survival" || survival.Label != "생존전" || survival.Garbage || survival.GoalLines != nil ||
		survival.TimeLimitSec != 600 || survival.SpeedUp == nil || survival.SpeedUp.StartSec != 30 || survival.SpeedUp.EverySec != 20 {
		t.Fatalf("survival = %+v", survival)
	}
}

func TestCellBalanceSlackOnlyRaisesUpperBound(t *testing.T) {
	// 41 조각 = 164 칸: 여유 없으면 위반, 방금 보낸 방해 줄 1줄 여유면 통과.
	if CellBalanceWithin(41, 0, 0, 0) || !CellBalanceWithin(41, 0, 0, 1) {
		t.Fatal("위쪽 여유가 적용되지 않았다")
	}
	// 지운 줄 부풀리기(아래쪽 한도)는 여유와 무관하게 막는다.
	if CellBalanceWithin(1, 0, 4, 12) {
		t.Fatal("아래쪽 한도에 여유가 적용됐다")
	}
}
