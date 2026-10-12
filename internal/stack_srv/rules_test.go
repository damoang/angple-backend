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
		rule   string
		lines  [2]int
		scores [2]int64
		want   int
	}{
		{RuleSprint40, [2]int{20, 31}, [2]int64{9000, 0}, 1},
		{RuleSprint40, [2]int{31, 20}, [2]int64{0, 9000}, 0},
		{RuleSprint40, [2]int{25, 25}, [2]int64{100, 0}, -1},
		{RuleSurvival, [2]int{48, 12}, [2]int64{0, 9000}, 0},
		{RuleSurvival, [2]int{3, 7}, [2]int64{9000, 0}, 1},
		{RuleSurvival, [2]int{30, 30}, [2]int64{0, 100}, -1},
		{RuleSurvival, [2]int{0, 0}, [2]int64{0, 0}, -1},
		{RuleAttack, [2]int{30, 1}, [2]int64{9000, 0}, -1},
		{RuleAttack, [2]int{1, 30}, [2]int64{0, 9000}, -1},
		{RuleAttack, [2]int{10, 10}, [2]int64{0, 0}, -1},
		// score120 은 줄이 아니라 점수로 판정한다.
		{RuleScore120, [2]int{2, 30}, [2]int64{5000, 4999}, 0},
		{RuleScore120, [2]int{30, 2}, [2]int64{1200, 3400}, 1},
		{RuleScore120, [2]int{30, 2}, [2]int64{2500, 2500}, -1},
		{RuleScore120, [2]int{0, 0}, [2]int64{0, 0}, -1},
		{"score60", [2]int{30, 2}, [2]int64{9000, 0}, -1},
	}
	for _, c := range cases {
		if w := judgeTimeLimit(c.rule, c.lines, c.scores); w != c.want {
			t.Fatalf("judgeTimeLimit(%s, %v, %v) = %d, want %d", c.rule, c.lines, c.scores, w, c.want)
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
	score120, _ := LookupRule(RuleScore120)
	if goalReached(score120, 1000) {
		t.Fatal("score120 에는 목표 줄이 없다")
	}
}

func TestOnlyOpenRules(t *testing.T) {
	for _, id := range []string{"score60", "", "ATTACK", "Survival", "SCORE120"} {
		if _, ok := LookupRule(id); ok {
			t.Fatalf("%q 는 열리면 안 된다", id)
		}
	}
	open := OpenRules()
	if len(open) != 4 || open[0] != RuleAttack || open[1] != RuleSprint40 || open[2] != RuleSurvival || open[3] != RuleScore120 {
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
	score120, ok := LookupRule(RuleScore120)
	if !ok || score120.ID != "score120" || score120.Label != "2분 점수전" || score120.Garbage || score120.GoalLines != nil ||
		score120.TimeLimitSec != 120 || score120.SpeedUp != nil {
		t.Fatalf("score120 = %+v", score120)
	}
}

func TestScoreLockLineUsesLevelBeforeLock(t *testing.T) {
	cases := []struct {
		lineScore   int64
		linesBefore int
		cleared     int
		want        int64
	}{
		{0, 0, 0, 0},
		{0, 0, 1, 100},
		{0, 0, 4, 700},
		// 7줄에서 4줄을 지우면 지우는 순간 레벨은 아직 1이다(웹 엔진: 점수 가산 후 레벨 갱신).
		{1000, 7, 4, 1700},
		// 8줄부터 레벨 2.
		{1000, 8, 1, 1200},
		{1000, 8, 4, 2400},
		{0, 15, 2, 500},
		{0, 16, 3, 1350},
		// 범위 밖 cleared 는 0점.
		{500, 0, 5, 500},
	}
	for _, c := range cases {
		if got := scoreLockLine(c.lineScore, c.linesBefore, c.cleared); got != c.want {
			t.Fatalf("scoreLockLine(%d, %d, %d) = %d, want %d", c.lineScore, c.linesBefore, c.cleared, got, c.want)
		}
	}
	// 웹 엔진 순서로 한 판을 다시 계산한 값과 같아야 한다: 4·4·1·3 줄.
	var lineScore int64
	lines := 0
	for _, n := range []int{4, 4, 1, 3} {
		lineScore = scoreLockLine(lineScore, lines, n)
		lines += n
	}
	// 4(L1)=700, 4(L1)=700, 1(L2)=200, 3(L2)=900.
	if lineScore != 2500 {
		t.Fatalf("누적 줄 점수 = %d, want 2500", lineScore)
	}
}

func TestJudgedScoreClampsDrop(t *testing.T) {
	dropCap := func(locks int) int64 { return 2 * (Rows + 12) * int64(locks+1) }
	cases := []struct {
		name        string
		lineScore   int64
		clientScore int64
		locks       int
		want        int64
	}{
		{"정상(낙하 점수 상한 안)", 700, 760, 10, 760},
		{"줄 점수만", 700, 700, 10, 700},
		{"부풀림은 상한까지 잘림", 700, 1000000, 10, 700 + dropCap(10)},
		{"상한과 같으면 그대로", 700, 700 + dropCap(10), 10, 700 + dropCap(10)},
		{"음수 낙하는 0", 1400, 0, 5, 1400},
		{"줄 점수보다 작으면 줄 점수", 1400, 1399, 5, 1400},
		{"첫 lock 전", 0, 0, 0, 0},
		{"첫 lock 전 부풀림", 0, 99999, 0, dropCap(0)},
	}
	for _, c := range cases {
		if got := judgedScore(c.lineScore, c.clientScore, c.locks); got != c.want {
			t.Fatalf("%s: judgedScore(%d, %d, %d) = %d, want %d", c.name, c.lineScore, c.clientScore, c.locks, got, c.want)
		}
	}
	if dropCap(10) != 660 {
		t.Fatalf("상한 식이 혼자하기 drop_cap 과 다르다: %d", dropCap(10))
	}
	if dropReason(dropCap(10), 10) != "" || dropReason(dropCap(10)+1, 10) != "drop_cap" {
		t.Fatal("judgedScore 상한은 혼자하기 dropReason 경계와 같아야 한다")
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
