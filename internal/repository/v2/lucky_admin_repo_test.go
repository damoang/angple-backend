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

// ⛔ 이 파일이 지키는 계약 (앙팡 관리자 저장소):
//
//   - A2 lucky_config 저장은 settings_json 의 다른 키를 보존하고, 같은 트랜잭션에서 이력 {at, by, before, after}
//     를 남기며 이력은 최근 50개만 둔다. 행이 없으면 active_theme 를 명시해 만든다. 저장 뒤 이 파드 캐시를 비운다.
//   - A3 게시판 일괄 변경은 lucky 키만 바꾸고 다른 키를 보존하며, 행이 없는 게시판은 새로 만든다.
//   - A4 원장 조회는 [start, end) 다 — KST 00:00 정각은 그날, 다음날 00:00 정각은 다음날이다.
//
// sqlite 의 json_set/json() 으로 같은 SQL 경로를 돈다(운영 MySQL 은 CAST(? AS JSON)). 값은 임의의 테스트 값이다.

func newLuckyAdminTestRepo(t *testing.T) (*luckyAdminRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard, TranslateError: true})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 얻기 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	for _, ddl := range []string{
		`CREATE TABLE site_settings (
			site_id TEXT PRIMARY KEY,
			settings_json TEXT NULL,
			active_theme TEXT DEFAULT 'damoang-official',
			created_at DATETIME NULL,
			updated_at DATETIME NULL)`,
		`CREATE TABLE v2_board_extended_settings (
			board_id TEXT PRIMARY KEY,
			settings TEXT NULL,
			created_at DATETIME NULL,
			updated_at DATETIME NULL)`,
		`CREATE TABLE g5_board (bo_table TEXT PRIMARY KEY, bo_subject TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE g5_da_lucky_grant (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mb_id TEXT NOT NULL, source_table TEXT NOT NULL, source_id TEXT NOT NULL,
			kind TEXT NOT NULL, amount INTEGER NOT NULL, po_id INTEGER NULL,
			created_at DATETIME NOT NULL,
			UNIQUE (source_table, source_id, kind))`,
		`CREATE TABLE g5_point (po_id INTEGER PRIMARY KEY AUTOINCREMENT, po_content TEXT)`,
		`CREATE TABLE g5_na_xp (
			xp_id INTEGER PRIMARY KEY AUTOINCREMENT, xp_point INTEGER, xp_content TEXT,
			xp_rel_table TEXT, xp_rel_id TEXT, xp_rel_action TEXT)`,
		`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_nick TEXT)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("테이블 생성 실패: %v", err)
		}
	}
	return &luckyAdminRepository{db: db}, db
}

func readSettingsTop(t *testing.T, db *gorm.DB) map[string]json.RawMessage {
	t.Helper()
	var s string
	if err := db.Raw(`SELECT settings_json FROM site_settings WHERE site_id = 'default'`).Scan(&s).Error; err != nil {
		t.Fatalf("settings_json 읽기 실패: %v", err)
	}
	top := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(s), &top); err != nil {
		t.Fatalf("settings_json 이 JSON 이 아니다: %v (%s)", err, s)
	}
	return top
}

// TestSaveLuckyConfig_PreservesOtherKeysAndRecordsHistory — A2: 다른 키 보존·이력·캐시 무효화.
func TestSaveLuckyConfig_PreservesOtherKeysAndRecordsHistory(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	if err := db.Exec(`INSERT INTO site_settings (site_id, settings_json, active_theme) VALUES ('default', ?, 'theme-x')`,
		`{"xp_config":{"login_xp":12},"point_config":{"expiry_days":34},"lucky_config":{"enabled":false,"member_daily_cap":2}}`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	// 캐시에 옛 값을 넣어 두고 저장 뒤 비워지는지 본다.
	luckyConfigCacheMu.Lock()
	luckyConfigCacheVal, luckyConfigCacheExpAt = DefaultLuckyConfig(), time.Now().Add(time.Hour)
	luckyConfigCacheMu.Unlock()

	cfg := DefaultLuckyConfig()
	cfg.Enabled = true
	cfg.FixedWindows = []LuckyFixedWindow{{Name: "고정가", Start: "01:00", End: "02:00", Odds: 3, Points: 4, Prizes: []LuckyPrize{}}}
	now := time.Date(2026, 10, 1, 3, 4, 5, 0, time.UTC)
	if err := repo.SaveLuckyConfig(cfg, "admin_a", now); err != nil {
		t.Fatalf("저장 실패: %v", err)
	}

	top := readSettingsTop(t, db)
	if string(top["xp_config"]) != `{"login_xp":12}` || string(top["point_config"]) != `{"expiry_days":34}` {
		t.Errorf("다른 키는 그대로여야 한다: xp=%s point=%s", top["xp_config"], top["point_config"])
	}
	var got LuckyConfig
	if err := json.Unmarshal(top["lucky_config"], &got); err != nil || !got.Enabled || len(got.FixedWindows) != 1 || got.FixedWindows[0].Name != "고정가" {
		t.Errorf("lucky_config 가 새 값이어야 한다: %s", top["lucky_config"])
	}
	var hist []LuckyConfigHistoryEntry
	if err := json.Unmarshal(top["lucky_config_history"], &hist); err != nil || len(hist) != 1 {
		t.Fatalf("이력 1건이어야 한다: %s", top["lucky_config_history"])
	}
	h := hist[0]
	if h.By != "admin_a" || h.At != "2026-10-01T12:04:05+09:00" {
		t.Errorf("이력 by·at(KST): %+v", h)
	}
	var before map[string]any
	if err := json.Unmarshal(h.Before, &before); err != nil || before["member_daily_cap"] != float64(2) {
		t.Errorf("before 는 직전 저장값이어야 한다: %s", h.Before)
	}
	var after LuckyConfig
	if err := json.Unmarshal(h.After, &after); err != nil || !after.Enabled {
		t.Errorf("after 는 새 값이어야 한다: %s", h.After)
	}
	var theme string
	db.Raw(`SELECT active_theme FROM site_settings WHERE site_id='default'`).Scan(&theme)
	if theme != "theme-x" {
		t.Errorf("기존 행의 테마를 건드리면 안 된다: %s", theme)
	}
	luckyConfigCacheMu.RLock()
	cached := luckyConfigCacheVal
	luckyConfigCacheMu.RUnlock()
	if cached != nil {
		t.Error("저장 뒤 이 파드의 캐시가 비어야 한다")
	}

	// 다시 읽으면 저장된 값과 이력이 나온다.
	stored, gotHist, raw, err := repo.GetStoredLuckyConfig()
	if err != nil || !stored.Enabled || len(gotHist) != 1 || len(raw) == 0 {
		t.Errorf("다시 읽기: %+v %d raw=%s %v", stored, len(gotHist), raw, err)
	}
}

// TestGetStoredLuckyConfig_ReturnsRawUntouched — 타입이 틀린 저장값도 원문을 그대로 돌려주고,
// 설정 값은 지급 경로와 같은 관대한 파싱(기본값 폴백)을 거친다. 설정이 없으면 원문은 nil 이다.
func TestGetStoredLuckyConfig_ReturnsRawUntouched(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	if _, _, raw, err := repo.GetStoredLuckyConfig(); err != nil || raw != nil {
		t.Fatalf("행이 없으면 원문 nil: %s %v", raw, err)
	}
	const bad = `{"enabled":true,"windows":[{"name":"가","minutes":10,"odds":"8","points":3}]}`
	if err := db.Exec(`INSERT INTO site_settings (site_id, settings_json) VALUES ('default', ?)`,
		`{"xp_config":{"login_xp":12},"lucky_config":`+bad+`}`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	cfg, _, raw, err := repo.GetStoredLuckyConfig()
	if err != nil {
		t.Fatalf("읽기 실패: %v", err)
	}
	if string(raw) != bad {
		t.Errorf("원문 그대로여야 한다: %s", raw)
	}
	if cfg.Enabled {
		t.Error("타입 오류 설정은 지급 경로와 같게 기본값(꺼짐)으로 읽혀야 한다")
	}
}

// TestSaveLuckyConfig_HistoryCappedAt50 — A2: 이력은 최근 50개만, 오래된 것부터 버린다.
func TestSaveLuckyConfig_HistoryCappedAt50(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < luckyConfigHistoryMax+5; i++ {
		cfg := DefaultLuckyConfig()
		cfg.MemberDailyCap = i
		if err := repo.SaveLuckyConfig(cfg, fmt.Sprintf("admin_%d", i), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("%d번째 저장 실패: %v", i, err)
		}
	}
	var hist []LuckyConfigHistoryEntry
	if err := json.Unmarshal(readSettingsTop(t, db)["lucky_config_history"], &hist); err != nil {
		t.Fatalf("이력 파싱 실패: %v", err)
	}
	if len(hist) != luckyConfigHistoryMax {
		t.Fatalf("이력은 %d개여야 한다, got %d", luckyConfigHistoryMax, len(hist))
	}
	if hist[0].By != "admin_5" || hist[len(hist)-1].By != fmt.Sprintf("admin_%d", luckyConfigHistoryMax+4) {
		t.Errorf("오래된 것부터 버려야 한다: first=%s last=%s", hist[0].By, hist[len(hist)-1].By)
	}
}

// TestSaveLuckyConfig_CreatesRowWithTheme — A2: 행이 없으면 active_theme 를 명시해 만든다(before=null).
func TestSaveLuckyConfig_CreatesRowWithTheme(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	if err := repo.SaveLuckyConfig(DefaultLuckyConfig(), "admin_a", time.Now()); err != nil {
		t.Fatalf("저장 실패: %v", err)
	}
	var theme string
	db.Raw(`SELECT active_theme FROM site_settings WHERE site_id='default'`).Scan(&theme)
	if theme != luckyDefaultActiveTheme {
		t.Errorf("새 행의 테마는 %s, got %s", luckyDefaultActiveTheme, theme)
	}
	var hist []LuckyConfigHistoryEntry
	_ = json.Unmarshal(readSettingsTop(t, db)["lucky_config_history"], &hist)
	if len(hist) != 1 || string(hist[0].Before) != "null" {
		t.Errorf("첫 이력의 before 는 null: %+v", hist)
	}
}

// TestSaveLuckyConfig_NullSettingsJSON — settings_json 이 NULL 이어도 키가 사라지지 않고 써진다.
func TestSaveLuckyConfig_NullSettingsJSON(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	if err := db.Exec(`INSERT INTO site_settings (site_id, settings_json) VALUES ('default', NULL)`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	cfg := DefaultLuckyConfig()
	cfg.Enabled = true
	if err := repo.SaveLuckyConfig(cfg, "admin_a", time.Now()); err != nil {
		t.Fatalf("저장 실패: %v", err)
	}
	var got LuckyConfig
	if err := json.Unmarshal(readSettingsTop(t, db)["lucky_config"], &got); err != nil || !got.Enabled {
		t.Errorf("NULL 위에도 저장돼야 한다")
	}
}

// TestSetBoardLucky_PreservesOtherKeysAndCreates — A3: lucky 외 키 보존, 행 없는 게시판 생성.
func TestSetBoardLucky_PreservesOtherKeysAndCreates(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	for _, s := range []string{
		`INSERT INTO g5_board (bo_table, bo_subject) VALUES ('board_a','가'),('board_b','나'),('board_c','다')`,
		`INSERT INTO v2_board_extended_settings (board_id, settings) VALUES ('board_a', '{"write_notice":{"text":"안내"},"lucky":{"enabled":false,"odds":9,"points":1}}')`,
		`INSERT INTO v2_board_extended_settings (board_id, settings) VALUES ('board_c', '{"write_notice":{"text":"그대로"}}')`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("시드 실패: %v", err)
		}
	}
	lucky := BoardLucky{Enabled: true, Odds: 3, CommentOdds: 0, Points: 5, Prizes: []LuckyPrize{{Weight: 1, Points: 2, Exp: 1}}}
	changed, err := repo.SetBoardLucky([]string{"board_a", "board_b"}, lucky, time.Now())
	if err != nil {
		t.Fatalf("저장 실패: %v", err)
	}
	if len(changed) != 2 {
		t.Errorf("바뀐 게시판 2개의 settings 를 돌려줘야 한다: %v", changed)
	}

	read := func(id string) map[string]json.RawMessage {
		var s string
		db.Raw(`SELECT settings FROM v2_board_extended_settings WHERE board_id = ?`, id).Scan(&s)
		m := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("%s settings 파싱 실패: %v (%s)", id, err, s)
		}
		return m
	}
	a := read("board_a")
	if string(a["write_notice"]) != `{"text":"안내"}` {
		t.Errorf("lucky 외 키는 그대로여야 한다: %s", a["write_notice"])
	}
	for id, m := range map[string]map[string]json.RawMessage{"board_a": a, "board_b": read("board_b")} {
		var got BoardLucky
		if err := json.Unmarshal(m["lucky"], &got); err != nil || !got.Enabled || got.Odds != 3 || got.Points != 5 || len(got.Prizes) != 1 {
			t.Errorf("%s lucky 가 새 값이어야 한다: %s", id, m["lucky"])
		}
	}
	if c := read("board_c"); c["lucky"] != nil || string(c["write_notice"]) != `{"text":"그대로"}` {
		t.Errorf("고르지 않은 게시판은 그대로: %v", c)
	}

	// 지급 경로(GetBoardLucky)가 새 값을 그대로 읽는다.
	odds := (&luckyRepository{db: db}).GetBoardLucky("board_b")
	if odds.Odds != 3 || odds.CommentOdds != 0 || !odds.Payable() {
		t.Errorf("지급 경로가 새 값을 읽어야 한다: %+v", odds)
	}

	exists, err := repo.ExistingBoardIDs([]string{"board_a", "nope"})
	if err != nil || !exists["board_a"] || exists["nope"] {
		t.Errorf("존재 확인: %v %v", exists, err)
	}
	list, err := repo.ListBoardLucky()
	if err != nil || len(list) != 3 || list[2].BoardID != "board_c" || list[2].Lucky != nil || list[0].Lucky == nil || list[0].Subject != "가" {
		t.Errorf("게시판 목록: %+v %v", list, err)
	}
}

// TestListLuckyGrantsBetween_KSTBoundary — A4: [start, end) 경계와 보조 정보(문구·경험치·닉네임).
func TestListLuckyGrantsBetween_KSTBoundary(t *testing.T) {
	repo, db := newLuckyAdminTestRepo(t)
	dayStart := kstDayStartUTC(time.Date(2026, 10, 1, 12, 0, 0, 0, luckyKST)) // KST 10/01 00:00
	dayEnd := dayStart.Add(24 * time.Hour)
	poID := int64(1)
	if err := db.Exec(`INSERT INTO g5_point (po_id, po_content) VALUES (1, '고정가 럭키 포인트')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO g5_na_xp (xp_point, xp_content, xp_rel_table, xp_rel_id, xp_rel_action) VALUES
		(6, '앙팡타임 럭키 경험치(댓글)', 'free', '12', '@lucky'),
		(99, '글쓰기', 'free', '12', '@write')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_nick) VALUES ('member_a','닉가'),('member_b','닉나')`).Error; err != nil {
		t.Fatal(err)
	}
	for _, g := range []LuckyGrant{
		{MbID: "member_a", SourceTable: "free", SourceID: "10", Kind: LuckyKindPost, Amount: 3, CreatedAt: dayStart.Add(-time.Second)}, // 전날 23:59:59
		{MbID: "member_a", SourceTable: "free", SourceID: "11", Kind: LuckyKindPost, Amount: 4, PoID: &poID, CreatedAt: dayStart},      // 00:00:00 포함
		{MbID: "member_b", SourceTable: "free", SourceID: "12", Kind: LuckyKindComment, Amount: 0, CreatedAt: dayEnd.Add(-time.Second)},
		{MbID: "member_b", SourceTable: "free", SourceID: "13", Kind: LuckyKindPost, Amount: 5, CreatedAt: dayEnd}, // 다음날 00:00 제외
	} {
		if err := db.Create(&g).Error; err != nil {
			t.Fatalf("원장 시드 실패: %v", err)
		}
	}
	got, err := repo.ListLuckyGrantsBetween(dayStart, dayEnd)
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("그날 것만 2건이어야 한다, got %d: %+v", len(got), got)
	}
	ids := map[string]LuckyGrantDetail{}
	for _, g := range got {
		ids[g.SourceID] = g
	}
	if g := ids["11"]; g.PointContent != "고정가 럭키 포인트" || g.Nick != "닉가" || g.Amount != 4 {
		t.Errorf("포인트 문구·닉네임: %+v", g)
	}
	if g := ids["12"]; g.Exp != 6 || g.ExpContent != "앙팡타임 럭키 경험치(댓글)" || g.Nick != "닉나" {
		t.Errorf("경험치는 @lucky 행만: %+v", g)
	}
}
