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
	if w := judgeTimeLimit(RuleSprint40, [2]int{20, 31}); w != 1 {
		t.Fatalf("sprint40 줄 많은 쪽 = %d", w)
	}
	if w := judgeTimeLimit(RuleSprint40, [2]int{25, 25}); w != -1 {
		t.Fatalf("sprint40 동률 = %d", w)
	}
	if w := judgeTimeLimit(RuleAttack, [2]int{30, 1}); w != -1 {
		t.Fatalf("attack 10분 = %d (무승부여야 한다)", w)
	}
	spec, _ := LookupRule(RuleSprint40)
	if goalReached(spec, 39) || !goalReached(spec, 40) {
		t.Fatal("sprint40 목표 40줄")
	}
	attack, _ := LookupRule(RuleAttack)
	if goalReached(attack, 1000) {
		t.Fatal("attack 에는 목표 줄이 없다")
	}
}

func TestOnlyV1RulesOpen(t *testing.T) {
	for _, id := range []string{"score120", "survival", "", "ATTACK"} {
		if _, ok := LookupRule(id); ok {
			t.Fatalf("%q 는 v1 에서 열리면 안 된다", id)
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
}
