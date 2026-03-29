package v2

import (
	"testing"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
)

func TestNormalizeRecurringDateValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "single day", input: "03-01", want: "03-01"},
		{name: "range", input: "03-20 ~ 04-02", want: "03-20~04-02"},
		{name: "invalid day", input: "02-30", wantErr: true},
		{name: "invalid format", input: "2026-03-01", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRecurringDateValue(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestResolveActiveLogoFromSchedules(t *testing.T) {
	t.Run("recurring range wins over default", func(t *testing.T) {
		seasonRange := "03-20~04-02"
		schedules := []*v2domain.SiteLogo{
			{ID: 1, Name: "Default", ScheduleType: "default", Priority: 0, IsActive: true},
			{ID: 2, Name: "Spring", ScheduleType: "recurring", RecurringDate: &seasonRange, Priority: 10, IsActive: true},
		}

		active := resolveActiveLogoFromSchedules(schedules, time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
		if active == nil || active.ID != 2 {
			t.Fatalf("expected recurring range logo to be active, got %#v", active)
		}
	})

	t.Run("wraparound recurring range matches january dates", func(t *testing.T) {
		winterRange := "12-20~01-10"
		schedules := []*v2domain.SiteLogo{
			{ID: 1, Name: "Default", ScheduleType: "default", Priority: 0, IsActive: true},
			{ID: 2, Name: "Winter", ScheduleType: "recurring", RecurringDate: &winterRange, Priority: 10, IsActive: true},
		}

		active := resolveActiveLogoFromSchedules(schedules, time.Date(2027, 1, 5, 10, 0, 0, 0, time.UTC))
		if active == nil || active.ID != 2 {
			t.Fatalf("expected wraparound recurring logo to be active, got %#v", active)
		}
	})

	t.Run("date range matches single day event", func(t *testing.T) {
		start := "2026-04-16"
		end := "2026-04-16"
		schedules := []*v2domain.SiteLogo{
			{ID: 1, Name: "Default", ScheduleType: "default", Priority: 0, IsActive: true},
			{ID: 3, Name: "Memorial", ScheduleType: "date_range", StartDate: &start, EndDate: &end, Priority: 20, IsActive: true},
		}

		active := resolveActiveLogoFromSchedules(schedules, time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC))
		if active == nil || active.ID != 3 {
			t.Fatalf("expected one-day date range logo to be active, got %#v", active)
		}
	})
}
