package gnuboard

import (
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// luckyRelAction 은 럭키 당첨 기록(g5_point.po_rel_action, g5_na_xp.xp_rel_action)의 값이다.
const luckyRelAction = "@lucky"

// luckyBuiltinTierNames 는 설정과 무관하게 항상 단계로 인정하는 이름이다. 설정에서 단계를 지우거나 이름을 바꿔도
// 이미 지급된 당첨의 배지 단계가 사라지지 않게 하려는 것이다.
var luckyBuiltinTierNames = []string{"앙복타임", "앙팡타임", "앙팡팡타임"}

// luckyTierContentSep 는 지급 내역 문구(「<단계> 럭키 포인트」「<단계> 럭키 경험치(댓글)」)에서 단계 이름 뒤에 오는 부분이다.
// 이 구분자까지 포함한 접두사로만 비교한다 — 레거시 「나리야 럭키 포인트」나 설정에 없는 이름은 단계·시각을 비운다(배지만).
const luckyTierContentSep = " 럭키 "

// luckyAtLayout 은 lucky_at 형식이다. po_datetime·xp_datetime 은 KST 벽시계로 저장되므로
// (DSN loc=Asia/Seoul 로 쓰고 읽는다) 벽시계 숫자에 +09:00 만 붙인다.
const luckyAtLayout = "2006-01-02T15:04:05"

// LuckyBadge 는 배지에 싣는 한 글·댓글의 럭키 당첨 내용이다(기록이 없으면 맵에 없다).
type LuckyBadge struct {
	Points int    // lucky_point — g5_point(@lucky) 의 MAX(po_point)
	Exp    int    // lucky_exp — g5_na_xp(@lucky) 의 MAX(xp_point)
	Tier   string // lucky_tier — 지급 문구의 단계 이름. 레거시 문구면 ""(응답에서 생략)
	At     string // lucky_at — 그 지급 행의 시각(YYYY-MM-DDTHH:mm:ss+09:00). Tier 가 없으면 ""
}

// ApplyTo 는 응답 아이템에 배지 필드를 싣는다. lucky_point·lucky_exp 는 항상(없으면 0),
// lucky_tier·lucky_at 은 단계가 확인된 당첨에만 싣는다(레거시·미당첨이면 키 자체를 생략).
func (b LuckyBadge) ApplyTo(item map[string]any) {
	if item == nil {
		return
	}
	item["lucky_point"] = b.Points
	item["lucky_exp"] = b.Exp
	if b.Tier != "" && b.At != "" {
		item["lucky_tier"] = b.Tier
		item["lucky_at"] = b.At
	}
}

// luckyBadgeRow 는 두 원장(g5_point·g5_na_xp)에서 읽은 한 행이다.
type luckyBadgeRow struct {
	RelID    string    `gorm:"column:rel_id"`
	Amount   int       `gorm:"column:amt"`
	Content  string    `gorm:"column:content"`
	Datetime time.Time `gorm:"column:dt"`
}

// LuckyBadgesByWrID 는 한 게시판(slug)의 wr_id 목록에 대해 배지 재료(포인트·경험치·단계·시각)를
// 목록 크기와 무관하게 정확히 2쿼리(g5_point 1 + g5_na_xp 1)로 조회한다. 빈 목록이면 0쿼리다.
// 당첨 기록이 없는 wr_id 는 맵에 없다(호출부에서 0 으로 폴백).
//
// 포인트 소스는 g5_point (po_rel_action='@lucky') 다 — 2026-03-06 이전 레거시 당첨까지 포함하려면 이 원장을
// 직접 봐야 한다. po_rel_table=게시판 슬러그, po_rel_id=wr_id(문자열). (po_rel_table, po_rel_id, po_rel_action)
// 인덱스(idx_lucky_display)를 탄다. 경험치 소스는 g5_na_xp (xp_rel_action='@lucky') 이고
// (xp_rel_table, xp_rel_id) 인덱스(ix_g5_na_xp_2)를 탄다. 글·댓글은 같은 g5_write_{slug} 에서 wr_id 가
// 유일하므로 이 조회 하나로 둘 다 커버한다.
//
// ⭐ 금액은 MAX — 레거시 데이터에 같은 wr_id 로 중복 당첨 행이 있을 수 있어 최댓값으로 방어한다.
// 단계·시각은 단계 문구가 있는 행 중 가장 이른 행에서 가져온다(포인트·경험치가 같은 지급이면 같은 값).
// 시각은 이미 지급된 행의 시각뿐이다 — 단계가 열리는 시각과는 무관하다.
//
// tierNames 는 설정에 있는 단계 이름(무작위 window·고정 시간대)이다. 기본 3개(luckyBuiltinTierNames)는 항상 인정하므로
// nil 이어도 된다. 이름 비교는 접두사 문자열 비교라 이름에 정규식 메타문자가 있어도 안전하고, 쿼리 수도 그대로다.
//
// 부분 실패: 포인트 조회가 실패하면 빈 맵과 에러, 경험치 조회만 실패하면 포인트만 채운 맵과 에러를 돌려준다
// (경험치 조회 실패가 기존 포인트 배지까지 지우지 않게). 호출부는 에러를 로그로 남기고 맵은 그대로 쓴다.
func LuckyBadgesByWrID(db *gorm.DB, slug string, wrIDs []int, tierNames []string) (map[int]LuckyBadge, error) {
	out := make(map[int]LuckyBadge)
	if db == nil || slug == "" {
		return out, nil
	}
	ids := luckyRelIDs(wrIDs)
	if len(ids) == 0 {
		return out, nil
	}
	earliest := make(map[int]time.Time) // 단계가 있는 행 중 가장 이른 시각

	var pointRows []luckyBadgeRow
	if err := db.Table("g5_point").
		Select("po_rel_id AS rel_id, po_point AS amt, po_content AS content, po_datetime AS dt").
		Where("po_rel_action = ? AND po_rel_table = ? AND po_rel_id IN ?", luckyRelAction, slug, ids).
		Scan(&pointRows).Error; err != nil {
		return out, err
	}
	for _, r := range pointRows {
		mergeLuckyBadgeRow(out, earliest, r, false, tierNames)
	}

	var expRows []luckyBadgeRow
	if err := db.Table("g5_na_xp").
		Select("xp_rel_id AS rel_id, xp_point AS amt, xp_content AS content, xp_datetime AS dt").
		Where("xp_rel_table = ? AND xp_rel_id IN ? AND xp_rel_action = ?", slug, ids, luckyRelAction).
		Scan(&expRows).Error; err != nil {
		return out, err
	}
	for _, r := range expRows {
		mergeLuckyBadgeRow(out, earliest, r, true, tierNames)
	}
	return out, nil
}

// mergeLuckyBadgeRow 는 한 행을 배지 맵에 합친다. 금액은 MAX, 단계·시각은 단계 문구가 있는 가장 이른 행.
func mergeLuckyBadgeRow(out map[int]LuckyBadge, earliest map[int]time.Time, r luckyBadgeRow, isExp bool, tierNames []string) {
	wrID, err := strconv.Atoi(r.RelID)
	if err != nil {
		return
	}
	b := out[wrID]
	if isExp {
		if r.Amount > b.Exp {
			b.Exp = r.Amount
		}
	} else if r.Amount > b.Points {
		b.Points = r.Amount
	}
	if tier, ok := LuckyTierFromContent(r.Content, tierNames); ok && !r.Datetime.IsZero() {
		if prev, seen := earliest[wrID]; !seen || r.Datetime.Before(prev) {
			earliest[wrID] = r.Datetime
			b.Tier = tier
			b.At = FormatLuckyAt(r.Datetime)
		}
	}
	out[wrID] = b
}

// LuckyTierFromContent 는 지급 내역 문구에서 단계 이름을 뽑는다. 「<이름> 럭키 」로 시작해야 하고, 이름은
// 기본 3개(앙복타임·앙팡타임·앙팡팡타임) 또는 tierNames(설정된 이름) 중 하나여야 한다. 아니면 ok=false.
// 예) 「앙팡타임 럭키 포인트(댓글)」→ 앙팡타임, 「나리야 럭키 포인트」→ 없음.
// 여러 이름이 맞으면 가장 긴 이름을 쓴다(구분자까지 비교하므로 실제로는 하나만 맞는다).
func LuckyTierFromContent(content string, tierNames []string) (string, bool) {
	best := ""
	try := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || len(name) <= len(best) {
			return
		}
		if strings.HasPrefix(content, name+luckyTierContentSep) {
			best = name
		}
	}
	for _, n := range luckyBuiltinTierNames {
		try(n)
	}
	for _, n := range tierNames {
		try(n)
	}
	return best, best != ""
}

// FormatLuckyAt 은 KST 벽시계로 저장된 시각을 「YYYY-MM-DDTHH:mm:ss+09:00」로 만든다.
// 드라이버가 붙인 시간대 라벨과 무관하게 벽시계 숫자를 그대로 쓴다(저장값 자체가 KST).
func FormatLuckyAt(t time.Time) string {
	return t.Format(luckyAtLayout) + "+09:00"
}

// luckyRelIDs 는 wr_id 목록을 rel_id 비교용 문자열로 바꾼다(0 이하·중복 제거).
// po_rel_id·xp_rel_id 는 VARCHAR 라 문자열로 비교해야 인덱스를 온전히 탄다.
func luckyRelIDs(wrIDs []int) []string {
	seen := make(map[int]struct{}, len(wrIDs))
	ids := make([]string, 0, len(wrIDs))
	for _, id := range wrIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, strconv.Itoa(id))
	}
	return ids
}
