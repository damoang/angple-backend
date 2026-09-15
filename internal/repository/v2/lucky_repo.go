package v2

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	sqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// 나리야 럭키 포인트 상수. 지급 로그(g5_point)의 rel_action 은 레거시 @lucky 와 동일하게
// 맞춰 마이페이지 포인트 내역에서 기존 럭키 지급과 함께 묶여 보이도록 한다.
const (
	luckyKindPoint    = "point"
	luckyRelAction    = "@lucky"
	luckyPointContent = "나리야 럭키 포인트"
)

// luckyConfigCache caches LuckyConfig to avoid hitting site_settings on every post/comment.
var (
	luckyConfigCacheMu    sync.RWMutex
	luckyConfigCacheVal   *LuckyConfig
	luckyConfigCacheExpAt time.Time
)

const luckyConfigCacheTTL = 30 * time.Second

// LuckyConfig is the GLOBAL master switch for "나리야 럭키 포인트"
// (stored in site_settings.settings_json under the "lucky_config" key).
//
// Enabled 이 전체 킬스위치다. 기본값 false — 켜기 전에는 어느 게시판에서도 지급되지 않는다.
// 확률·금액은 전역이 아니라 **게시판별**(v2_board_extended_settings.lucky)로 정한다(GetBoardLucky).
type LuckyConfig struct {
	Enabled bool `json:"enabled"` // 마스터 스위치 (default: false)
}

// DefaultLuckyConfig returns the default lucky configuration (disabled).
func DefaultLuckyConfig() *LuckyConfig {
	return &LuckyConfig{Enabled: false}
}

// BoardLucky is the PER-BOARD lucky setting read from v2_board_extended_settings.settings.lucky
// (관리자가 게시판 편집 화면에서 설정). enabled=true 이고 odds>=1, points>=1 일 때만 발동한다 —
// 그래서 게시판마다 다른 가중치(소모임은 크게 등)를 줄 수 있다.
type BoardLucky struct {
	Enabled bool `json:"enabled"` // 게시판별 사용 여부 (default: false → 미발동)
	Points  int  `json:"points"`  // 당첨 시 1..Points 지급
	Odds    int  `json:"odds"`    // 당첨확률 = 1/Odds (쌍주사위)
}

// LuckyGrant maps the g5_da_lucky_grant idempotency ledger row.
type LuckyGrant struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	MbID        string    `gorm:"column:mb_id"`
	SourceTable string    `gorm:"column:source_table"`
	SourceID    string    `gorm:"column:source_id"`
	Kind        string    `gorm:"column:kind"`
	Amount      int       `gorm:"column:amount"`
	PoID        *int64    `gorm:"column:po_id"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName returns the table name for GORM
func (LuckyGrant) TableName() string {
	return "g5_da_lucky_grant"
}

// LuckyRepository handles idempotent "나리야 럭키 포인트" grants + config reads.
type LuckyRepository interface {
	// Grant records a lucky payout idempotently and applies it in a single transaction.
	// granted=false (with nil error) means the (sourceTable, sourceID, kind) was already
	// granted — a no-op — so this is safe to retry.
	Grant(mbID, sourceTable, sourceID, kind string, amount int) (granted bool, err error)
	// GetLuckyConfig returns the GLOBAL master switch (cached 30s).
	GetLuckyConfig() (*LuckyConfig, error)
	// GetBoardLucky returns the per-board lucky setting (odds/points) if the board has
	// lucky enabled; otherwise dice=0, maxAmount=0 (=미발동). 확률·금액은 게시판별로 다르다.
	GetBoardLucky(boardSlug string) (dice, maxAmount int)
}

type luckyRepository struct {
	db *gorm.DB
}

// NewLuckyRepository creates a new LuckyRepository
func NewLuckyRepository(db *gorm.DB) LuckyRepository {
	return &luckyRepository{db: db}
}

// Grant inserts the idempotency ledger row and, when kind == "point", credits the
// member balance + writes the g5_point log — all in ONE transaction. Atomicity +
// idempotency are both provided by the UNIQUE(source_table, source_id, kind) key:
// a duplicate insert short-circuits to granted=false (no-op) before any point moves.
func (r *luckyRepository) Grant(mbID, sourceTable, sourceID, kind string, amount int) (bool, error) {
	granted := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		ledger := &LuckyGrant{
			MbID:        mbID,
			SourceTable: sourceTable,
			SourceID:    sourceID,
			Kind:        kind,
			Amount:      amount,
		}
		if err := tx.Create(ledger).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 이미 지급됨 — 아무것도 하지 않고 커밋(no-op).
				return nil
			}
			return err
		}

		if kind == luckyKindPoint {
			poID, err := insertLuckyPoint(tx, mbID, amount, sourceTable, sourceID)
			if err != nil {
				return err
			}
			if err := tx.Model(&LuckyGrant{}).
				Where("id = ?", ledger.ID).
				UpdateColumn("po_id", poID).Error; err != nil {
				return err
			}
		}

		granted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return granted, nil
}

// insertLuckyPoint credits the member balance and writes a g5_point credit log within tx.
// Lucky points never expire (po_expire_date = 9999-12-31). Returns the new po_id for audit.
func insertLuckyPoint(tx *gorm.DB, mbID string, amount int, sourceTable, sourceID string) (int64, error) {
	// 회원 잔액 증가
	if err := tx.Table("g5_member").
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", amount)).Error; err != nil {
		return 0, err
	}

	// 스냅샷용 갱신 잔액
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return 0, err
	}

	entry := &gnuboard.G5Point{
		MbID:         mbID,
		PoDatetime:   time.Now(),
		PoContent:    luckyPointContent,
		PoPoint:      amount,
		PoUsePoint:   0,
		PoExpired:    0,
		PoExpireDate: "9999-12-31",
		PoRelTable:   sourceTable,
		PoRelID:      sourceID,
		PoRelAction:  luckyRelAction,
		MbPoint:      mbPoint,
	}
	if err := tx.Create(entry).Error; err != nil {
		return 0, err
	}
	return int64(entry.PoID), nil
}

// isDuplicateKeyErr reports whether err is a UNIQUE/PRIMARY key violation.
// TranslateError 가 켜져 있지 않을 수 있어 MySQL 에러번호 1062 를 1차 근거로 본다.
func isDuplicateKeyErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var myErr *sqldriver.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1062
	}
	return false
}

// GetLuckyConfig reads lucky configuration from site_settings.settings_json (cached 30s)
func (r *luckyRepository) GetLuckyConfig() (*LuckyConfig, error) {
	luckyConfigCacheMu.RLock()
	if luckyConfigCacheVal != nil && time.Now().Before(luckyConfigCacheExpAt) {
		cached := *luckyConfigCacheVal
		luckyConfigCacheMu.RUnlock()
		return &cached, nil
	}
	luckyConfigCacheMu.RUnlock()

	config, err := r.getLuckyConfigFromDB()
	if err != nil {
		return nil, err
	}

	luckyConfigCacheMu.Lock()
	cp := *config
	luckyConfigCacheVal = &cp
	luckyConfigCacheExpAt = time.Now().Add(luckyConfigCacheTTL)
	luckyConfigCacheMu.Unlock()

	return config, nil
}

// getLuckyConfigFromDB fetches LuckyConfig directly from database
func (r *luckyRepository) getLuckyConfigFromDB() (*LuckyConfig, error) {
	var row siteSettingsJSON
	err := r.db.Select("settings_json").Where("site_id = ?", defaultSiteID).First(&row).Error
	if err != nil {
		return DefaultLuckyConfig(), nil
	}

	if row.SettingsJSON == nil || *row.SettingsJSON == "" || *row.SettingsJSON == nullJSON {
		return DefaultLuckyConfig(), nil
	}

	var wrapper settingsJSONWrapper
	if err := json.Unmarshal([]byte(*row.SettingsJSON), &wrapper); err != nil {
		return DefaultLuckyConfig(), nil
	}

	if wrapper.LuckyConfig == nil {
		return DefaultLuckyConfig(), nil
	}

	return wrapper.LuckyConfig, nil
}

// boardLuckyWrapper parses only the "lucky" key of v2_board_extended_settings.settings.
type boardLuckyWrapper struct {
	Lucky *BoardLucky `json:"lucky"`
}

// GetBoardLucky reads the per-board lucky setting. 게시판이 럭키를 켰고(odds>=1, points>=1)
// 이면 (dice=odds, maxAmount=points) 를, 아니면 (0, 0) 을 돌려준다. 캐시 없음(호출 빈도=글/댓글 작성).
func (r *luckyRepository) GetBoardLucky(boardSlug string) (dice, maxAmount int) {
	var settingsJSON string
	err := r.db.Table("v2_board_extended_settings").
		Select("settings").Where("board_id = ?", boardSlug).
		Scan(&settingsJSON).Error
	if err != nil || settingsJSON == "" || settingsJSON == nullJSON {
		return 0, 0
	}
	var w boardLuckyWrapper
	if err := json.Unmarshal([]byte(settingsJSON), &w); err != nil || w.Lucky == nil {
		return 0, 0
	}
	if !w.Lucky.Enabled || w.Lucky.Odds < 1 || w.Lucky.Points < 1 {
		return 0, 0
	}
	return w.Lucky.Odds, w.Lucky.Points
}
