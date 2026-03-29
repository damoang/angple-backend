package giving

import (
	"fmt"
	"time"
)

var seoulLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.FixedZone("KST", 9*60*60)
	}
	return loc
}()

type Status string

const (
	StatusActive   Status = "active"
	StatusWaiting  Status = "waiting"
	StatusPaused   Status = "paused"
	StatusEnded    Status = "ended"
	StatusNoGiving Status = "no_giving"
)

type Meta struct {
	StartRaw         string
	EndRaw           string
	StateRaw         string
	ParticipantCount int
}

type Normalized struct {
	GivingStart      string
	GivingEnd        string
	Status           Status
	ParticipantCount int
	IsPaused         bool
	IsUrgent         bool
}

// ParseTime parses the time formats currently stored in giving posts.
func ParseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, seoulLocation); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

func Normalize(now time.Time, meta Meta) Normalized {
	normalized := Normalized{
		GivingStart:      meta.StartRaw,
		GivingEnd:        meta.EndRaw,
		ParticipantCount: meta.ParticipantCount,
		Status:           StatusNoGiving,
	}

	startTime, startErr := ParseTime(meta.StartRaw)
	endTime, endErr := ParseTime(meta.EndRaw)
	hasSchedule := meta.StartRaw != "" && meta.EndRaw != "" && startErr == nil && endErr == nil

	switch {
	case meta.StateRaw == "2":
		normalized.Status = StatusEnded
	case meta.StateRaw == "1" && hasSchedule:
		normalized.Status = StatusPaused
		normalized.IsPaused = true
	case !hasSchedule:
		normalized.Status = StatusNoGiving
	case now.After(endTime):
		normalized.Status = StatusEnded
	case !now.Before(startTime):
		normalized.Status = StatusActive
	default:
		normalized.Status = StatusWaiting
	}

	if normalized.Status == StatusActive {
		diff := endTime.Sub(now)
		normalized.IsUrgent = diff > 0 && diff <= 24*time.Hour
	}

	return normalized
}
