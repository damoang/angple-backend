package dantry

import "time"

// Error represents a single JavaScript error from ClickHouse
type Error struct {
	ID              string    `json:"id"`
	MemberID        string    `json:"member_id"`
	Timestamp       time.Time `json:"timestamp"`
	Type            string    `json:"type"`
	Message         string    `json:"message"`
	Source          string    `json:"source"`
	Lineno          uint32    `json:"lineno"`
	Colno           uint32    `json:"colno"`
	Stack           string    `json:"stack"`
	URL             string    `json:"url"`
	UserAgent       string    `json:"user_agent"`
	IsScriptBlocked uint8     `json:"is_script_blocked"`
	IsMobile        uint8     `json:"is_mobile"`
}

// ErrorGroup represents errors grouped by message
type ErrorGroup struct {
	Message       string    `json:"message"`
	Type          string    `json:"type"`
	Count         uint64    `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	SampleSource  string    `json:"sample_source"`
	SampleURL     string    `json:"sample_url"`
	SampleStack   string    `json:"sample_stack"`
	UniqueURLs    uint64    `json:"unique_urls"`
	UniqueMembers uint64    `json:"unique_members"`
}

// Stats represents summary statistics
type Stats struct {
	TodayCount    uint64  `json:"today_count"`
	WeekCount     uint64  `json:"week_count"`
	UniqueErrors  uint64  `json:"unique_errors"`
	MobileRatio   float64 `json:"mobile_ratio"`
	TopError      string  `json:"top_error,omitempty"`
	TopErrorCount uint64  `json:"top_error_count,omitempty"`
}

// TimeBucket represents a time series data point
type TimeBucket struct {
	Bucket time.Time `json:"bucket"`
	Count  uint64    `json:"count"`
}

// ErrorMember represents a member who triggered a specific error
type ErrorMember struct {
	MemberID  string    `json:"member_id"`
	Count     uint64    `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	SampleURL string    `json:"sample_url"`
}
