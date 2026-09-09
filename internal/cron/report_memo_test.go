package cron

import (
	"strings"
	"testing"
	"time"
)

// 주의(0일)와 이용제한(1일 이상)이 서로 다른 안내문을 쓰는지 확인한다.
// 종전에는 안내문이 하나뿐이라 주의를 받은 회원에게도 "글쓰기, 댓글, 쪽지 기능이
// 잠시 쉬어갑니다" 가 나갔다. 받은 사람은 제한이 없는데도 글을 쓰지 않고 기다린다.
func TestBuildMemoContent_주의는_이용제한_문구를_쓰지_않는다(t *testing.T) {
	now := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)
	memo := buildMemoContent("google_1234", "닉네임", 0, "level", []int{22}, "", 4640, now)

	if !strings.Contains(memo, "[주의 안내]") {
		t.Errorf("주의 안내 머리말이 없다:\n%s", memo)
	}
	if strings.Contains(memo, "쉬어가") {
		t.Errorf("주의인데 쉬어가기 문구가 들어 있다:\n%s", memo)
	}
	if !strings.Contains(memo, "그대로 이용하실 수 있어요") {
		t.Errorf("이용에 제한이 없다는 안내가 없다:\n%s", memo)
	}
	if !strings.Contains(memo, "• 구분: 주의(이용제한 없음)") {
		t.Errorf("구분 표기가 다르다:\n%s", memo)
	}
	// 종료일은 붙지 않는다 — 주의는 기간이 없다
	if strings.Contains(memo, " ~ ") {
		t.Errorf("주의에 종료일이 붙었다:\n%s", memo)
	}
}

func TestBuildMemoContent_이용제한은_기간과_종료일을_보여준다(t *testing.T) {
	now := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)
	memo := buildMemoContent("google_1234", "닉네임", 5, "level", []int{22}, "", 4640, now)

	if !strings.Contains(memo, "[잠시 쉬어가기 안내]") {
		t.Errorf("쉬어가기 머리말이 없다:\n%s", memo)
	}
	if !strings.Contains(memo, "• 기간: 5일 ~ 2026-09-14 21:30:00") {
		t.Errorf("기간·종료일 표기가 다르다:\n%s", memo)
	}
	if !strings.Contains(memo, "잠시 쉬어갑니다") {
		t.Errorf("이용제한 안내 문구가 없다:\n%s", memo)
	}
	if strings.Contains(memo, "[주의 안내]") {
		t.Errorf("이용제한인데 주의 머리말이 들어 있다:\n%s", memo)
	}
}

func TestBuildMemoContent_영구는_기간이_영구로_나온다(t *testing.T) {
	now := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)
	for _, days := range []int{9999, -1} {
		memo := buildMemoContent("google_1234", "닉네임", days, "level", []int{22}, "", 4640, now)
		if !strings.Contains(memo, "• 기간: 영구 ~") {
			t.Errorf("days=%d 에서 영구 표기가 다르다:\n%s", days, memo)
		}
		if strings.Contains(memo, "[주의 안내]") {
			t.Errorf("days=%d 인데 주의 머리말이 들어 있다", days)
		}
	}
}

// 안내문은 누구에게나 같은 문구로 나가야 한다. 닉네임을 사유로 안내할 때
// 그 닉네임을 본문에서 부르면 안내문이 조롱처럼 읽힌다.
func TestBuildMemoContent_본문에_닉네임을_넣지_않는다(t *testing.T) {
	now := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)
	nick := "부적절한닉네임"
	for _, days := range []int{0, 5, 9999} {
		memo := buildMemoContent("google_1234", nick, days, "level", []int{22}, "", 4640, now)
		if strings.Contains(memo, nick) {
			t.Errorf("days=%d 안내문에 닉네임이 들어 있다:\n%s", days, memo)
		}
		if !strings.Contains(memo, "안녕하세요, 회원님! 👋") {
			t.Errorf("days=%d 호칭이 회원님이 아니다:\n%s", days, memo)
		}
	}
}
