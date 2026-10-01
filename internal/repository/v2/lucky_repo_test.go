package v2

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약 (럭키 포인트 하루 상한·단계 이름):
//
//   - 같은 회원은 KST 하루에 MemberDailyCap 번까지만, 사이트 전체는 DailyCap 번까지만 받는다.
//   - 상한에 닿으면 원장·포인트·잔액을 **하나도** 쓰지 않는다(반쯤 쓰고 멈추면 안 된다).
//   - 「하루」는 KST 00:00 기준이다 — UTC 15:00 이 경계다.
//   - 설정 키가 없으면 무제한이 아니라 기본값(회원 1·사이트 10·글만)이다.
//
// sqlite 에는 FOR UPDATE 가 없어(드라이버가 잠금 절을 뺀다) 동시성(C4)은 여기서 물지 않는다.
// 여기서는 건수 판정과 「안 쓰고 끝내기」만 본다.

func newLuckyTestRepo(t *testing.T) (*luckyRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	// :memory: 는 연결마다 다른 DB 다. 연결을 하나로 묶어야 트랜잭션 밖 확인 쿼리가 같은 DB 를 본다.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	for _, ddl := range []string{
		`CREATE TABLE g5_da_lucky_grant (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT NOT NULL,
			source_table TEXT NOT NULL,
			source_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			amount INTEGER NOT NULL,
			po_id INTEGER NULL,
			created_at DATETIME NOT NULL,
			UNIQUE (source_table, source_id, kind))`,
		`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_point INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE g5_point (
			po_id INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT, po_datetime DATETIME, po_content TEXT, po_point INTEGER,
			po_use_point INTEGER, po_expired INTEGER, po_expire_date TEXT,
			po_rel_table TEXT, po_rel_id TEXT, po_rel_action TEXT, po_mb_point INTEGER)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("테이블 생성 실패: %v", err)
		}
	}
	return &luckyRepository{db: db}, db
}

func addLuckyMember(t *testing.T, db *gorm.DB, mbID string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_point) VALUES (?, 0)`, mbID).Error; err != nil {
		t.Fatalf("회원 생성 실패: %v", err)
	}
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.Table(table).Count(&n).Error; err != nil {
		t.Fatalf("%s 건수 조회 실패: %v", table, err)
	}
	return n
}

func memberPoint(t *testing.T, db *gorm.DB, mbID string) int {
	t.Helper()
	var p int
	if err := db.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&p).Error; err != nil {
		t.Fatalf("잔액 조회 실패: %v", err)
	}
	return p
}

// TestKSTDayStartUTC 는 하루 경계가 UTC 15:00 인지 본다(L4).
func TestKSTDayStartUTC(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"UTC 14:59:59 = KST 23:59:59 → 전날 15:00 UTC 시작",
			time.Date(2026, 10, 1, 14, 59, 59, 0, time.UTC),
			time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)},
		{"UTC 15:00:00 = KST 다음날 00:00 → 당일 15:00 UTC 시작",
			time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)},
		{"UTC 00:00 = KST 09:00 → 전날 15:00 UTC",
			time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)},
		{"KST 로 들어와도 같은 순간이면 같은 답",
			time.Date(2026, 10, 2, 0, 0, 0, 0, luckyKST),
			time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)},
		{"월말 경계",
			time.Date(2026, 10, 31, 15, 30, 0, 0, time.UTC),
			time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got := kstDayStartUTC(c.now)
		if !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
		if got.Location() != time.UTC {
			t.Errorf("%s: UTC 로 돌려줘야 한다, got %s", c.name, got.Location())
		}
	}
}

// TestGrant_MemberDailyCap_KSTBoundary — C2: 같은 회원 같은 KST 날 두 번째 당첨은 원장·포인트 모두 미기록,
// KST 자정이 지나면 다시 받는다.
func TestGrant_MemberDailyCap_KSTBoundary(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	opt := GrantOptions{MemberDailyCap: 1, DailyCap: 10, TierName: "앙복타임"}

	// KST 10/01 23:00
	opt.Now = time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	if out, err := r.GrantWithOptions("member_a", "free", "1", "point", 100, opt); err != nil || out != GrantOutcomeGranted {
		t.Fatalf("첫 당첨은 지급돼야 한다: out=%s err=%v", out, err)
	}

	// KST 10/01 23:59:59 — 같은 날, 다른 글
	opt.Now = time.Date(2026, 10, 1, 14, 59, 59, 0, time.UTC)
	out, err := r.GrantWithOptions("member_a", "free", "2", "point", 200, opt)
	if err != nil {
		t.Fatalf("상한은 에러가 아니다: %v", err)
	}
	if out != GrantOutcomeCappedMember {
		t.Fatalf("같은 KST 날 두 번째는 capped_member 여야 한다, got %s", out)
	}
	if n := countRows(t, db, "g5_da_lucky_grant"); n != 1 {
		t.Errorf("원장은 1행이어야 한다, got %d", n)
	}
	if n := countRows(t, db, "g5_point"); n != 1 {
		t.Errorf("포인트 내역은 1행이어야 한다, got %d", n)
	}
	if p := memberPoint(t, db, "member_a"); p != 100 {
		t.Errorf("잔액은 첫 당첨분(100)만이어야 한다, got %d", p)
	}

	// KST 10/02 00:00:00 — 다음 날
	opt.Now = time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	if out, err := r.GrantWithOptions("member_a", "free", "3", "point", 250, opt); err != nil || out != GrantOutcomeGranted {
		t.Fatalf("KST 자정이 지나면 다시 받아야 한다: out=%s err=%v", out, err)
	}
	if p := memberPoint(t, db, "member_a"); p != 350 {
		t.Errorf("잔액 100+250 이어야 한다, got %d", p)
	}
}

// TestGrant_DailyCap — C3: 사이트 하루 10건 도달 후 11번째 당첨은 미기록, 다음 KST 날엔 다시 지급.
func TestGrant_DailyCap(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) // KST 12:00
	opt := GrantOptions{MemberDailyCap: 1, DailyCap: 10, Now: now}

	for i := 1; i <= 10; i++ {
		mb := fmt.Sprintf("member_%02d", i)
		addLuckyMember(t, db, mb)
		out, err := r.GrantWithOptions(mb, "free", fmt.Sprintf("%d", i), "point", 10, opt)
		if err != nil || out != GrantOutcomeGranted {
			t.Fatalf("%d번째는 지급돼야 한다: out=%s err=%v", i, out, err)
		}
	}

	addLuckyMember(t, db, "member_11")
	out, err := r.GrantWithOptions("member_11", "free", "11", "point", 10, opt)
	if err != nil {
		t.Fatalf("상한은 에러가 아니다: %v", err)
	}
	if out != GrantOutcomeCappedDaily {
		t.Fatalf("11번째는 capped_daily 여야 한다, got %s", out)
	}
	if n := countRows(t, db, "g5_da_lucky_grant"); n != 10 {
		t.Errorf("원장은 10행이어야 한다, got %d", n)
	}
	if n := countRows(t, db, "g5_point"); n != 10 {
		t.Errorf("포인트 내역은 10행이어야 한다, got %d", n)
	}
	if p := memberPoint(t, db, "member_11"); p != 0 {
		t.Errorf("막힌 회원 잔액은 그대로 0 이어야 한다, got %d", p)
	}

	// 다음 KST 날
	opt.Now = time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	if out, err := r.GrantWithOptions("member_11", "free", "12", "point", 10, opt); err != nil || out != GrantOutcomeGranted {
		t.Fatalf("다음 KST 날엔 지급돼야 한다: out=%s err=%v", out, err)
	}
}

// TestGrant_ZeroCapIsUnlimited 는 0 이 「명시적 무제한」인지, 그리고 기존 Grant(옵션 없음)가 그대로인지 본다.
func TestGrant_ZeroCapIsUnlimited(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		out, err := r.GrantWithOptions("member_a", "free", fmt.Sprintf("%d", i), "point", 1, GrantOptions{Now: now})
		if err != nil || out != GrantOutcomeGranted {
			t.Fatalf("상한 0 이면 계속 지급: i=%d out=%s err=%v", i, out, err)
		}
	}
	if ok, err := r.Grant("member_a", "free", "4", "point", 1); err != nil || !ok {
		t.Fatalf("기존 Grant 는 상한 없이 지급: ok=%v err=%v", ok, err)
	}
}

// TestGrant_Duplicate 는 같은 글 재시도가 no-op 인지 본다(기존 멱등성 유지).
func TestGrant_Duplicate(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	if out, err := r.GrantWithOptions("member_a", "free", "1", "point", 5, GrantOptions{Now: now}); err != nil || out != GrantOutcomeGranted {
		t.Fatalf("첫 지급: out=%s err=%v", out, err)
	}
	// sqlite 의 UNIQUE 위반은 MySQL 1062 가 아니라 isDuplicateKeyErr 로 못 잡을 수 있다.
	// 그래서 결과값이 아니라 「두 번 쓰이지 않았다」만 본다.
	_, _ = r.GrantWithOptions("member_a", "free", "1", "point", 5, GrantOptions{Now: now})
	if n := countRows(t, db, "g5_point"); n != 1 {
		t.Errorf("같은 글 두 번째는 포인트를 쓰면 안 된다, got %d", n)
	}
	if p := memberPoint(t, db, "member_a"); p != 5 {
		t.Errorf("잔액은 5 여야 한다, got %d", p)
	}
}

// TestGrant_TierNameInPointContent — L7: 내역 문구에 단계 이름이 들어가고, 배지 조인 키(po_rel_action)는 그대로다.
func TestGrant_TierNameInPointContent(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	addLuckyMember(t, db, "member_b")
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)

	if _, err := r.GrantWithOptions("member_a", "free", "1", "point", 7, GrantOptions{TierName: "앙팡팡타임", Now: now}); err != nil {
		t.Fatalf("지급 실패: %v", err)
	}
	if _, err := r.GrantWithOptions("member_b", "free", "2", "point", 7, GrantOptions{Now: now}); err != nil {
		t.Fatalf("지급 실패: %v", err)
	}

	type row struct {
		MbID        string
		PoContent   string
		PoRelTable  string
		PoRelID     string
		PoRelAction string
	}
	var rows []row
	if err := db.Table("g5_point").Select("mb_id, po_content, po_rel_table, po_rel_id, po_rel_action").Order("po_id").Scan(&rows).Error; err != nil {
		t.Fatalf("내역 조회 실패: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("내역 2행이어야 한다, got %d", len(rows))
	}
	if rows[0].PoContent != "앙팡팡타임 럭키 포인트" {
		t.Errorf("단계 이름 문구, got %q", rows[0].PoContent)
	}
	if rows[1].PoContent != luckyPointContent {
		t.Errorf("단계 이름이 없으면 기존 문구, got %q", rows[1].PoContent)
	}
	for _, rw := range rows {
		if rw.PoRelAction != "@lucky" {
			t.Errorf("po_rel_action 은 @lucky 그대로여야 한다(배지 조인), got %q", rw.PoRelAction)
		}
		if rw.PoRelTable != "free" {
			t.Errorf("po_rel_table 은 게시판 slug, got %q", rw.PoRelTable)
		}
	}
}

// TestLuckyConfig_Defaults — C5: 키가 없으면 기본값(글만·회원 1·사이트 10·9~23시·시간대 없음). 무제한이 아니다.
func TestLuckyConfig_Defaults(t *testing.T) {
	var c LuckyConfig
	if err := json.Unmarshal([]byte(`{"enabled": true}`), &c); err != nil {
		t.Fatalf("파싱 실패: %v", err)
	}
	if !c.Enabled {
		t.Error("enabled=true 가 반영돼야 한다")
	}
	if c.IncludeComments {
		t.Error("include_comments 기본은 false(글만)")
	}
	if c.MemberDailyCap != 1 || c.DailyCap != 10 {
		t.Errorf("상한 기본 1/10, got %d/%d", c.MemberDailyCap, c.DailyCap)
	}
	if c.WindowStartHour != 9 || c.WindowEndHour != 23 {
		t.Errorf("시간대 범위 기본 9/23, got %d/%d", c.WindowStartHour, c.WindowEndHour)
	}
	if len(c.Windows) != 0 {
		t.Errorf("windows 기본은 비어 있음(앙복만), got %d", len(c.Windows))
	}

	d := DefaultLuckyConfig()
	if d.Enabled || d.IncludeComments || d.MemberDailyCap != 1 || d.DailyCap != 10 {
		t.Errorf("DefaultLuckyConfig 가 바뀌었다: %+v", *d)
	}
}

// TestLuckyConfig_ExplicitValues 는 0(명시적 무제한)·음수·단계 목록 파싱을 본다.
func TestLuckyConfig_ExplicitValues(t *testing.T) {
	var c LuckyConfig
	raw := `{"enabled":true,"include_comments":true,"member_daily_cap":0,"daily_cap":0,
		"window_start_hour":10,"window_end_hour":22,
		"windows":[{"name":"앙팡타임","minutes":45,"odds":7,"points":50}]}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("파싱 실패: %v", err)
	}
	if !c.IncludeComments || c.MemberDailyCap != 0 || c.DailyCap != 0 {
		t.Errorf("명시값이 반영돼야 한다: %+v", c)
	}
	if c.WindowStartHour != 10 || c.WindowEndHour != 22 || len(c.Windows) != 1 || c.Windows[0].Name != "앙팡타임" {
		t.Errorf("시간대 설정 파싱: %+v", c)
	}

	var neg LuckyConfig
	if err := json.Unmarshal([]byte(`{"enabled":true,"member_daily_cap":-1,"daily_cap":-5}`), &neg); err != nil {
		t.Fatalf("파싱 실패: %v", err)
	}
	if neg.MemberDailyCap != 1 || neg.DailyCap != 10 {
		t.Errorf("음수는 기본값으로(무제한으로 새면 안 됨), got %d/%d", neg.MemberDailyCap, neg.DailyCap)
	}
}

// TestLuckyConfig_MalformedIsOffAndIsolated 는 럭키 설정 오타가 「꺼짐」이 되고 xp_config 를 같이 날리지 않는지 본다.
func TestLuckyConfig_MalformedIsOffAndIsolated(t *testing.T) {
	raw := `{"xp_config":{"write_xp":123},"lucky_config":{"enabled":true,"windows":"oops"}}`
	var w settingsJSONWrapper
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("럭키 설정 오류가 settings_json 전체 파싱을 깨면 안 된다: %v", err)
	}
	if w.XPConfig == nil || w.XPConfig.WriteXP != 123 {
		t.Errorf("xp_config 는 그대로 읽혀야 한다: %+v", w.XPConfig)
	}
	if w.LuckyConfig == nil {
		t.Fatal("lucky_config 는 기본값(꺼짐)으로 채워져야 한다")
	}
	if w.LuckyConfig.Enabled {
		t.Error("잘못된 설정이면 꺼져야 한다")
	}
}
