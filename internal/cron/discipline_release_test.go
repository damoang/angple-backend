package cron

import (
	"testing"
	"time"
)

func TestParseInterceptDateForCron(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(t *testing.T, got time.Time)
	}{
		{
			name:  "datetime format",
			input: "2026-03-19 11:00:01",
			checkFunc: func(t *testing.T, got time.Time) {
				expected := time.Date(2026, 3, 19, 11, 0, 1, 0, cronKST)
				if !got.Equal(expected) {
					t.Errorf("expected %v, got %v", expected, got)
				}
			},
		},
		{
			name:  "short YYYYMMDD — end of day",
			input: "20260319",
			checkFunc: func(t *testing.T, got time.Time) {
				if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
					t.Errorf("expected 23:59:59, got %s", got.Format("15:04:05"))
				}
			},
		},
		{
			name:  "dash format — end of day",
			input: "2026-03-19",
			checkFunc: func(t *testing.T, got time.Time) {
				if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
					t.Errorf("expected 23:59:59, got %s", got.Format("15:04:05"))
				}
			},
		},
		{
			name:    "invalid",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInterceptDateForCron(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, got)
			}
		})
	}
}

func TestCronExpiredDetection(t *testing.T) {
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, cronKST)

	// datetime 형식: 11시에 만료 → 12시 now 기준 만료됨
	banEnd, _ := parseInterceptDateForCron("2026-03-19 11:00:01")
	if !now.After(banEnd) {
		t.Error("datetime ban ending at 11:00:01 should be expired at 12:00:00")
	}

	// datetime 형식: 13시에 만료 → 아직 유효
	banEnd2, _ := parseInterceptDateForCron("2026-03-19 13:00:00")
	if now.After(banEnd2) {
		t.Error("datetime ban ending at 13:00:00 should NOT be expired at 12:00:00")
	}

	// YYYYMMDD 형식: 20260319 → 23:59:59까지 유효 → 12시에는 아직 유효
	banEnd3, _ := parseInterceptDateForCron("20260319")
	if now.After(banEnd3) {
		t.Error("YYYYMMDD ban for 20260319 should NOT be expired at 12:00:00 (valid until 23:59:59)")
	}

	// YYYYMMDD 형식: 20260318 → 어제 → 만료됨
	banEnd4, _ := parseInterceptDateForCron("20260318")
	if !now.After(banEnd4) {
		t.Error("YYYYMMDD ban for 20260318 should be expired at 2026-03-19 12:00:00")
	}
}

// TestShouldReleaseIntercept 는 2026-08-17 "하루 초과 구금" 회귀를 막는다.
//
// 사고: 노사모준(log4245) 5일 처분이 KST 08-16 23:05 에 끝났는데 08-17 00:00 크론이
// 풀지 않았다. mb_intercept_date 가 종료일보다 하루 뒤인 "20260817" 이었고,
// 크론이 그 값을 다시 "그 날 23:59:59 까지"로 해석해 가산이 두 번 일어났다.
// 회원이 "이용제한 시간이 지났는데 글을 쓸 수 없다"고 문의해 발견됐다.
func TestShouldReleaseIntercept(t *testing.T) {
	// 통보된 종료: 2026-08-16 23:05:15 (= penalty_date_from 08-11 23:05:15 + 5일)
	notifiedEnd := time.Date(2026, 8, 16, 23, 5, 15, 0, cronKST)
	justAfter := time.Date(2026, 8, 17, 0, 0, 14, 0, cronKST) // 실제 크론 실행 시각
	justBefore := time.Date(2026, 8, 16, 22, 0, 0, 0, cronKST)

	tests := []struct {
		name          string
		now           time.Time
		interceptDate string
		authEnd       time.Time
		hasAuth       bool
		want          bool
	}{
		{
			// ⭐ 회귀 케이스 — 파생값은 하루 뒤(20260817)지만 정본으로 풀어야 한다.
			name:          "정본이 만료됐으면 파생값이 하루 뒤여도 해제",
			now:           justAfter,
			interceptDate: "20260817",
			authEnd:       notifiedEnd,
			hasAuth:       true,
			want:          true,
		},
		{
			name:          "정본이 아직 안 끝났으면 유지",
			now:           justBefore,
			interceptDate: "20260817",
			authEnd:       notifiedEnd,
			hasAuth:       true,
			want:          false,
		},
		{
			name:          "정본 없으면 파생값으로 판정 — 당일은 유지",
			now:           justAfter,
			interceptDate: "20260817",
			hasAuth:       false,
			want:          false,
		},
		{
			name:          "정본 없고 파생값이 어제면 해제",
			now:           justAfter,
			interceptDate: "20260816",
			hasAuth:       false,
			want:          true,
		},
		{
			name:          "형식 불명은 풀지 않는다",
			now:           justAfter,
			interceptDate: "not-a-date",
			hasAuth:       false,
			want:          false,
		},
		{
			name:          "영구(99991231)는 풀리지 않는다",
			now:           justAfter,
			interceptDate: "99991231",
			hasAuth:       false,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldReleaseIntercept(tt.now, tt.interceptDate, tt.authEnd, tt.hasAuth)
			if got != tt.want {
				t.Errorf("shouldReleaseIntercept(now=%s, intercept=%q, auth=%v/%t) = %t, want %t",
					tt.now.Format("2006-01-02 15:04:05"), tt.interceptDate,
					tt.authEnd.Format("2006-01-02 15:04:05"), tt.hasAuth, got, tt.want)
			}
		})
	}
}
