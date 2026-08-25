package common

import (
	"testing"
	"time"
)

func TestParseLeaveDate(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0000-00-00", false},
		{"00000000", false},
		{"20260601", true},
		{"2026-06-01", true},
		{"garbage", false},
	}
	for _, c := range cases {
		if _, ok := ParseLeaveDate(c.in); ok != c.want {
			t.Errorf("ParseLeaveDate(%q) ok=%v, want %v", c.in, ok, c.want)
		}
	}
}

func TestClassifyWithdrawal(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)

	// 탈퇴 신청 아님
	if s, _ := ClassifyWithdrawal("", now); s != WithdrawalNone {
		t.Errorf("empty leave_date should be WithdrawalNone, got %v", s)
	}

	// ⛔ 2026-08-25 부터 숙려 없음(WithdrawalGraceDays=0). 신청 즉시 확정된다.

	// 오늘 신청 → 즉시 확정
	leaveToday := now.Format("20060102")
	if s, _ := ClassifyWithdrawal(leaveToday, now); s != WithdrawalConfirmed {
		t.Errorf("leave today should be WithdrawalConfirmed, got %v", s)
	}

	// 5일 전 신청 → 확정
	leave5 := now.AddDate(0, 0, -5).Format("20060102")
	if s, _ := ClassifyWithdrawal(leave5, now); s != WithdrawalConfirmed {
		t.Errorf("leave 5 days ago should be WithdrawalConfirmed, got %v", s)
	}

	// 29일 전 → 확정
	leave29 := now.AddDate(0, 0, -29).Format("20060102")
	if s, _ := ClassifyWithdrawal(leave29, now); s != WithdrawalConfirmed {
		t.Errorf("leave 29 days ago should be WithdrawalConfirmed, got %v", s)
	}

	// 숙려가 살아 있던 시절의 회귀 방지 — 어떤 과거 날짜도 Grace 가 되면 안 된다.
	if s, _ := ClassifyWithdrawal(leave5, now); s == WithdrawalGrace {
		t.Error("WithdrawalGrace must be unreachable while WithdrawalGraceDays==0")
	}

	// 40일 전 → 확정(경과)
	leave40 := now.AddDate(0, 0, -40).Format("20060102")
	if s, _ := ClassifyWithdrawal(leave40, now); s != WithdrawalConfirmed {
		t.Errorf("leave 40 days ago should be WithdrawalConfirmed, got %v", s)
	}
}

func TestIsWithdrawalAnonymized(t *testing.T) {
	if !IsWithdrawalAnonymized("탈퇴회원_123") {
		t.Error("탈퇴회원_123 should be detected as anonymized")
	}
	if !IsWithdrawalAnonymized("탈퇴사용자") {
		t.Error("탈퇴사용자 should be detected as anonymized")
	}
	if IsWithdrawalAnonymized("일반닉네임") {
		t.Error("일반닉네임 should NOT be detected as anonymized")
	}
}
