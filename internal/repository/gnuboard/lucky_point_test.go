package gnuboard

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약 (럭키 배지 재료 조회):
//
//   - LuckyBadgesByWrID 는 목록 크기와 무관하게 정확히 2쿼리(g5_point 1 + g5_na_xp 1)다. 빈 목록은 0쿼리(C20).
//   - 포인트는 g5_point(po_rel_action=@lucky), 경험치는 g5_na_xp(xp_rel_action=@lucky)에서만 읽는다 —
//     같은 글의 다른 경험치(글쓰기 등)·다른 게시판 기록은 배지에 섞이지 않는다.
//   - 경험치 조회만 실패하면 포인트 배지는 남긴다.
//   - 단계(lucky_tier)·시각(lucky_at)은 지급 행의 문구·시각에서만 나온다. 정해진 단계 문구가 아니면(레거시) 둘 다 생략.
//     시각은 KST 벽시계 + 「+09:00」이다. 단계·시각을 더 실어도 쿼리 수는 그대로다.

// sqlCountingLogger 는 실행된 SQL 문을 모두 기록한다(쿼리 수 검증용).
type sqlCountingLogger struct {
	mu   sync.Mutex
	sqls []string
}

func (l *sqlCountingLogger) LogMode(logger.LogLevel) logger.Interface      { return l }
func (l *sqlCountingLogger) Info(context.Context, string, ...interface{})  {}
func (l *sqlCountingLogger) Warn(context.Context, string, ...interface{})  {}
func (l *sqlCountingLogger) Error(context.Context, string, ...interface{}) {}
func (l *sqlCountingLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	l.mu.Lock()
	l.sqls = append(l.sqls, sql)
	l.mu.Unlock()
}

func (l *sqlCountingLogger) reset() {
	l.mu.Lock()
	l.sqls = nil
	l.mu.Unlock()
}

func (l *sqlCountingLogger) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.sqls...)
}

func newLuckyBadgeTestDB(t *testing.T, withXP bool) (*gorm.DB, *sqlCountingLogger) {
	t.Helper()
	counter := &sqlCountingLogger{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: counter})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	ddls := []string{
		`CREATE TABLE g5_point (
			po_id INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT, po_datetime DATETIME, po_content TEXT, po_point INTEGER,
			po_rel_table TEXT, po_rel_id TEXT, po_rel_action TEXT)`,
	}
	if withXP {
		ddls = append(ddls, `CREATE TABLE g5_na_xp (
			xp_id INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT, xp_datetime DATETIME, xp_content TEXT, xp_point INTEGER,
			xp_rel_table TEXT, xp_rel_id TEXT, xp_rel_action TEXT)`)
	}
	for _, ddl := range ddls {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("테이블 생성 실패: %v", err)
		}
	}

	const pIns = `INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_rel_table, po_rel_id, po_rel_action) VALUES `
	const xIns = `INSERT INTO g5_na_xp (mb_id, xp_datetime, xp_content, xp_point, xp_rel_table, xp_rel_id, xp_rel_action) VALUES `
	seed := []string{
		// wr 101: 포인트만, 레거시 문구 → 단계·시각 없음
		pIns + `('m1', '2026-03-01 08:00:00', '나리야 럭키 포인트', 73, 'free', '101', '@lucky')`,
		// wr 103: 둘 다(같은 지급)
		pIns + `('m3', '2026-10-01 10:00:00', '앙팡팡타임 럭키 포인트', 58, 'free', '103', '@lucky')`,
		// wr 106: 레거시 중복 행 + 단계 행 — 금액은 MAX, 단계·시각은 단계 문구가 있는 가장 이른 행
		pIns + `('m6', '2026-10-01 09:30:00', '앙복타임 럭키 포인트', 11, 'free', '106', '@lucky')`,
		pIns + `('m6', '2026-01-01 00:00:00', '나리야 럭키 포인트', 29, 'free', '106', '@lucky')`,
		// 섞이면 안 되는 포인트: 다른 action, 다른 게시판
		pIns + `('m1', '2026-10-01 07:00:00', '앙팡타임 럭키 포인트', 999, 'free', '101', '@write')`,
		pIns + `('m4', '2026-10-01 07:00:00', '앙팡타임 럭키 포인트', 777, 'other', '104', '@lucky')`,
	}
	if withXP {
		seed = append(seed,
			// wr 102: 경험치만(댓글 문구)
			xIns+`('m2', '2026-10-01 14:23:05', '앙팡타임 럭키 경험치(댓글)', 37, 'free', '102', '@lucky')`,
			// wr 103: 둘 다(같은 지급)
			xIns+`('m3', '2026-10-01 10:00:00', '앙팡팡타임 럭키 경험치', 41, 'free', '103', '@lucky')`,
			// wr 106: 포인트 행보다 이른 경험치 행
			xIns+`('m6', '2026-10-01 08:00:00', '앙팡타임 럭키 경험치', 37, 'free', '106', '@lucky')`,
			// 섞이면 안 되는 경험치: 같은 글의 글쓰기 경험치, 다른 게시판
			xIns+`('m1', '2026-10-01 06:00:00', '앙팡타임 럭키 경험치', 123, 'free', '101', '@write')`,
			xIns+`('m4', '2026-10-01 06:00:00', '앙팡타임 럭키 경험치', 888, 'other', '104', '@lucky')`,
		)
	}
	for _, s := range seed {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("시드 실패: %v", err)
		}
	}
	counter.reset()
	return db, counter
}

// TestLuckyBadgesByWrID_Values — 포인트만·경험치만·둘 다·없음이 정확히 나뉘고, 다른 action·게시판은 섞이지 않는다.
func TestLuckyBadgesByWrID_Values(t *testing.T) {
	db, _ := newLuckyBadgeTestDB(t, true)
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 102, 103, 104, 105, 106}, LuckyTierNames{})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	want := map[int]LuckyBadge{
		101: {Points: 73}, // 레거시 문구 — 단계·시각 없음
		102: {Exp: 37, Tier: "앙팡타임", At: "2026-10-01T14:23:05+09:00"},
		103: {Points: 58, Exp: 41, Tier: "앙팡팡타임", At: "2026-10-01T10:00:00+09:00"},
		106: {Points: 29, Exp: 37, Tier: "앙팡타임", At: "2026-10-01T08:00:00+09:00"},
	}
	if len(got) != len(want) {
		t.Fatalf("기록 있는 글만 맵에 있어야 한다: got %+v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("wr %d: got %+v want %+v", id, got[id], w)
		}
	}
}

// TestLuckyBadgesByWrID_TwoQueriesPerList — C20: 목록 크기(1·5·50)와 무관하게 2쿼리, 빈 목록·0 이하 id 만이면 0쿼리.
func TestLuckyBadgesByWrID_TwoQueriesPerList(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	big := make([]int, 50)
	for i := range big {
		big[i] = 100 + i
	}
	for _, ids := range [][]int{{101}, {101, 102, 103, 104, 105}, big} {
		counter.reset()
		if _, err := LuckyBadgesByWrID(db, "free", ids, LuckyTierNames{}); err != nil {
			t.Fatalf("조회 실패: %v", err)
		}
		sqls := counter.snapshot()
		if len(sqls) != 2 {
			t.Fatalf("목록 %d건: 쿼리 2회여야 한다, got %d: %v", len(ids), len(sqls), sqls)
		}
		if !strings.Contains(sqls[0], "g5_point") || !strings.Contains(sqls[1], "g5_na_xp") {
			t.Errorf("g5_point 1 + g5_na_xp 1 이어야 한다: %v", sqls)
		}
	}

	for _, ids := range [][]int{nil, {}, {0, -3}} {
		counter.reset()
		if _, err := LuckyBadgesByWrID(db, "free", ids, LuckyTierNames{}); err != nil {
			t.Fatalf("조회 실패: %v", err)
		}
		if n := len(counter.snapshot()); n != 0 {
			t.Errorf("빈 목록 %v: 쿼리 0회여야 한다, got %d", ids, n)
		}
	}
}

// TestLuckyBadgesByWrID_ExpFailureKeepsPoints — 경험치 조회만 실패하면 에러와 함께 포인트 배지는 남긴다.
func TestLuckyBadgesByWrID_ExpFailureKeepsPoints(t *testing.T) {
	db, _ := newLuckyBadgeTestDB(t, false) // g5_na_xp 없음 → 경험치 조회 실패
	got, err := LuckyBadgesByWrID(db, "free", []int{101, 103}, LuckyTierNames{})
	if err == nil {
		t.Fatal("경험치 조회 실패는 에러로 알려야 한다")
	}
	if got[101].Points != 73 || got[103].Points != 58 || got[101].Exp != 0 || got[103].Tier != "앙팡팡타임" {
		t.Errorf("포인트 배지는 남아야 한다, got %+v", got)
	}
}

// TestLuckyTierFromContent — 정해진 단계로 시작하는 문구만 단계로 인정한다(댓글 「(댓글)」 포함). 레거시·변형은 미표시.
func TestLuckyTierFromContent(t *testing.T) {
	cases := []struct {
		content string
		want    string
		ok      bool
	}{
		{"앙복타임 럭키 포인트", LuckyDefaultBaseName, true}, // 예전 평소 단계 → 현재 평소 단계 이름(기본 「앙팡」)
		{"앙팡 럭키 포인트", "앙팡", true},
		{"앙팡타임 럭키 경험치(댓글)", "앙팡타임", true},
		{"앙팡팡타임 럭키 포인트(댓글)", "앙팡팡타임", true},
		{"나리야 럭키 포인트", "", false},
		{"나리야 럭키 경험치(댓글)", "", false},
		{"앙팡타임럭키 포인트", "", false},   // 공백 없음
		{" 앙팡타임 럭키 포인트", "", false}, // 앞 공백
		{"다른타임 럭키 포인트", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := LuckyTierFromContent(c.content, LuckyTierNames{})
		if got != c.want || ok != c.ok {
			t.Errorf("LuckyTierFromContent(%q) = %q,%v want %q,%v", c.content, got, ok, c.want, c.ok)
		}
	}
}

// TestFormatLuckyAt — 저장된 KST 벽시계 숫자에 +09:00 만 붙인다(시간대 라벨과 무관).
func TestFormatLuckyAt(t *testing.T) {
	wall := time.Date(2026, 10, 1, 14, 23, 5, 0, time.UTC) // 드라이버가 UTC 라벨로 읽은 경우
	if got := FormatLuckyAt(wall); got != "2026-10-01T14:23:05+09:00" {
		t.Errorf("got %s", got)
	}
	kst := time.Date(2026, 10, 1, 14, 23, 5, 0, time.FixedZone("KST", 9*3600)) // loc=Asia/Seoul 로 읽은 경우
	if got := FormatLuckyAt(kst); got != "2026-10-01T14:23:05+09:00" {
		t.Errorf("got %s", got)
	}
}

// TestLuckyBadgeApplyTo — 응답 필드: lucky_point·lucky_exp 는 항상, lucky_tier·lucky_at 은 단계가 있을 때만.
// 지급된 행에서 온 값 외에는 어떤 시각 키도 싣지 않는다.
func TestLuckyBadgeApplyTo(t *testing.T) {
	legacy := map[string]any{"id": 1}
	LuckyBadge{Points: 73}.ApplyTo(legacy)
	if legacy["lucky_point"] != 73 || legacy["lucky_exp"] != 0 {
		t.Errorf("포인트·경험치는 항상 실린다, got %v", legacy)
	}
	if _, ok := legacy["lucky_tier"]; ok {
		t.Errorf("레거시는 lucky_tier 생략, got %v", legacy)
	}
	if _, ok := legacy["lucky_at"]; ok {
		t.Errorf("레거시는 lucky_at 생략, got %v", legacy)
	}

	none := map[string]any{}
	LuckyBadge{}.ApplyTo(none)
	if len(none) != 2 || none["lucky_point"] != 0 || none["lucky_exp"] != 0 {
		t.Errorf("미당첨은 0 두 개만, got %v", none)
	}

	tiered := map[string]any{}
	LuckyBadge{Exp: 37, Tier: "앙팡타임", At: "2026-10-01T14:23:05+09:00"}.ApplyTo(tiered)
	want := map[string]any{"lucky_point": 0, "lucky_exp": 37, "lucky_tier": "앙팡타임", "lucky_at": "2026-10-01T14:23:05+09:00"}
	if len(tiered) != len(want) {
		t.Fatalf("키 집합이 달라선 안 된다, got %v", tiered)
	}
	for k, v := range want {
		if tiered[k] != v {
			t.Errorf("%s: got %v want %v", k, tiered[k], v)
		}
	}
}

// TestLuckyTierFromContent_ConfiguredNames — F3: 설정된 이름(고정 시간대 등)으로 시작하는 문구는 그 이름이 단계가 된다.
// 이름에 정규식 메타문자가 있어도 글자 그대로 비교하고, 목록에 없는 이름·레거시 문구는 여전히 미표시다.
func TestLuckyTierFromContent_ConfiguredNames(t *testing.T) {
	names := LuckyTierNames{Names: []string{"테스트구간", "a.b(c)*", "테스트"}}
	cases := []struct {
		content string
		want    string
		ok      bool
	}{
		{"테스트구간 럭키 포인트", "테스트구간", true},
		{"테스트구간 럭키 경험치(댓글)", "테스트구간", true},
		{"테스트 럭키 포인트", "테스트", true},         // 짧은 이름도 구분자까지 맞아야만
		{"a.b(c)* 럭키 포인트", "a.b(c)*", true}, // 메타문자는 글자 그대로
		{"aXb(c)* 럭키 포인트", "", false},       // 정규식처럼 해석되면 안 된다
		{"테스트구간럭키 포인트", "", false},          // 구분자 없음
		{"테스트구간 포인트", "", false},            // 「 럭키 」 없음
		{"앙팡타임 럭키 포인트", "앙팡타임", true},       // 기본 이름은 목록과 무관하게 인정
		{"나리야 럭키 포인트", "", false},           // 레거시는 미표시 유지
		{"다른구간 럭키 포인트", "", false},          // 설정에 없는 이름
	}
	for _, c := range cases {
		got, ok := LuckyTierFromContent(c.content, names)
		if got != c.want || ok != c.ok {
			t.Errorf("LuckyTierFromContent(%q) = %q,%v want %q,%v", c.content, got, ok, c.want, c.ok)
		}
	}
	// 목록을 안 주면 설정 이름은 인정하지 않는다(기본 이름만).
	if _, ok := LuckyTierFromContent("테스트구간 럭키 포인트", LuckyTierNames{}); ok {
		t.Error("이름 목록이 없으면 설정 이름은 단계가 아니어야 한다")
	}
}

// TestLuckyBadgesByWrID_ConfiguredTier — F3: 설정 이름으로 지급된 행은 배지 tier·시각이 실리고, 쿼리 수는 그대로 2다.
func TestLuckyBadgesByWrID_ConfiguredTier(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_rel_table, po_rel_id, po_rel_action)
		VALUES ('m7', '2026-10-01 03:10:00', '테스트구간 럭키 포인트(댓글)', 21, 'free', '107', '@lucky')`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	counter.reset()

	got, err := LuckyBadgesByWrID(db, "free", []int{101, 107}, LuckyTierNames{Names: []string{"테스트구간"}})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := len(counter.snapshot()); n != 2 {
		t.Errorf("이름 목록을 받아도 2쿼리여야 한다, got %d", n)
	}
	b := got[107]
	if b.Tier != "테스트구간" || b.At != "2026-10-01T03:10:00+09:00" || b.Points != 21 {
		t.Errorf("설정 이름 tier 가 실려야 한다: %+v", b)
	}
	if got[101].Tier != "" || got[101].At != "" {
		t.Errorf("레거시 문구는 여전히 tier 미표시: %+v", got[101])
	}

	// 이름 목록 없이 보면 같은 행이 tier 없이(포인트만) 나온다.
	got, _ = LuckyBadgesByWrID(db, "free", []int{107}, LuckyTierNames{})
	if got[107].Tier != "" || got[107].Points != 21 {
		t.Errorf("목록에 없는 이름은 tier 미표시·포인트는 유지: %+v", got[107])
	}
}
