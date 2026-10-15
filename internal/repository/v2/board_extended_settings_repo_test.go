package v2

import (
	"sync/atomic"
	"testing"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 게시판 확장 설정 프로세스 캐시 계약:
//   - 적중이면 쿼리하지 않는다. TTL 이 지나면 다시 읽는다.
//   - 조회 오류는 캐시하지 않는다. 「행 없음 → 기본값」은 캐시한다.
//   - Upsert·행운 관리자 게시판 설정 저장 뒤에는 그 게시판 키를 지운다.
//   - 반환값을 고쳐도 캐시는 그대로다.

// newBoardExtTestDB 는 쿼리 수를 세는 sqlite DB 를 연다. migrate 가 false 면 테이블을 만들지 않는다.
func newBoardExtTestDB(t *testing.T, migrate bool) (*gorm.DB, *atomic.Int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB 실패: %v", err)
	}
	sqlDB.SetMaxOpenConns(1) // :memory: 는 연결마다 다른 DB 다
	if migrate {
		if err := db.AutoMigrate(&v2domain.V2BoardExtendedSettings{}); err != nil {
			t.Fatalf("마이그레이션 실패: %v", err)
		}
	}
	var queries atomic.Int64
	if err := db.Callback().Query().Before("gorm:query").Register("test:count_queries", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatalf("콜백 등록 실패: %v", err)
	}
	boardExtSettings.reset()
	t.Cleanup(func() {
		boardExtSettings.reset()
		boardExtSettingsNow = time.Now
	})
	return db, &queries
}

func seedBoardExt(t *testing.T, db *gorm.DB, slug, settings string) {
	t.Helper()
	if err := db.Exec("INSERT INTO v2_board_extended_settings (board_id, settings, created_at, updated_at) VALUES (?, ?, ?, ?)",
		slug, settings, time.Now(), time.Now()).Error; err != nil {
		t.Fatalf("행 넣기 실패: %v", err)
	}
}

func TestBoardExtSettingsCacheHitSkipsQuery(t *testing.T) {
	db, queries := newBoardExtTestDB(t, true)
	seedBoardExt(t, db, "free", `{"a":1}`)
	repo := NewBoardExtendedSettingsRepository(db)

	first, err := repo.FindByBoardSlug("free")
	if err != nil || first.Settings != `{"a":1}` {
		t.Fatalf("첫 조회: %+v, %v", first, err)
	}
	before := queries.Load()
	second, err := repo.FindByBoardSlug("free")
	if err != nil || second.Settings != `{"a":1}` {
		t.Fatalf("두 번째 조회: %+v, %v", second, err)
	}
	if got := queries.Load() - before; got != 0 {
		t.Fatalf("적중인데 쿼리 %d 회", got)
	}
}

func TestBoardExtSettingsCacheNotFoundIsCached(t *testing.T) {
	db, queries := newBoardExtTestDB(t, true)
	repo := NewBoardExtendedSettingsRepository(db)

	got, err := repo.FindByBoardSlug("none")
	if err != nil || got.BoardID != "none" || got.Settings != "{}" {
		t.Fatalf("행 없음 기본값이 아님: %+v, %v", got, err)
	}
	before := queries.Load()
	got, err = repo.FindByBoardSlug("none")
	if err != nil || got.Settings != "{}" {
		t.Fatalf("두 번째 조회: %+v, %v", got, err)
	}
	if n := queries.Load() - before; n != 0 {
		t.Fatalf("행 없음 결과가 캐시되지 않음: 쿼리 %d 회", n)
	}
}

func TestBoardExtSettingsCacheExpiresAfterTTL(t *testing.T) {
	db, _ := newBoardExtTestDB(t, true)
	seedBoardExt(t, db, "free", `{"v":1}`)
	repo := NewBoardExtendedSettingsRepository(db)

	base := time.Now()
	boardExtSettingsNow = func() time.Time { return base }
	if _, err := repo.FindByBoardSlug("free"); err != nil {
		t.Fatalf("첫 조회: %v", err)
	}
	if err := db.Exec("UPDATE v2_board_extended_settings SET settings = ? WHERE board_id = ?", `{"v":2}`, "free").Error; err != nil {
		t.Fatalf("직접 갱신 실패: %v", err)
	}

	boardExtSettingsNow = func() time.Time { return base.Add(boardExtSettingsTTL - time.Second) }
	if got, _ := repo.FindByBoardSlug("free"); got.Settings != `{"v":1}` {
		t.Fatalf("TTL 안에서는 캐시 값이어야 함: %s", got.Settings)
	}
	boardExtSettingsNow = func() time.Time { return base.Add(boardExtSettingsTTL) }
	if got, _ := repo.FindByBoardSlug("free"); got.Settings != `{"v":2}` {
		t.Fatalf("TTL 뒤에는 다시 읽어야 함: %s", got.Settings)
	}
}

func TestBoardExtSettingsCacheErrorNotCached(t *testing.T) {
	db, _ := newBoardExtTestDB(t, false) // 테이블 없음 → 조회 오류
	repo := NewBoardExtendedSettingsRepository(db)

	if _, err := repo.FindByBoardSlug("free"); err == nil {
		t.Fatalf("테이블이 없는데 오류가 없음")
	}
	if err := db.AutoMigrate(&v2domain.V2BoardExtendedSettings{}); err != nil {
		t.Fatalf("마이그레이션 실패: %v", err)
	}
	seedBoardExt(t, db, "free", `{"ok":true}`)
	got, err := repo.FindByBoardSlug("free")
	if err != nil || got.Settings != `{"ok":true}` {
		t.Fatalf("오류가 캐시된 듯함: %+v, %v", got, err)
	}
}

func TestBoardExtSettingsCacheInvalidatedByUpsert(t *testing.T) {
	db, _ := newBoardExtTestDB(t, true)
	repo := NewBoardExtendedSettingsRepository(db)

	if got, _ := repo.FindByBoardSlug("free"); got.Settings != "{}" {
		t.Fatalf("초기값: %s", got.Settings)
	}
	if err := repo.Upsert(&v2domain.V2BoardExtendedSettings{BoardID: "free", Settings: `{"x":1}`}); err != nil {
		t.Fatalf("Upsert 실패: %v", err)
	}
	if got, _ := repo.FindByBoardSlug("free"); got.Settings != `{"x":1}` {
		t.Fatalf("생성 뒤 무효화 안 됨: %s", got.Settings)
	}
	if err := repo.Upsert(&v2domain.V2BoardExtendedSettings{BoardID: "free", Settings: `{"x":2}`}); err != nil {
		t.Fatalf("Upsert 갱신 실패: %v", err)
	}
	if got, _ := repo.FindByBoardSlug("free"); got.Settings != `{"x":2}` {
		t.Fatalf("갱신 뒤 무효화 안 됨: %s", got.Settings)
	}
}

func TestBoardExtSettingsCacheInvalidatedByLuckyAdmin(t *testing.T) {
	db, _ := newBoardExtTestDB(t, true)
	seedBoardExt(t, db, "free", `{"write_notice":"n"}`)
	repo := NewBoardExtendedSettingsRepository(db)
	if got, _ := repo.FindByBoardSlug("free"); got.Settings != `{"write_notice":"n"}` {
		t.Fatalf("초기값: %s", got.Settings)
	}

	admin := &luckyAdminRepository{db: db}
	if _, err := admin.SetBoardLucky([]string{"free"}, BoardLucky{}, time.Now()); err != nil {
		t.Fatalf("SetBoardLucky 실패: %v", err)
	}
	got, err := repo.FindByBoardSlug("free")
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if got.Settings == `{"write_notice":"n"}` {
		t.Fatalf("행운 관리자 저장 뒤 무효화 안 됨: %s", got.Settings)
	}
}

func TestBoardExtSettingsCacheReturnsCopy(t *testing.T) {
	db, _ := newBoardExtTestDB(t, true)
	seedBoardExt(t, db, "free", `{"a":1}`)
	repo := NewBoardExtendedSettingsRepository(db)

	first, err := repo.FindByBoardSlug("free")
	if err != nil {
		t.Fatalf("첫 조회: %v", err)
	}
	first.Settings = "changed"
	first.BoardID = "changed"

	second, err := repo.FindByBoardSlug("free")
	if err != nil {
		t.Fatalf("두 번째 조회: %v", err)
	}
	if second.Settings != `{"a":1}` || second.BoardID != "free" {
		t.Fatalf("반환값 수정이 캐시에 번짐: %+v", second)
	}
	second.Settings = "changed-again"
	third, _ := repo.FindByBoardSlug("free")
	if third.Settings != `{"a":1}` {
		t.Fatalf("적중 반환값 수정이 캐시에 번짐: %+v", third)
	}
}

func TestBoardExtSettingsCacheDropsLoadRacingInvalidation(t *testing.T) {
	db, _ := newBoardExtTestDB(t, true)
	key := boardExtSettingsKey{cfg: db.Config, slug: "free"}
	now := time.Now()

	_, ok, gen := boardExtSettings.get(key, now)
	if ok {
		t.Fatalf("빈 캐시인데 적중")
	}
	invalidateBoardExtSettings(db, "free") // 조회 도중 쓰기가 끼었다
	boardExtSettings.put(key, v2domain.V2BoardExtendedSettings{BoardID: "free", Settings: "stale"}, gen, now)
	if _, ok, _ := boardExtSettings.get(key, now); ok {
		t.Fatalf("무효화 전에 시작한 조회 결과가 캐시에 들어감")
	}
}
