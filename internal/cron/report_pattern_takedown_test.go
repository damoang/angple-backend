package cron

import (
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	tdRights    = "rights_infringement"
	tdElection  = "election_commission"
	tdKisa      = "kisa"
	tdCopyright = "copyright_infringement"
	tdSince     = "2026-02-26"
)

func openTakedownTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sqlite: %v", err)
		}
	})
	return db
}

func seedTakedownRequests(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE TABLE g5_takedown_requests (
		id INTEGER PRIMARY KEY,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		received_date TEXT NOT NULL,
		processed_at TEXT NULL
	)`).Error; err != nil {
		t.Fatalf("create g5_takedown_requests: %v", err)
	}
	// 보고 기간: 2026-09-27(일) ~ 2026-10-03(토), processed_at 은 KST 벽시계 값
	if err := db.Exec(`
		INSERT INTO g5_takedown_requests (id, type, status, received_date, processed_at) VALUES
		(1, 'rights_infringement', 'completed', '2026-09-26', '2026-09-26 23:59:59.999'),
		(2, 'rights_infringement', 'completed', '2026-09-27', '2026-09-27 00:00:00.000'),
		(3, 'election_commission', 'completed', '2026-10-03', '2026-10-03 23:59:59.500'),
		(4, 'election_commission', 'completed', '2026-10-04', '2026-10-04 00:00:00.000'),
		(5, 'kisa', 'completed', '2026-09-30', NULL),
		(6, 'kisa', 'completed', '2026-09-26', NULL),
		(7, 'rights_infringement', 'received', '2026-01-01', NULL),
		(8, 'adsense_policy', 'rejected', '2026-09-29', '2026-09-29 10:00:00.000'),
		(9, 'copyright_infringement', 'completed', '2026-02-26', '2026-02-27 11:00:00.000'),
		(10, 'election_commission', 'completed', '2026-09-27', '2026-09-27 08:30:00.000')
	`).Error; err != nil {
		t.Fatalf("seed g5_takedown_requests: %v", err)
	}
}

func assertCounts(t *testing.T, label string, got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", label, got, want)
		return
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s[%s]: got %d, want %d (all=%v)", label, k, got[k], v, got)
		}
	}
}

// 기간 경계는 [시작일 00:00, 종료 다음날 00:00) 이고, processed_at 은 KST 벽시계
// 그대로 비교한다. processed_at 이 없는 완료 건은 received_date 로 판정한다.
func TestCollectTakedownStats_기간경계와_NULL대체(t *testing.T) {
	db := openTakedownTestDB(t, "takedown_boundary")
	seedTakedownRequests(t, db)

	ts, err := collectTakedownStats(db, "2026-09-27", "2026-10-03")
	if err != nil {
		t.Fatalf("collectTakedownStats: %v", err)
	}

	// 1: 시작 1ms 전 → 제외 / 2: 시작 정각 → 포함 / 3: 종료일 23:59:59.5 → 포함
	// 4: 종료 다음날 정각 → 제외 / 5: NULL, 기간 안 received → 포함 / 6: NULL, 기간 밖 → 제외
	// 7·8: 완료 아님 → 제외 / 10: 일요일 아침(KST) → 포함
	assertCounts(t, "week", ts.Week, map[string]int{
		tdRights:   1,
		tdElection: 2,
		tdKisa:     1,
	})
	assertCounts(t, "total", ts.Total, map[string]int{
		tdRights:    2,
		tdElection:  3,
		tdKisa:      2,
		tdCopyright: 1,
	})
	// 완료 건 중 가장 이른 received_date (완료 아닌 7번의 2026-01-01 은 무시)
	if ts.Since != tdSince {
		t.Errorf("since: got %q, want 2026-02-26", ts.Since)
	}

	line := takedownSummaryLine(ts)
	if !strings.Contains(line, "외부 요청 처리: 이번 주 4건 (누적 8건)") {
		t.Errorf("summary line: got %q", line)
	}
}

func TestCollectTakedownStats_빈기간은_빈맵(t *testing.T) {
	db := openTakedownTestDB(t, "takedown_empty_week")
	seedTakedownRequests(t, db)

	ts, err := collectTakedownStats(db, "2026-08-02", "2026-08-08")
	if err != nil {
		t.Fatalf("collectTakedownStats: %v", err)
	}
	if ts.Week == nil || len(ts.Week) != 0 {
		t.Errorf("week: got %v, want empty map", ts.Week)
	}
	if len(ts.Total) == 0 {
		t.Errorf("total should not be empty")
	}

	raw, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"week":{}`) {
		t.Errorf("empty week should serialize as {}: %s", raw)
	}
	if !strings.Contains(takedownSummaryLine(ts), "이번 주 0건") {
		t.Errorf("summary line: got %q", takedownSummaryLine(ts))
	}
}

// 테이블 조회가 실패하면 섹션만 빠지고(키 생략) 나머지 통계는 그대로다.
func TestAttachTakedownStats_조회실패시_생략(t *testing.T) {
	db := openTakedownTestDB(t, "takedown_missing_table")

	stats := &reportStats{
		DateFrom:     "2026-09-27",
		DateTo:       "2026-10-03",
		TotalReports: 7,
	}
	attachTakedownStats(db, stats, stats.DateFrom, stats.DateTo)
	if stats.TakedownStats != nil {
		t.Fatalf("expected nil takedown stats on query failure, got %+v", stats.TakedownStats)
	}

	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "takedown_stats") {
		t.Errorf("takedown_stats should be omitted: %s", raw)
	}
	if !strings.Contains(string(raw), `"total_reports":7`) {
		t.Errorf("existing fields should be unchanged: %s", raw)
	}
	if line := takedownSummaryLine(stats.TakedownStats); line != "" {
		t.Errorf("summary line should be empty, got %q", line)
	}
}

func TestReportStatsJSON_takedown_stats_형태(t *testing.T) {
	stats := &reportStats{
		TakedownStats: &takedownStats{
			Week:  map[string]int{tdKisa: 1},
			Total: map[string]int{tdKisa: 3},
			Since: tdSince,
		},
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `"takedown_stats":{"week":{"kisa":1},"total":{"kisa":3},"since":"2026-02-26"}`
	if !strings.Contains(string(raw), want) {
		t.Errorf("json shape: got %s, want substring %s", raw, want)
	}
}
