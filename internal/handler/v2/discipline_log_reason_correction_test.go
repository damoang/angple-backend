package v2

import (
	"encoding/json"
	"strings"
	"testing"
)

// 사유 정정 이력을 회원에게 내릴 때 지켜야 할 것:
//  ① 운영자 ID·내부 메모가 새지 않는다
//  ② 구 코드(1-18)와 현행 코드(21-38)가 섞여도 "뜻이 같은 변경"을 변경으로 세지 않는다
//  ③ 무엇이 빠졌는지가 실제로 보인다 (감추면 소명이 반영됐는지 알 수 없다)

func TestBuildReasonCorrections_제외된_사유가_보인다(t *testing.T) {
	out := buildReasonCorrections([]ReasonHistoryEntry{
		{
			At:      "2026-08-13 20:00:00",
			By:      "sdk",
			From:    []int{21, 36, 25},
			To:      []int{21, 25},
			Memo:    "내부 판단 근거",
			ClaimID: 1738,
		},
	})

	if len(out) != 1 {
		t.Fatalf("정정 1건이 나와야 한다: %v", out)
	}
	if len(out[0].Removed) != 1 || out[0].Removed[0] != "운영정책부정" {
		t.Errorf("제외된 사유가 틀리다: %v", out[0].Removed)
	}
	if len(out[0].Added) != 0 {
		t.Errorf("추가된 사유가 없어야 한다: %v", out[0].Added)
	}
	if out[0].ClaimID != 1738 {
		t.Errorf("소명 번호가 유실됐다: %d", out[0].ClaimID)
	}
}

// ⛔ 이 테스트가 깨지면 회원 화면으로 운영 내부 정보가 나간 것이다.
func TestBuildReasonCorrections_운영자와_내부메모는_나가지_않는다(t *testing.T) {
	out := buildReasonCorrections([]ReasonHistoryEntry{
		{At: "2026-08-13 20:00:00", By: "sdk", From: []int{21, 36}, To: []int{21}, Memo: "다중이 물증 없음"},
	})
	if len(out) != 1 {
		t.Fatalf("정정 1건이 나와야 한다")
	}
	if out[0].At == "" {
		t.Errorf("시점은 남아야 한다")
	}

	// 실제로 응답에 실릴 JSON 을 검사한다. 필드를 새로 달면 여기서 걸린다.
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("직렬화 실패: %v", err)
	}
	for _, leak := range []string{"sdk", "다중이 물증 없음", "\"by\"", "\"memo\""} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("회원 응답에 %q 가 새어 나왔다:\n%s", leak, raw)
		}
	}
}

func TestBuildReasonCorrections_구코드와_현행코드가_섞여도_변경이_아니다(t *testing.T) {
	// 16(구) → 36(현행) 은 같은 "운영정책부정"이다. 변경으로 세면 안 된다.
	out := buildReasonCorrections([]ReasonHistoryEntry{
		{At: "2026-08-13 20:00:00", From: []int{1, 16}, To: []int{21, 36}},
	})
	if out != nil {
		t.Errorf("코드 정리만 한 건은 정정으로 보이지 않아야 한다: %v", out)
	}
}

func TestBuildReasonCorrections_추가도_잡는다(t *testing.T) {
	out := buildReasonCorrections([]ReasonHistoryEntry{
		{At: "2026-08-14 10:00:00", From: []int{21}, To: []int{21, 25}},
	})
	if len(out) != 1 || len(out[0].Added) != 1 || out[0].Added[0] != "분란유도/갈등조장" {
		t.Fatalf("추가된 사유가 잡히지 않았다: %v", out)
	}
	if len(out[0].Removed) != 0 {
		t.Errorf("제외가 없어야 한다: %v", out[0].Removed)
	}
}

func TestBuildReasonCorrections_이력이_없으면_nil(t *testing.T) {
	if got := buildReasonCorrections(nil); got != nil {
		t.Errorf("nil 이어야 한다(omitempty 로 키가 빠진다): %v", got)
	}
}

// 이름을 모르는 코드는 숫자로 노출하지 않고 버린다.
func TestDiffViolationTitles_미상_코드는_버린다(t *testing.T) {
	got := diffViolationTitles([]int{21, 999}, []int{})
	if len(got) != 1 || got[0] != "회원비하" {
		t.Errorf("미상 코드가 새어 나왔다: %v", got)
	}
}

// 같은 사유가 두 번 들어 있어도 한 번만 보여준다.
func TestDiffViolationTitles_중복은_한번만(t *testing.T) {
	got := diffViolationTitles([]int{16, 36}, []int{})
	if len(got) != 1 {
		t.Errorf("중복이 그대로 나왔다: %v", got)
	}
}

// 41(부적절한 닉네임)은 운영 콘솔이 제공하는 사유인데 표에 없었다.
// 없으면 removed·added 가 모두 비어 **정정 항목 자체가 사라지고**,
// 회원은 소명이 반영된 걸 볼 수 없다.
func TestBuildReasonCorrections_코드41_도_보인다(t *testing.T) {
	out := buildReasonCorrections([]ReasonHistoryEntry{
		{At: "2026-08-13 20:00:00", From: []int{41}, To: []int{}},
	})
	if len(out) != 1 {
		t.Fatalf("정정이 통째로 사라졌다: %v", out)
	}
	if len(out[0].Removed) != 1 || out[0].Removed[0] != "부적절한 닉네임" {
		t.Errorf("41 이 이름 없이 버려졌다: %v", out[0].Removed)
	}
}
