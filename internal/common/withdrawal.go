package common

import (
	"errors"
	"strings"
	"time"
)

// WithdrawalGraceDays is the length of the self-withdrawal grace period.
//
// ⛔ 2026-08-25 정책 변경으로 0 이다. 탈퇴는 신청 즉시 확정되고 본인 취소 경로는 없다.
//
// 왜 상수만 0 으로 두고 WithdrawalGrace 분기를 지우지 않았나 —
// 그 분기는 로그인 경로(v2/auth_service, v2/auth_handler) 한복판에 있다.
// 지금 걷어내면 정상 회원 로그인까지 건드리게 되므로, 도달 불가 상태로 두고
// 별도 정리한다. ClassifyWithdrawal 이 WithdrawalGrace 를 반환하지 않으므로
// 동작상 차이는 없다.
//
// ⛔ 되돌리려면 이 값만 30 으로 올리면 된다. 그 경우 방침 제2조③도 함께 되돌려야 한다.
const WithdrawalGraceDays = 0

// WithdrawalAnonymizedNickPrefix — 확정 익명화 시 mb_nick 에 부여되는 접두사.
// cron 멱등성 판정과 취소 불가(이미 익명화) 판정에 사용한다.
const WithdrawalAnonymizedNickPrefix = "탈퇴"

// ErrAccountWithdrawn 는 탈퇴가 확정된 계정의 로그인 시도에 반환한다.
// ⛔ 2026-08-25 부터 탈퇴 신청 즉시 확정이므로, 신청한 그 순간부터 로그인이 거부된다.
var ErrAccountWithdrawn = errors.New("account withdrawn")

// WithdrawalState 는 mb_leave_date 플래그로부터 계산한 회원 탈퇴 상태다.
type WithdrawalState int

const (
	// WithdrawalNone — 탈퇴 신청 상태가 아님(정상 회원).
	WithdrawalNone WithdrawalState = iota
	// WithdrawalGrace — 숙려중(본인 취소 가능).
	// ⛔ WithdrawalGraceDays=0 이므로 현재 ClassifyWithdrawal 은 이 값을 반환하지 않는다.
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

// WithdrawalGraceDeadline 은 탈퇴가 확정되는 순간(신청일 + WithdrawalGraceDays)을 돌려준다.
func WithdrawalGraceDeadline(leave time.Time) time.Time {
	return leave.AddDate(0, 0, WithdrawalGraceDays)
}

// ClassifyWithdrawal 는 mb_leave_date 와 기준 시각으로 탈퇴 상태와 확정 시각을 계산한다.
// now < deadline 이면 WithdrawalGrace, 그 외에는 WithdrawalConfirmed(확정).
// ⛔ WithdrawalGraceDays=0 이라 deadline 은 신청일 자정 = 항상 과거다. 즉 신청 즉시 확정된다.
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
