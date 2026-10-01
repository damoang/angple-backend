package v2

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	sqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
// 평소(앙복타임) 확률·금액은 전역이 아니라 **게시판별**(v2_board_extended_settings.lucky)로 정한다(GetBoardLucky).
//
// ⛔ 키가 없으면 「무제한」이 아니라 DefaultLuckyConfig 의 값(댓글 제외·회원 1·사이트 10·시간대 없음)이다.
// 그래서 정수 필드는 0 과 「키 없음」을 구분해야 하고, 그 구분은 UnmarshalJSON 이 기본값을 먼저 깔아 둔다.
// 0 은 「명시적으로 무제한」이다. 저장 시 0 이 사라지면 다음 읽기에서 기본값으로 바뀌므로 omitempty 를 달지 않는다.
type LuckyConfig struct {
	Enabled         bool `json:"enabled"`          // 마스터 스위치 (default: false)
	IncludeComments bool `json:"include_comments"` // 댓글에도 발동할지 (default: false=글만)
	MemberDailyCap  int  `json:"member_daily_cap"` // 회원당 KST 하루 당첨 상한 (default: 1, 0=무제한)
	DailyCap        int  `json:"daily_cap"`        // 사이트 전체 KST 하루 당첨 상한 (default: 10, 0=무제한)
	// 시간대 단계(앙팡타임 등)를 놓을 수 있는 KST 시 범위 [WindowStartHour, WindowEndHour).
	WindowStartHour int `json:"window_start_hour"` // default: 9
	WindowEndHour   int `json:"window_end_hour"`   // default: 23
	// Windows 는 하루 한 번씩 무작위(서버 비밀값으로 결정적) 시각에 열리는 단계들이다.
	// 비어 있으면 앙복타임(게시판 설정)만 쓴다.
	Windows []LuckyWindow `json:"windows"`
}

// LuckyWindow 는 하루 한 번 열리는 시간대 단계 하나다. 길이·확률·금액은 설정에서 정한다.
// 시작 시각은 설정에 두지 않는다 — 회원이 설정·응답으로 알아낼 수 없게 서버가 날짜별로 정한다.
type LuckyWindow struct {
	Name    string `json:"name"`    // 단계 이름. 당첨 포인트 내역 문구에 들어간다
	Minutes int    `json:"minutes"` // 열려 있는 길이(분)
	Odds    int    `json:"odds"`    // 당첨확률 = 1/Odds (쌍주사위)
	Points  int    `json:"points"`  // 당첨 시 1..Points 지급
}

// 기본값. 「설정 키 없음」이 무제한으로 읽히지 않도록 한곳에 모은다.
const (
	defaultLuckyMemberDailyCap  = 1
	defaultLuckyDailyCap        = 10
	defaultLuckyWindowStartHour = 9
	defaultLuckyWindowEndHour   = 23
)

// DefaultLuckyConfig returns the default lucky configuration (disabled, 글만, 회원 1·사이트 10, 시간대 없음).
func DefaultLuckyConfig() *LuckyConfig {
	return &LuckyConfig{
		Enabled:         false,
		IncludeComments: false,
		MemberDailyCap:  defaultLuckyMemberDailyCap,
		DailyCap:        defaultLuckyDailyCap,
		WindowStartHour: defaultLuckyWindowStartHour,
		WindowEndHour:   defaultLuckyWindowEndHour,
	}
}

// UnmarshalJSON 은 기본값 위에 들어온 키만 덮어쓴다 — 없는 키는 기본값이 남는다.
//
// ⛔ 파싱에 실패해도 에러를 돌려주지 않고 「꺼짐」으로 둔다. lucky_config 는 settings_json 전체를
// 한 번에 푸는 settingsJSONWrapper 안에 있어서, 여기서 에러가 나면 xp_config·point_config 까지
// 같이 기본값으로 떨어진다. 럭키 설정 오타 하나가 경험치 설정을 날리면 안 된다.
func (c *LuckyConfig) UnmarshalJSON(b []byte) error {
	type plain LuckyConfig // 메서드 없는 별칭 — 재귀 호출 방지
	v := plain(*DefaultLuckyConfig())
	if err := json.Unmarshal(b, &v); err != nil {
		*c = *DefaultLuckyConfig() // Enabled=false — 잘못된 설정으로는 지급하지 않는다
		return nil
	}
	// 음수는 의미가 없다. 무제한으로 새지 않게 기본값으로 되돌린다.
	if v.MemberDailyCap < 0 {
		v.MemberDailyCap = defaultLuckyMemberDailyCap
	}
	if v.DailyCap < 0 {
		v.DailyCap = defaultLuckyDailyCap
	}
	*c = LuckyConfig(v)
	return nil
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
	// GrantWithOptions is Grant plus 하루 상한(회원·사이트)과 단계 이름. 상한에 닿으면 원장·포인트를
	// 하나도 쓰지 않고 GrantOutcomeCappedMember/GrantOutcomeCappedDaily 를 nil 에러와 함께 돌려준다.
	GrantWithOptions(mbID, sourceTable, sourceID, kind string, amount int, opt GrantOptions) (GrantOutcome, error)
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

// GrantOptions 는 Grant 에 얹는 선택 조건이다. 0 값(GrantOptions{})이면 기존 Grant 와 똑같이 동작한다.
type GrantOptions struct {
	// MemberDailyCap 은 같은 회원의 KST 하루 지급 상한이다. 0 이하 = 확인 안 함.
	MemberDailyCap int
	// DailyCap 은 사이트 전체 KST 하루 지급 상한이다. 0 이하 = 확인 안 함.
	DailyCap int
	// TierName 은 단계 이름(앙복타임·앙팡타임 등)이다. 포인트 내역 문구 앞에 붙는다. 비면 기존 문구.
	TierName string
	// Now 는 판정·기록 기준 시각이다. zero 면 time.Now(). 테스트가 하루 경계를 고정하려고 둔다.
	Now time.Time
}

// GrantOutcome 은 GrantWithOptions 의 결과 종류다. 호출부가 상한 로그를 남길 수 있게 「왜 안 줬는지」를 구분한다.
type GrantOutcome string

// GrantWithOptions 결과 값.
const (
	// GrantOutcomeGranted 는 원장·포인트를 기록했다는 뜻이다.
	GrantOutcomeGranted GrantOutcome = "granted"
	// GrantOutcomeDuplicate 는 같은 (source_table, source_id, kind) 가 이미 지급돼 no-op 이라는 뜻이다.
	GrantOutcomeDuplicate GrantOutcome = "duplicate"
	// GrantOutcomeCappedMember 는 회원 하루 상한에 닿아 아무것도 쓰지 않았다는 뜻이다.
	GrantOutcomeCappedMember GrantOutcome = "capped_member"
	// GrantOutcomeCappedDaily 는 사이트 하루 상한에 닿아 아무것도 쓰지 않았다는 뜻이다.
	GrantOutcomeCappedDaily GrantOutcome = "capped_daily"
)

// luckyKST 는 「하루」의 기준 시간대다. time.Local·LoadLocation 에 기대지 않고 코드에 고정한다
// (컨테이너 TZ 설정과 무관하게 같은 경계가 나와야 한다 — discipline_release 의 9시간 사고 참조).
var luckyKST = time.FixedZone("KST", 9*60*60)

// kstDayStartUTC 는 now 가 속한 KST 날짜의 00:00 을 UTC 시각으로 돌려준다.
// 예) 2026-10-01 14:59 UTC(=KST 23:59) → 2026-09-30 15:00 UTC, 15:00 UTC(=KST 10/02 00:00) → 2026-10-01 15:00 UTC.
func kstDayStartUTC(now time.Time) time.Time {
	k := now.In(luckyKST)
	return time.Date(k.Year(), k.Month(), k.Day(), 0, 0, 0, 0, luckyKST).UTC()
}

// Grant inserts the idempotency ledger row and, when kind == "point", credits the
// member balance + writes the g5_point log — all in ONE transaction. Atomicity +
// idempotency are both provided by the UNIQUE(source_table, source_id, kind) key:
// a duplicate insert short-circuits to granted=false (no-op) before any point moves.
//
// 상한 없이 지급한다(기존 동작). 상한이 필요하면 GrantWithOptions 를 쓴다.
func (r *luckyRepository) Grant(mbID, sourceTable, sourceID, kind string, amount int) (bool, error) {
	outcome, err := r.GrantWithOptions(mbID, sourceTable, sourceID, kind, amount, GrantOptions{})
	if err != nil {
		return false, err
	}
	return outcome == GrantOutcomeGranted, nil
}

// GrantWithOptions 는 Grant 와 같은 단일 트랜잭션 안에서 먼저 하루 상한을 센다.
//
// 상한 확인은 오늘(KST 0시 이후) 원장 행을 SELECT ... FOR UPDATE 로 잠그고 센다. 당첨자만 여기까지
// 오므로(하루 수십 건 이하) 잠금 경합은 사실상 없고, 동시에 두 당첨이 들어와도 범위 잠금 때문에
// 한쪽이 기다렸다가 늘어난 건수를 보고 멈춘다 — 상한을 넘겨 쓰는 일이 없다.
//
// ⭐ 시각 비교는 created_at 에 쓰는 값(Now)과 같은 time.Time 바인딩 경로로 한다. DSN 의 loc 이 무엇이든
// 쓰기와 비교가 같은 변환을 거치므로 둘이 9시간 어긋나지 않는다.
func (r *luckyRepository) GrantWithOptions(mbID, sourceTable, sourceID, kind string, amount int, opt GrantOptions) (GrantOutcome, error) {
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}
	// DATETIME(초) 는 소수 초를 반올림한다 — 23:59:59.7 이 다음 날로 넘어가지 않게 잘라 둔다.
	now = now.Truncate(time.Second)
	dayStart := kstDayStartUTC(now)

	outcome := GrantOutcomeDuplicate
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if opt.MemberDailyCap > 0 {
			n, err := countLuckyGrantsLocked(tx, mbID, dayStart)
			if err != nil {
				return err
			}
			if n >= opt.MemberDailyCap {
				outcome = GrantOutcomeCappedMember
				return nil
			}
		}
		if opt.DailyCap > 0 {
			n, err := countLuckyGrantsLocked(tx, "", dayStart)
			if err != nil {
				return err
			}
			if n >= opt.DailyCap {
				outcome = GrantOutcomeCappedDaily
				return nil
			}
		}

		ledger := &LuckyGrant{
			MbID:        mbID,
			SourceTable: sourceTable,
			SourceID:    sourceID,
			Kind:        kind,
			Amount:      amount,
			CreatedAt:   now,
		}
		if err := tx.Create(ledger).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 이미 지급됨 — 아무것도 하지 않고 커밋(no-op).
				return nil
			}
			return err
		}

		if kind == luckyKindPoint {
			poID, err := insertLuckyPoint(tx, mbID, amount, sourceTable, sourceID, luckyPointContentFor(opt.TierName), now)
			if err != nil {
				return err
			}
			if err := tx.Model(&LuckyGrant{}).
				Where("id = ?", ledger.ID).
				UpdateColumn("po_id", poID).Error; err != nil {
				return err
			}
		}

		outcome = GrantOutcomeGranted
		return nil
	})
	if err != nil {
		return "", err
	}
	return outcome, nil
}

// countLuckyGrantsLocked 는 since 이후 원장 행 수를 FOR UPDATE 로 잠그며 센다. mbID 가 비면 사이트 전체.
// 회원별은 idx_mb, 전체는 idx_created 를 탄다(DDL 추가 없음).
func countLuckyGrantsLocked(tx *gorm.DB, mbID string, since time.Time) (int, error) {
	q := tx.Model(&LuckyGrant{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("created_at >= ?", since)
	if mbID != "" {
		q = q.Where("mb_id = ?", mbID)
	}
	var ids []int64
	if err := q.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return len(ids), nil
}

// luckyPointContentFor 는 g5_point.po_content 문구를 만든다. 단계 이름이 있으면 「<단계> 럭키 포인트」.
// 당첨자가 마이페이지 내역에서 어느 단계였는지 보게 하려는 것이다. 배지·내역 조인은 po_rel_action(@lucky)
// 기준이라 문구가 바뀌어도 영향이 없다. 시각은 넣지 않는다.
func luckyPointContentFor(tierName string) string {
	tierName = strings.TrimSpace(tierName)
	if tierName == "" {
		return luckyPointContent
	}
	return tierName + " 럭키 포인트"
}

// insertLuckyPoint credits the member balance and writes a g5_point credit log within tx.
// Lucky points never expire (po_expire_date = 9999-12-31). Returns the new po_id for audit.
// content 는 단계 이름이 들어간 내역 문구, now 는 원장과 같은 기준 시각이다.
func insertLuckyPoint(tx *gorm.DB, mbID string, amount int, sourceTable, sourceID, content string, now time.Time) (int64, error) {
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
		PoDatetime:   now,
		PoContent:    content,
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
