package common

import (
	"errors"
	"strings"
	"time"
)

// WithdrawalGraceDays is the length of the self-withdrawal grace period.
// 회원이 탈퇴를 신청한 뒤 이 기간 안에는 본인이 재로그인하여 탈퇴를 취소할 수 있다.
// 기간이 경과하면 cron 이 확정 익명화한다. (mb_leave_date 를 "숙려 시작일"로 재사용)
const WithdrawalGraceDays = 30

// WithdrawalAnonymizedNickPrefix — 확정 익명화 시 mb_nick 에 부여되는 접두사.
// cron 멱등성 판정과 취소 불가(이미 익명화) 판정에 사용한다.
const WithdrawalAnonymizedNickPrefix = "탈퇴"

// ErrAccountWithdrawn 는 숙려기간이 경과하여 탈퇴가 확정(익명화)된 계정의 로그인 시도에 반환한다.
var ErrAccountWithdrawn = errors.New("account withdrawn")

// WithdrawalState 는 mb_leave_date 플래그로부터 계산한 회원 탈퇴 상태다.
type WithdrawalState int

const (
	// WithdrawalNone — 탈퇴 신청 상태가 아님(정상 회원).
	WithdrawalNone WithdrawalState = iota
	// WithdrawalGrace — 숙려중. 30일 미경과, 본인 취소 가능.
	WithdrawalGrace
	// WithdrawalConfirmed — 숙려기간 경과. 확정(익명화 대상). 로그인/복구 불가.
	WithdrawalConfirmed
)

// ParseLeaveDate 는 g5_member.mb_leave_date 를 파싱한다.
// 빈 값/제로값이면 ok=false(탈퇴 신청 아님). admin/self 로직은 "20060102" 로 저장하지만
// 혼재 형식("2006-01-02")도 관대하게 허용한다.
func ParseLeaveDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0000-00-00" || s == "00000000" {
		return time.Time{}, false
	}
	for _, f := range []string{"20060102", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// WithdrawalGraceDeadline 은 숙려기간이 끝나는 순간(신청일 + 30일)을 돌려준다.
func WithdrawalGraceDeadline(leave time.Time) time.Time {
	return leave.AddDate(0, 0, WithdrawalGraceDays)
}

// ClassifyWithdrawal 는 mb_leave_date 와 기준 시각으로 탈퇴 상태와 숙려 만료 시각을 계산한다.
// now < deadline 이면 WithdrawalGrace(취소 가능), 그 외에는 WithdrawalConfirmed(확정).
func ClassifyWithdrawal(leaveDate string, now time.Time) (WithdrawalState, time.Time) {
	leave, ok := ParseLeaveDate(leaveDate)
	if !ok {
		return WithdrawalNone, time.Time{}
	}
	deadline := WithdrawalGraceDeadline(leave)
	if now.Before(deadline) {
		return WithdrawalGrace, deadline
	}
	return WithdrawalConfirmed, deadline
}

// IsWithdrawalAnonymized 는 mb_nick 이 이미 확정 익명화된 닉네임인지 판정한다(멱등성용).
func IsWithdrawalAnonymized(mbNick string) bool {
	return strings.HasPrefix(strings.TrimSpace(mbNick), WithdrawalAnonymizedNickPrefix)
}
