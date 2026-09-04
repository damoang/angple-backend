package middleware

import (
	"testing"
	"time"
)

func TestParseInterceptDate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(t *testing.T, got time.Time)
	}{
		{
			name:  "datetime format (future)",
			input: now.Add(2 * time.Hour).Format("2006-01-02 15:04:05"),
			checkFunc: func(t *testing.T, got time.Time) {
				diff := got.Sub(now.Add(2 * time.Hour))
				if diff < -2*time.Second || diff > 2*time.Second {
					t.Errorf("expected ~2h from now, got diff=%v", diff)
				}
			},
		},
		{
			name:  "datetime format (past)",
			input: now.Add(-1 * time.Hour).Format("2006-01-02 15:04:05"),
			checkFunc: func(t *testing.T, got time.Time) {
				if !now.After(got) {
					t.Error("expected past time to be before now")
				}
			},
		},
		{
			name:  "short YYYYMMDD format — end of day",
			input: now.Format("20060102"),
			checkFunc: func(t *testing.T, got time.Time) {
				// Should be 23:59:59 of that day
				if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
					t.Errorf("expected 23:59:59, got %s", got.Format("15:04:05"))
				}
			},
		},
		{
			name:  "dash YYYY-MM-DD format — end of day",
			input: now.Format("2006-01-02"),
			checkFunc: func(t *testing.T, got time.Time) {
				if got.Hour() != 23 || got.Minute() != 59 || got.Second() != 59 {
					t.Errorf("expected 23:59:59, got %s", got.Format("15:04:05"))
				}
			},
		},
		{
			name:  "permanent ban 99991231",
			input: "99991231",
			checkFunc: func(t *testing.T, got time.Time) {
				if got.Year() != 9999 {
					t.Errorf("expected year 9999, got %d", got.Year())
				}
			},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "garbage",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInterceptDate(tt.input)
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

func TestParseInterceptDateTimePrecision(t *testing.T) {
	// 핵심 시나리오: 오전 11시에 제재가 풀려야 하는데 YYYYMMDD로 저장하면 23:59:59까지 제재
	// datetime으로 저장하면 정확한 시간에 풀림

	// 시나리오: 3/18 11:00:01에 1일 제재 → 3/19 11:00:01에 만료
	banEnd := "2026-03-19 11:00:01"
	parsed, err := parseInterceptDate(banEnd)
	if err != nil {
		t.Fatalf("failed to parse datetime: %v", err)
	}

	// 3/19 10:00에는 아직 제재 중
	checkTime := time.Date(2026, 3, 19, 10, 0, 0, 0, banCheckKST)
	if checkTime.After(parsed) {
		t.Error("10:00 should still be banned, but After returned true")
	}

	// 3/19 11:01에는 제재 해제
	checkTime = time.Date(2026, 3, 19, 11, 1, 0, 0, banCheckKST)
	if !checkTime.After(parsed) {
		t.Error("11:01 should be unbanned, but After returned false")
	}

	// 대조: YYYYMMDD 형식이면 23:59:59까지 제재
	banEndShort := "20260319"
	parsedShort, err := parseInterceptDate(banEndShort)
	if err != nil {
		t.Fatalf("failed to parse short date: %v", err)
	}
	// 3/19 11:01에도 여전히 제재 중 (이것이 기존 버그)
	if checkTime.After(parsedShort) {
		t.Error("with YYYYMMDD format, 11:01 should still be banned until 23:59:59")
	}
}
