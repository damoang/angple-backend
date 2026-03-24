package giving

import (
	"testing"
	"time"
)

func TestNormalizeStatuses(t *testing.T) {
	now := time.Date(2026, 3, 24, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))

	tests := []struct {
		name string
		meta Meta
		want Status
	}{
		{
			name: "active",
			meta: Meta{StartRaw: "2026-03-24T11:00", EndRaw: "2026-03-24T13:00"},
			want: StatusActive,
		},
		{
			name: "waiting",
			meta: Meta{StartRaw: "2026-03-24T13:00", EndRaw: "2026-03-24T15:00"},
			want: StatusWaiting,
		},
		{
			name: "paused",
			meta: Meta{StartRaw: "2026-03-24T11:00", EndRaw: "2026-03-24T13:00", StateRaw: "1"},
			want: StatusPaused,
		},
		{
			name: "ended by time",
			meta: Meta{StartRaw: "2026-03-24T09:00", EndRaw: "2026-03-24T11:00"},
			want: StatusEnded,
		},
		{
			name: "ended by force",
			meta: Meta{StartRaw: "2026-03-24T13:00", EndRaw: "2026-03-24T15:00", StateRaw: "2"},
			want: StatusEnded,
		},
		{
			name: "no giving without schedule",
			meta: Meta{},
			want: StatusNoGiving,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(now, tt.meta)
			if got.Status != tt.want {
				t.Fatalf("status = %s, want %s", got.Status, tt.want)
			}
		})
	}
}

func TestNormalizeUrgentWindow(t *testing.T) {
	now := time.Date(2026, 3, 24, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))

	got := Normalize(now, Meta{
		StartRaw: "2026-03-24T11:00",
		EndRaw:   "2026-03-24T23:00",
	})

	if !got.IsUrgent {
		t.Fatalf("expected active giving within 24h to be urgent")
	}
}
