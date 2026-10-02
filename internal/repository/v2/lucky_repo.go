package v2

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	sqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 나리야 럭키 포인트 상수. 지급 로그(g5_point)의 rel_action 은 레거시 @lucky 와 동일하게
// 맞춰 마이페이지 포인트 내역에서 기존 럭키 지급과 함께 묶여 보이도록 한다.
const (
	luckyRelAction    = "@lucky"
	luckyPointContent = "나리야 럭키 포인트"
	// luckyExpContent 는 단계 이름이 없을 때의 경험치 내역(g5_na_xp.xp_content) 문구다.
	luckyExpContent = "나리야 럭키 경험치"
	// luckyCommentContentSuffix 는 댓글 당첨 내역 문구 끝에 붙는다(「<단계> 럭키 포인트(댓글)」).
	luckyCommentContentSuffix = "(댓글)"
	// luckyNeverExpireDate 는 만료 없는 포인트의 po_expire_date 다(그누보드 관례, 만료 크론이 건너뛴다).
	luckyNeverExpireDate = "9999-12-31"
)

// 원장(g5_da_lucky_grant.kind) 종류. 사이트 하루 상한은 종류별로 따로 세고, 회원 하루 상한은 종류를 합쳐 센다.
// 둘 다 g5_point 지급을 동반한다. UNIQUE(source_table, source_id, kind) 가 종류별 이중지급을 막는다.
const (
	// LuckyKindPost 는 글 당첨이다(기존 행은 모두 이 값).
	LuckyKindPost = "point"
	// LuckyKindComment 는 댓글 당첨이다.
	LuckyKindComment = "cpoint"
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
// 평소 단계(BaseName, 기본 「앙팡」) 확률·금액은 전역이 아니라 **게시판별**(v2_board_extended_settings.lucky)로 정한다(GetBoardLucky).
//
// ⛔ 키가 없으면 「무제한」이 아니라 DefaultLuckyConfig 의 값(댓글 제외·회원 1·글 10·댓글 20·댓글 10자·만료 365일·시간대 없음)이다.
// 그래서 정수 필드는 0 과 「키 없음」을 구분해야 하고, 그 구분은 UnmarshalJSON 이 기본값을 먼저 깔아 둔다.
// 0 은 「명시적으로 무제한」이다. 저장 시 0 이 사라지면 다음 읽기에서 기본값으로 바뀌므로 omitempty 를 달지 않는다.
type LuckyConfig struct {
	Enabled         bool `json:"enabled"`          // 마스터 스위치 (default: false)
	IncludeComments bool `json:"include_comments"` // 댓글에도 발동할지 (default: false=글만)
	MemberDailyCap  int  `json:"member_daily_cap"` // 회원당 KST 하루 당첨 상한, 글+댓글 합산 (default: 1, 0=무제한)
	// DailyCap 은 예전 키다. daily_cap_post 가 없을 때만 글 상한으로 쓴다(하위호환).
	DailyCap        int `json:"daily_cap"`         // (default: 10, 0=무제한)
	DailyCapPost    int `json:"daily_cap_post"`    // 사이트 전체 KST 하루 글 당첨 상한 (default: daily_cap 또는 10, 0=무제한)
	DailyCapComment int `json:"daily_cap_comment"` // 사이트 전체 KST 하루 댓글 당첨 상한 (default: 20, 0=무제한)
	// MinCommentChars 는 댓글이 발동 대상이 되는 최소 길이(정리 후 글자 수)다. 글에는 적용하지 않는다.
	MinCommentChars int `json:"min_comment_chars"` // (default: 10, 0=제한 없음)
	// ExpireDays 는 당첨 포인트의 유효기간(일)이다. 만료일 = 지급 시각의 KST 날짜 + ExpireDays.
	ExpireDays int `json:"expire_days"` // (default: 365, 0=만료 없음 9999-12-31)
	// BaseName 은 평소 단계(시간대 밖, 게시판 값으로 지급) 이름이다. 지급 문구 「<BaseName> 럭키 포인트/경험치」와
	// 배지 tier 에 쓰인다. 키가 없거나 비면 gnurepo.LuckyDefaultBaseName(「앙팡」). 예전 이름 「앙복타임」 문구는
	// 배지·집계에서 이 이름으로 보고한다(표시 별칭, 원장은 그대로).
	BaseName string `json:"base_name"` // default: "앙팡"
	// 시간대 단계(앙팡타임 등)를 놓을 수 있는 KST 시 범위 [WindowStartHour, WindowEndHour).
	WindowStartHour int `json:"window_start_hour"` // default: 9
	WindowEndHour   int `json:"window_end_hour"`   // default: 23
	// Windows 는 하루 한 번씩 무작위(서버 비밀값으로 결정적) 시각에 열리는 단계들이다.
	// 비어 있으면 평소 단계(BaseName, 게시판 설정)만 쓴다.
	Windows []LuckyWindow `json:"windows"`
	// FixedWindows 는 매일 같은 KST 시각에 열리는 고정 시간대다(예: 저트래픽 시간 가중치).
	// 무작위 window 와 달리 시각이 설정에 그대로 있다 — 공개돼도 되는 값이라서다.
	// 무작위 window 에 걸리면 그쪽이 먼저고, 고정 시간대끼리 겹치면 배열 앞쪽이 이긴다. 비어 있으면 기존과 같다.
	FixedWindows []LuckyFixedWindow `json:"fixed_windows"`
}

// LuckyFixedWindow 는 고정 시간대 하나다. Start·End 는 KST "HH:MM" 이고 [Start, End) 이다(시작 포함·끝 제외).
// End 가 Start 보다 이르면 자정을 넘는 구간으로 본다(예: 23:00~01:00). Start==End 는 빈 구간이라 열지 않는다.
type LuckyFixedWindow struct {
	Name        string       `json:"name"`         // 단계 이름. 내역 문구·배지 tier 에 그대로 쓰인다
	Start       string       `json:"start"`        // KST "HH:MM" (포함)
	End         string       `json:"end"`          // KST "HH:MM" (제외)
	Odds        int          `json:"odds"`         // 글 당첨확률 분모. 1 미만이면 이 시간대를 열지 않는다
	CommentOdds int          `json:"comment_odds"` // 댓글 당첨확률 분모. 1 미만이면 이 시간대에서 댓글 미발동
	Points      int          `json:"points"`       // 상품 표가 없을 때 1..Points 포인트
	Prizes      []LuckyPrize `json:"prizes"`       // 상품 표(비면 Points 로 포인트만)
}

// BaseTierName 은 평소 단계 이름이다. 비었거나 nil 이면 기본값(「앙팡」).
func (c *LuckyConfig) BaseTierName() string {
	if c != nil {
		if b := strings.TrimSpace(c.BaseName); b != "" {
			return b
		}
	}
	return gnurepo.LuckyDefaultBaseName
}

// BadgeTierNames 는 배지·집계가 원장 문구에서 단계를 읽을 때 쓰는 이름 묶음(평소 단계 이름 + 설정 단계 이름)이다.
// nil 이면 기본값만 담는다.
func (c *LuckyConfig) BadgeTierNames() gnurepo.LuckyTierNames {
	return gnurepo.LuckyTierNames{Base: c.BaseTierName(), Names: c.TierNames()}
}

// TierNames 는 이 설정에서 정한 단계 이름(무작위 window + 고정 시간대)을 중복 없이 돌려준다.
// 배지가 원장 문구에서 단계를 읽을 때 쓰는 허용 목록이다 — 설정에 없는 이름으로 시작하는 문구(레거시 등)는
// 단계로 인정하지 않으려는 것이다. 기본 단계 이름(앙팡타임 등)과 평소 단계 이름은 배지 쪽이 따로 인정한다(BadgeTierNames).
func (c *LuckyConfig) TierNames() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(c.Windows)+len(c.FixedWindows))
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, w := range c.Windows {
		add(w.Name)
	}
	for _, f := range c.FixedWindows {
		add(f.Name)
	}
	return out
}

// InvalidateLuckyConfigCache 는 이 파드의 lucky_config 30초 캐시를 비운다.
// 관리자 저장 직후 이 파드에서는 바로 새 값이 보이게 하려는 것이다. 다른 파드는 TTL(30초) 안에 따라온다.
func InvalidateLuckyConfigCache() {
	luckyConfigCacheMu.Lock()
	luckyConfigCacheVal = nil
	luckyConfigCacheExpAt = time.Time{}
	luckyConfigCacheMu.Unlock()
}

// LuckyWindow 는 하루 한 번 열리는 시간대 단계 하나다. 길이·확률·금액은 설정에서 정한다.
// 시작 시각은 설정에 두지 않는다 — 회원이 설정·응답으로 알아낼 수 없게 서버가 날짜별로 정한다.
type LuckyWindow struct {
	Name    string `json:"name"`    // 단계 이름. 당첨 포인트 내역 문구에 들어간다
	Minutes int    `json:"minutes"` // 열려 있는 길이(분)
	Odds    int    `json:"odds"`    // 당첨확률 = 1/Odds (쌍주사위)
	Points  int    `json:"points"`  // 당첨 시 1..Points 지급(글·댓글 공통)
	// CommentOdds 는 이 단계의 댓글 당첨확률 분모다. 없거나 1 미만이면 이 단계에서 댓글은 발동하지 않는다.
	CommentOdds int `json:"comment_odds"`
	// Prizes 는 이 단계의 상품 표다. 비어 있으면 Points 로 포인트만 준다(하위호환).
	Prizes []LuckyPrize `json:"prizes"`
}

// LuckyPrize 는 상품 표의 한 줄이다. 당첨되면 Weight 비율로 한 줄을 고른다(weight<=0 인 줄은 무시).
// Points 는 최대 금액이고 실제 지급은 1..Points 균등이다(0 이하 = 포인트 없음).
// Exp 는 고정 경험치다(0 이하 = 경험치 없음). 둘 다 0 인 줄은 「꽝」이다.
type LuckyPrize struct {
	Weight int `json:"weight"`
	Points int `json:"points"`
	Exp    int `json:"exp"`
}

// HasPayablePrize 는 상품 표에 무언가를 주는 줄(weight>0 이고 포인트나 경험치가 있는 줄)이 있는지 본다.
func HasPayablePrize(prizes []LuckyPrize) bool {
	for _, p := range prizes {
		if p.Weight > 0 && (p.Points > 0 || p.Exp > 0) {
			return true
		}
	}
	return false
}

// 기본값. 「설정 키 없음」이 무제한으로 읽히지 않도록 한곳에 모은다.
const (
	defaultLuckyMemberDailyCap  = 1
	defaultLuckyDailyCap        = 10
	defaultLuckyDailyCapPost    = 10
	defaultLuckyDailyCapComment = 20
	defaultLuckyMinCommentChars = 10
	defaultLuckyExpireDays      = 365
	defaultLuckyWindowStartHour = 9
	defaultLuckyWindowEndHour   = 23
)

// DefaultLuckyConfig returns the default lucky configuration
// (disabled, 글만, 회원 1·글 10·댓글 20, 댓글 10자 이상, 만료 365일, 시간대 없음, 평소 단계 「앙팡」).
func DefaultLuckyConfig() *LuckyConfig {
	return &LuckyConfig{
		Enabled:         false,
		IncludeComments: false,
		MemberDailyCap:  defaultLuckyMemberDailyCap,
		DailyCap:        defaultLuckyDailyCap,
		DailyCapPost:    defaultLuckyDailyCapPost,
		DailyCapComment: defaultLuckyDailyCapComment,
		MinCommentChars: defaultLuckyMinCommentChars,
		ExpireDays:      defaultLuckyExpireDays,
		BaseName:        gnurepo.LuckyDefaultBaseName,
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
	// daily_cap_post 가 없으면 예전 daily_cap 을 글 상한으로 쓴다(둘 다 없으면 기본값이 남아 있다).
	var present struct {
		DailyCapPost *int `json:"daily_cap_post"`
		DailyCap     *int `json:"daily_cap"`
	}
	// 위에서 같은 입력이 이미 파싱됐다. 그래도 실패하면 이 하위호환 단계만 건너뛴다.
	if err := json.Unmarshal(b, &present); err == nil && present.DailyCapPost == nil && present.DailyCap != nil {
		v.DailyCapPost = v.DailyCap
	}
	if v.DailyCapPost < 0 {
		v.DailyCapPost = defaultLuckyDailyCapPost
	}
	if v.DailyCapComment < 0 {
		v.DailyCapComment = defaultLuckyDailyCapComment
	}
	if v.MinCommentChars < 0 {
		v.MinCommentChars = defaultLuckyMinCommentChars
	}
	if v.ExpireDays < 0 {
		v.ExpireDays = defaultLuckyExpireDays
	}
	// 평소 단계 이름이 비면(키가 "" 등) 기본값. 이름 없는 지급 문구가 생기지 않게 한다.
	v.BaseName = strings.TrimSpace(v.BaseName)
	if v.BaseName == "" {
		v.BaseName = gnurepo.LuckyDefaultBaseName
	}
	*c = LuckyConfig(v)
	return nil
}

// BoardLucky is the PER-BOARD lucky setting read from v2_board_extended_settings.settings.lucky
// (관리자가 게시판 편집 화면에서 설정). enabled=true 이고 지급할 것(points>=1 또는 주는 줄이 있는 prizes)이
// 있으며 그 종류의 확률(글 odds, 댓글 comment_odds)이 1 이상일 때만 그 종류가 발동한다.
type BoardLucky struct {
	Enabled     bool `json:"enabled"`      // 게시판별 사용 여부 (default: false → 미발동)
	Points      int  `json:"points"`       // 당첨 시 1..Points 지급(글·댓글 공통, prizes 가 없을 때)
	Odds        int  `json:"odds"`         // 글 당첨확률 = 1/Odds (쌍주사위)
	CommentOdds int  `json:"comment_odds"` // 댓글 당첨확률 = 1/CommentOdds. 없거나 1 미만이면 댓글 미발동
	// Prizes 는 평소 단계 상품 표다. 비어 있으면 Points 로 포인트만 준다(하위호환).
	Prizes []LuckyPrize `json:"prizes"`
}

// BoardLuckyOdds 는 GetBoardLucky 결과다. 0 은 「그 종류는 이 게시판에서 발동하지 않음」이다.
type BoardLuckyOdds struct {
	Odds        int          // 글 확률 분모(평소 단계)
	CommentOdds int          // 댓글 확률 분모(평소 단계)
	Points      int          // 최대 금액(글·댓글 공통, Prizes 가 없을 때)
	Prizes      []LuckyPrize // 평소 단계 상품 표(없으면 Points 로 포인트만)
}

// Payable 은 이 게시판 설정에 지급할 것이 있는지(레거시 points 또는 상품 표) 본다.
func (b BoardLuckyOdds) Payable() bool {
	return b.Points >= 1 || HasPayablePrize(b.Prizes)
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
	// GetBoardLucky returns the per-board lucky setting (글·댓글 확률, 금액) if the board has
	// lucky enabled; otherwise the zero value (=미발동). 확률·금액은 게시판별로 다르다.
	GetBoardLucky(boardSlug string) BoardLuckyOdds
}

// LuckyTierNamesFrom 는 설정 읽기 함수에서 배지용 단계 이름 묶음을 뽑는다(캐시된 설정을 쓰므로 요청마다 DB 를 치지 않는다).
// 읽기에 실패하면 제로 값 — 배지는 기본 단계 이름만 인정하고 평소 단계는 기본값으로 본다(표시만 줄 뿐 지급과는 무관하다).
func LuckyTierNamesFrom(get func() (*LuckyConfig, error)) gnurepo.LuckyTierNames {
	if get == nil {
		return gnurepo.LuckyTierNames{}
	}
	cfg, err := get()
	if err != nil || cfg == nil {
		return gnurepo.LuckyTierNames{}
	}
	return cfg.BadgeTierNames()
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
	// DailyCap 은 사이트 전체 KST 하루 지급 상한이다. 같은 kind 끼리만 센다(글·댓글 상한이 서로 독립). 0 이하 = 확인 안 함.
	DailyCap int
	// TierName 은 단계 이름(앙팡·앙팡타임 등)이다. 포인트 내역 문구 앞에 붙는다. 비면 기존 문구.
	TierName string
	// Now 는 판정·기록 기준 시각이다. zero 면 time.Now(). 테스트가 하루 경계를 고정하려고 둔다.
	Now time.Time
	// ExpireDays 는 g5_point 만료일 = Now 의 KST 날짜 + ExpireDays 일이다. 0 이하 = 만료 없음(9999-12-31, 기존 Grant 동작).
	ExpireDays int
	// Exp 는 같은 트랜잭션에서 함께 줄 경험치(g5_na_xp + as_exp/as_level)다. 0 이하 = 경험치 없음(기존 동작).
	// 포인트(amount)와 경험치는 원장 1행으로 묶인다 — amount 가 0 이고 Exp 만 있어도 원장은 1행이다.
	Exp int
	// ExpFallbackPoints 는 회원이 경험치를 받을 수 없을 때(AddExp 규칙상 적립 0) 「경험치만」 상품을 대신할
	// 포인트 최대 금액 N 이다(1..N 균등). 0 이하이면 대체하지 않고 지급 없이 끝난다(GrantOutcomeNothing).
	// 「둘 다」 상품이면 대체 없이 포인트만 준다.
	ExpFallbackPoints int
	// RandN 은 대체 포인트 금액에 쓰는 [0, n) 균등 난수다. nil 이면 crypto/rand.
	RandN func(n int) int
}

// errLuckyNothingToGrant 는 포인트도 경험치도 없는 지급 요청이다. 원장을 쓰지 않는다.
var errLuckyNothingToGrant = errors.New("lucky: nothing to grant (amount<=0 and exp<=0)")

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
	// GrantOutcomeNothing 은 경험치를 받을 수 없는 회원의 「경험치만」 당첨인데 대체 포인트도 없어 아무것도 쓰지 않았다는 뜻이다.
	GrantOutcomeNothing GrantOutcome = "nothing"
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
// 회원 상한은 종류(kind)를 합쳐 세고, 사이트 상한은 같은 kind 만 센다.
// 상한 확인은 오늘(KST 0시 이후) 원장 행을 SELECT ... FOR UPDATE 로 잠그고 센다. 당첨자만 여기까지
// 오므로(하루 수십 건 이하) 잠금 경합은 사실상 없고, 동시에 두 당첨이 들어와도 범위 잠금 때문에
// 한쪽이 기다렸다가 늘어난 건수를 보고 멈춘다 — 상한을 넘겨 쓰는 일이 없다.
//
// ⭐ 시각 비교는 created_at 에 쓰는 값(Now)과 같은 time.Time 바인딩 경로로 한다. DSN 의 loc 이 무엇이든
// 쓰기와 비교가 같은 변환을 거치므로 둘이 9시간 어긋나지 않는다.
//
// 지급 내용: amount>0 이면 g5_point(+mb_point), opt.Exp>0 이면 g5_na_xp(+as_exp, as_level 재계산).
// 둘 다 원장 INSERT 와 같은 트랜잭션이라 원장 UNIQUE 하나가 포인트·경험치 이중지급을 함께 막는다.
// 원장 amount 는 포인트 금액이다(경험치만이면 0). 경험치의 근거는 g5_na_xp(xp_rel_action=@lucky)다.
// ⛔ mb_level 은 건드리지 않는다 — as_level 만 바뀐다.
func (r *luckyRepository) GrantWithOptions(mbID, sourceTable, sourceID, kind string, amount int, opt GrantOptions) (GrantOutcome, error) {
	if amount < 0 {
		amount = 0
	}
	if amount == 0 && opt.Exp <= 0 {
		return "", errLuckyNothingToGrant
	}
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
			n, err := countLuckyGrantsLocked(tx, mbID, "", dayStart) // 글+댓글 합산
			if err != nil {
				return err
			}
			if n >= opt.MemberDailyCap {
				outcome = GrantOutcomeCappedMember
				return nil
			}
		}
		if opt.DailyCap > 0 {
			n, err := countLuckyGrantsLocked(tx, "", kind, dayStart) // 종류별
			if err != nil {
				return err
			}
			if n >= opt.DailyCap {
				outcome = GrantOutcomeCappedDaily
				return nil
			}
		}

		// 경험치 가능 여부는 상한 확인 뒤, 원장 INSERT 전에 tx 안에서 회원 행을 읽어 정한다(addExpTx 와 같은 조건).
		// 받을 수 없으면 「경험치만」은 대체 포인트(1..ExpFallbackPoints)로, 「둘 다」는 포인트만으로 바꾼다.
		payPoints, payExp := amount, opt.Exp
		if payExp > 0 {
			blocked, err := luckyExpBlocked(tx, mbID, payExp, sourceTable)
			if err != nil {
				return err
			}
			if blocked {
				payExp = 0
				if payPoints == 0 {
					if opt.ExpFallbackPoints < 1 {
						outcome = GrantOutcomeNothing
						return nil
					}
					payPoints = luckyRandN(opt.RandN, opt.ExpFallbackPoints) + 1
				}
			}
		}

		ledger := &LuckyGrant{
			MbID:        mbID,
			SourceTable: sourceTable,
			SourceID:    sourceID,
			Kind:        kind,
			Amount:      payPoints,
			CreatedAt:   now,
		}
		if err := tx.Create(ledger).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 이미 지급됨 — 아무것도 하지 않고 커밋(no-op).
				return nil
			}
			return err
		}

		if kind == LuckyKindPost || kind == LuckyKindComment {
			isComment := kind == LuckyKindComment
			if payPoints > 0 {
				content := luckyPointContentFor(opt.TierName, isComment)
				poID, err := insertLuckyPoint(tx, mbID, payPoints, sourceTable, sourceID, content, now, luckyExpireDate(now, opt.ExpireDays))
				if err != nil {
					return err
				}
				if err := tx.Model(&LuckyGrant{}).
					Where("id = ?", ledger.ID).
					UpdateColumn("po_id", poID).Error; err != nil {
					return err
				}
			}
			if payExp > 0 {
				// 기존 AddExp 와 같은 규칙(최대 레벨·고레벨 제한·as_level 재계산)을 같은 tx 로 적용한다.
				content := luckyExpContentFor(opt.TierName, isComment)
				if _, err := addExpTx(tx, mbID, payExp, content, sourceTable, sourceID, luckyRelAction, now); err != nil {
					return err
				}
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

// countLuckyGrantsLocked 는 since 이후 원장 행 수를 FOR UPDATE 로 잠그며 센다. mbID 가 비면 모든 회원,
// kind 가 비면 모든 종류. 회원별은 idx_mb, 전체는 idx_created 를 탄다(DDL 추가 없음).
func countLuckyGrantsLocked(tx *gorm.DB, mbID, kind string, since time.Time) (int, error) {
	q := tx.Model(&LuckyGrant{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("created_at >= ?", since)
	if mbID != "" {
		q = q.Where("mb_id = ?", mbID)
	}
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var ids []int64
	if err := q.Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	return len(ids), nil
}

// luckyPointContentFor 는 g5_point.po_content 문구를 만든다. 단계 이름이 있으면 「<단계> 럭키 포인트」,
// 댓글이면 끝에 「(댓글)」을 붙인다. 당첨자가 마이페이지 내역에서 어느 단계였는지 보게 하려는 것이다.
// 배지·내역 조인은 po_rel_action(@lucky)·po_rel_table·po_rel_id 기준이다. 단, 배지의 단계 표시(lucky_tier)는 이 문구의
// 「<단계> 」 접두어에서 읽으므로 접두어 형식을 바꾸면 배지 쪽도 같이 바꿔야 한다. 시각은 넣지 않는다.
func luckyPointContentFor(tierName string, isComment bool) string {
	content := luckyPointContent
	if tierName = strings.TrimSpace(tierName); tierName != "" {
		content = tierName + " 럭키 포인트"
	}
	if isComment {
		content += luckyCommentContentSuffix
	}
	return content
}

// luckyExpBlocked 는 tx 안에서 회원 as_level 을 읽어, 이 경험치가 AddExp 규칙상 적립되지 않는지 본다
// (판정은 addExpTx 와 같은 expAccrualBlocked). 회원 행이 없으면 에러다(지급 전체 롤백).
func luckyExpBlocked(tx *gorm.DB, mbID string, point int, relTable string) (bool, error) {
	var member gnuboard.G5Member
	if err := tx.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return false, err
	}
	return expAccrualBlocked(member.AsLevel, point, relTable), nil
}

// luckyRandN 은 [0, n) 균등 난수다. rng 가 nil 이면 crypto/rand 를 쓴다. n 은 1 이상이어야 한다.
func luckyRandN(rng func(int) int, n int) int {
	if rng != nil {
		return rng(n)
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// luckyExpContentFor 는 g5_na_xp.xp_content 문구를 만든다. 「<단계> 럭키 경험치」, 댓글이면 끝에 「(댓글)」.
// 단계 이름이 없으면 기본 문구다. 배지 조인은 xp_rel_action(@lucky)·xp_rel_table·xp_rel_id 기준이고,
// 배지의 단계 표시(lucky_tier)는 이 문구의 「<단계> 」 접두어에서 읽는다 — 접두어 형식을 바꾸면 배지 쪽도 같이 바꿔야 한다.
func luckyExpContentFor(tierName string, isComment bool) string {
	content := luckyExpContent
	if tierName = strings.TrimSpace(tierName); tierName != "" {
		content = tierName + " 럭키 경험치"
	}
	if isComment {
		content += luckyCommentContentSuffix
	}
	return content
}

// luckyExpireDate 는 g5_point.po_expire_date(YYYY-MM-DD) 를 만든다 — now 의 KST 날짜 + days 일.
// days <= 0 이면 만료 없음(9999-12-31). 컨테이너 TZ 와 무관하게 KST 날짜로 계산한다
// (예: UTC 14:59 = KST 23:59 는 그 KST 날짜, UTC 15:00 = KST 다음날 00:00 은 다음 날짜가 기준).
func luckyExpireDate(now time.Time, days int) string {
	if days <= 0 {
		return luckyNeverExpireDate
	}
	k := now.In(luckyKST)
	return time.Date(k.Year(), k.Month(), k.Day(), 0, 0, 0, 0, luckyKST).AddDate(0, 0, days).Format("2006-01-02")
}

// insertLuckyPoint credits the member balance and writes a g5_point credit log within tx.
// Returns the new po_id for audit. content 는 단계 이름이 들어간 내역 문구, now 는 원장과 같은 기준 시각,
// expireDate 는 luckyExpireDate 결과(YYYY-MM-DD, 만료 없음이면 9999-12-31)다.
func insertLuckyPoint(tx *gorm.DB, mbID string, amount int, sourceTable, sourceID, content string, now time.Time, expireDate string) (int64, error) {
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
		PoExpireDate: expireDate,
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

// GetBoardLucky reads the per-board lucky setting. 캐시 없음(호출 빈도=글/댓글 작성).
func (r *luckyRepository) GetBoardLucky(boardSlug string) BoardLuckyOdds {
	var settingsJSON string
	err := r.db.Table("v2_board_extended_settings").
		Select("settings").Where("board_id = ?", boardSlug).
		Scan(&settingsJSON).Error
	if err != nil || settingsJSON == "" || settingsJSON == nullJSON {
		return BoardLuckyOdds{}
	}
	var w boardLuckyWrapper
	if err := json.Unmarshal([]byte(settingsJSON), &w); err != nil || w.Lucky == nil {
		return BoardLuckyOdds{}
	}
	return boardLuckyOddsOf(w.Lucky)
}

// boardLuckyOddsOf 는 게시판 설정을 판정용 값으로 줄인다. 꺼졌거나 points<1 이면 전부 0,
// 확률이 1 미만인 종류는 0(그 종류는 이 게시판에서 발동하지 않음)이다.
func boardLuckyOddsOf(b *BoardLucky) BoardLuckyOdds {
	if b == nil || !b.Enabled || (b.Points < 1 && !HasPayablePrize(b.Prizes)) {
		return BoardLuckyOdds{}
	}
	out := BoardLuckyOdds{Points: b.Points, Prizes: b.Prizes}
	if out.Points < 0 {
		out.Points = 0
	}
	if b.Odds >= 1 {
		out.Odds = b.Odds
	}
	if b.CommentOdds >= 1 {
		out.CommentOdds = b.CommentOdds
	}
	return out
}
