package v2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// LuckyBaseTierName 은 시간대 단계 밖(평소)의 단계 이름 기본값이다(lucky_config.base_name 이 없을 때).
// 실제 이름은 설정의 base_name(LuckyConfig.BaseTierName)이고, 확률·금액은 게시판 설정을 쓴다.
const LuckyBaseTierName = gnurepo.LuckyDefaultBaseName

// LuckyLegacyBaseTierName 은 예전 평소 단계 이름이다. 지난 당첨 문구의 표시 별칭 전용이라 단계 이름으로 쓸 수 없다.
const LuckyLegacyBaseTierName = gnurepo.LuckyLegacyBaseName

// luckyWindowKeyLabel 은 기존 백엔드 시크릿에서 시간대 전용 키를 뽑을 때 붙이는 라벨이다.
// 라벨을 붙여 파생하므로 시간대 키가 새어도 원래 시크릿(서명 키)은 되짚을 수 없다.
const luckyWindowKeyLabel = "angple/lucky-window/v1"

// luckyWindowMaxAttempts 는 겹침 시 해시로 다음 후보를 뽑는 횟수다. 다 겹치면 결정적 선형 탐색으로 넘어간다.
const luckyWindowMaxAttempts = 32

// luckyKST 는 단계 시각·날짜 계산의 기준 시간대다. 컨테이너 TZ 에 기대지 않고 고정한다.
var luckyKST = time.FixedZone("KST", 9*60*60)

// LuckyLiveStore 는 LuckyLive 가 쓰는 저장소 기능만 뽑은 것이다(v2repo.LuckyRepository 가 만족한다).
// 테스트에서 가짜로 바꿔 끼우려고 인터페이스로 둔다.
type LuckyLiveStore interface {
	GetLuckyConfig() (*v2repo.LuckyConfig, error)
	GetBoardLucky(boardSlug string) v2repo.BoardLuckyOdds
	GrantWithOptions(mbID, sourceTable, sourceID, kind string, amount int, opt v2repo.GrantOptions) (v2repo.GrantOutcome, error)
}

// LuckyLiveResult 는 한 번의 판정 결과다. 테스트·호출부용이며 단계 시각은 일부러 담지 않는다.
type LuckyLiveResult struct {
	Rolled  bool                // 주사위까지 갔는가(스위치·댓글·길이·게시판·단계 확률 조건 통과)
	Won     bool                // 주사위 당첨
	Tier    string              // 적용된 단계 이름
	Odds    int                 // 적용된 확률 분모
	Points  int                 // 적용된 최대 금액(상품 표가 없을 때)
	Amount  int                 // 당첨 포인트(미당첨·경험치만 0)
	Exp     int                 // 당첨 경험치(미당첨·포인트만 0)
	Outcome v2repo.GrantOutcome // 지급 결과(당첨일 때만)
	Err     error               // 지급 실패
}

// LuckyLive 는 라이브 글/댓글 작성 직후 럭키 포인트 판정·지급을 한다.
// main 의 grantLuckyLive 에 있던 분기를 옮겨 와 단위 테스트할 수 있게 했다(주사위·시각·저장소 주입).
type LuckyLive struct {
	store     LuckyLiveStore
	roller    LuckyService
	windowKey []byte
	now       func() time.Time
	// randN 은 상품 표 선택·상품 금액에 쓰는 [0, n) 균등 난수다. 주사위와 같은 crypto/rand 를 쓰고, 테스트에서 바꿔 낀다.
	randN func(n int) int
}

// NewLuckyLive 는 LuckyLive 를 만든다. windowKey 가 비면 시간대 단계를 끈다(평소 단계만).
func NewLuckyLive(store LuckyLiveStore, roller LuckyService, windowKey []byte) *LuckyLive {
	return &LuckyLive{store: store, roller: roller, windowKey: windowKey, now: time.Now, randN: cryptoRandN}
}

// DeriveLuckyWindowKey 는 기존 백엔드 시크릿에서 라벨을 붙여 시간대 전용 키를 만든다.
// 새 환경변수를 두지 않으려는 것이다. 모든 파드가 같은 시크릿을 받으므로 같은 키가 나오고, 재시작에도 바뀌지 않는다.
// 시크릿이 비면 nil — 시간대 단계가 꺼진다.
func DeriveLuckyWindowKey(baseSecret string) []byte {
	if baseSecret == "" {
		return nil
	}
	m := hmac.New(sha256.New, []byte(baseSecret))
	m.Write([]byte(luckyWindowKeyLabel))
	return m.Sum(nil)
}

// Process 는 한 건(글 또는 댓글)을 판정하고 당첨이면 지급한다. best-effort 라 에러는 로그로만 남긴다.
// commentChars 는 댓글 본문의 정리 길이(LuckyCommentChars)다. 글이면 쓰지 않는다(글은 길이 제한 없음).
//
// 순서: 마스터 스위치 → 댓글 여부·길이 → 게시판(그 종류의 확률) → 단계 선택(무작위 window > 고정 시간대 > 평소 단계)
// → 그 단계의 그 종류 확률 → 주사위 → 지급(상한 포함).
// ⛔ 게시판이 꺼져 있으면 시간대와 무관하게 발동하지 않는다 — 운영 기록·광고 게시판에 시간대가 새면 안 된다.
// 댓글도 같다: 게시판 comment_odds 가 없으면 시간대 안이어도 댓글은 발동하지 않는다.
func (l *LuckyLive) Process(mbID, slug string, wrID int, isComment bool, commentChars int) LuckyLiveResult {
	var res LuckyLiveResult
	if mbID == "" {
		return res
	}
	cfg, err := l.store.GetLuckyConfig()
	if err != nil || cfg == nil || !cfg.Enabled { // 전역 마스터 스위치
		return res
	}
	if isComment {
		if !cfg.IncludeComments { // 기본은 글만
			return res
		}
		if cfg.MinCommentChars > 0 && commentChars < cfg.MinCommentChars { // 짧은 댓글 제외
			return res
		}
	}
	board := l.store.GetBoardLucky(slug) // 게시판별 평소 단계 확률·금액(안 켠 곳은 0)
	boardOdds := board.Odds
	if isComment {
		boardOdds = board.CommentOdds
	}
	if boardOdds < 1 || !board.Payable() {
		return res
	}

	now := l.now()
	tier := l.pickTier(cfg, now, board)
	res.Tier, res.Odds, res.Points = tier.name, tier.odds, tier.points
	if isComment {
		res.Odds = tier.commentOdds
	}
	if res.Odds < 1 { // 이 단계에는 댓글 확률이 없다 — 이 단계에서 댓글은 발동하지 않는다
		return res
	}
	res.Rolled = true

	won, amount := l.roller.RollLucky(res.Odds, res.Points)
	if !won {
		return res
	}
	exp, expFallback := 0, 0
	if prize, ok := pickPrize(tier.prizes, l.randN); ok {
		// 상품 표가 있으면 주사위 금액 대신 고른 줄로 준다(포인트 1..Points 균등, 경험치 고정).
		amount, exp = prizeAmount(prize, l.randN), prize.Exp
		if exp < 0 {
			exp = 0
		}
		if exp > 0 {
			expFallback = prizeFallbackPoints(tier.prizes, tier.points)
		}
	}
	if amount <= 0 && exp <= 0 { // 꽝 줄(포인트·경험치 모두 0) 또는 금액 없음
		return res
	}
	res.Won, res.Amount, res.Exp = true, amount, exp

	kind, dailyCap := v2repo.LuckyKindPost, cfg.DailyCapPost
	if isComment {
		kind, dailyCap = v2repo.LuckyKindComment, cfg.DailyCapComment
	}
	outcome, err := l.store.GrantWithOptions(mbID, slug, strconv.Itoa(wrID), kind, amount, v2repo.GrantOptions{
		MemberDailyCap: cfg.MemberDailyCap, // 글+댓글 합산
		DailyCap:       dailyCap,           // 종류별
		TierName:       res.Tier,
		Now:            now,
		ExpireDays:     cfg.ExpireDays,
		Exp:            exp,
		// 경험치를 받을 수 없는 회원이면 저장소가 tx 안에서 「경험치만」을 1..expFallback 포인트로 바꾼다.
		ExpFallbackPoints: expFallback,
		RandN:             l.randN,
	})
	res.Outcome, res.Err = outcome, err
	logLuckyOutcome(mbID, slug, wrID, res.Tier, outcome, err)
	return res
}

// logLuckyOutcome 은 지급 실패·상한 차단을 한 줄로 남긴다.
// ⛔ 단계 이름은 남겨도 단계 시각(구간)은 남기지 않는다 — 로그를 보는 사람에게도 시각이 새면 안 된다.
func logLuckyOutcome(mbID, slug string, wrID int, tier string, outcome v2repo.GrantOutcome, err error) {
	switch {
	case err != nil:
		log.Printf("[lucky] grant failed %s (%s/%d): %v", mbID, slug, wrID, err)
	case outcome == v2repo.GrantOutcomeCappedMember:
		log.Printf("[lucky] capped member %s (%s/%d) tier=%s", mbID, slug, wrID, tier)
	case outcome == v2repo.GrantOutcomeCappedDaily:
		log.Printf("[lucky] capped daily %s (%s/%d) tier=%s", mbID, slug, wrID, tier)
	case outcome == v2repo.GrantOutcomeNothing:
		log.Printf("[lucky] exp-only prize without fallback points %s (%s/%d) tier=%s", mbID, slug, wrID, tier)
	}
}

// luckyTier 는 판정에 쓰는 단계 하나의 이름·확률(글/댓글)·금액이다.
type luckyTier struct {
	name        string
	odds        int // 글 확률 분모
	commentOdds int // 댓글 확률 분모(0 = 이 단계에서 댓글 미발동)
	points      int
	prizes      []v2repo.LuckyPrize // 상품 표(비면 points 로 포인트만)
}

// pickPrize 는 상품 표에서 가중치 비율로 한 줄을 고른다(순수 함수, rng 주입).
// weight<=0 인 줄은 무시한다. 고를 줄이 없으면(빈 표·가중치 합 0) ok=false — 호출부는 레거시 points 를 쓴다.
// rng(n) 은 [0, n) 균등 난수여야 한다.
func pickPrize(prizes []v2repo.LuckyPrize, rng func(n int) int) (v2repo.LuckyPrize, bool) {
	total := 0
	for _, p := range prizes {
		if p.Weight > 0 {
			total += p.Weight
		}
	}
	if total <= 0 || rng == nil {
		return v2repo.LuckyPrize{}, false
	}
	r := rng(total)
	for _, p := range prizes {
		if p.Weight <= 0 {
			continue
		}
		if r < p.Weight {
			return p, true
		}
		r -= p.Weight
	}
	// rng 가 범위를 벗어난 값을 줘도 마지막 유효 줄로 떨어진다(도달하지 않아야 정상).
	for i := len(prizes) - 1; i >= 0; i-- {
		if prizes[i].Weight > 0 {
			return prizes[i], true
		}
	}
	return v2repo.LuckyPrize{}, false
}

// prizeAmount 는 상품 줄의 포인트를 1..Points 균등으로 뽑는다. Points<=0 이면 0(포인트 없음).
func prizeAmount(p v2repo.LuckyPrize, rng func(n int) int) int {
	if p.Points <= 0 {
		return 0
	}
	return rng(p.Points) + 1
}

// prizeFallbackPoints 는 경험치를 받을 수 없는 회원의 「경험치만」 당첨을 대신할 포인트 최대 금액이다.
// 그 단계 상품 표(weight>0 인 줄)의 points 최댓값, 없으면 레거시 points, 그것도 없으면 0(지급 없음).
func prizeFallbackPoints(prizes []v2repo.LuckyPrize, legacyPoints int) int {
	maxPts := 0
	for _, p := range prizes {
		if p.Weight > 0 && p.Points > maxPts {
			maxPts = p.Points
		}
	}
	if maxPts > 0 {
		return maxPts
	}
	if legacyPoints > 0 {
		return legacyPoints
	}
	return 0
}

// pickTier 는 now 에 적용할 단계를 고른다. 우선순위: 무작위 window(앙팡타임 등) > 고정 시간대 > 평소 단계(base_name, 게시판 값).
// 무작위 window 가 가장 드물고 가장 후한 이벤트라 고정 시간대가 그것을 가리면 안 된다.
func (l *LuckyLive) pickTier(cfg *v2repo.LuckyConfig, now time.Time, board v2repo.BoardLuckyOdds) luckyTier {
	k := now.In(luckyKST)
	dayStart := time.Date(k.Year(), k.Month(), k.Day(), 0, 0, 0, 0, luckyKST)
	for _, s := range luckyWindowsForDay(l.windowKey, dayStart, cfg) {
		if !k.Before(s.start) && k.Before(s.end) {
			return luckyTier{name: s.name, odds: s.odds, commentOdds: s.commentOdds, points: s.points, prizes: s.prizes}
		}
	}
	if f, ok := activeFixedWindow(cfg, k); ok {
		commentOdds := f.CommentOdds
		if commentOdds < 1 {
			commentOdds = 0
		}
		return luckyTier{name: strings.TrimSpace(f.Name), odds: f.Odds, commentOdds: commentOdds, points: f.Points, prizes: f.Prizes}
	}
	return luckyTier{name: cfg.BaseTierName(), odds: board.Odds, commentOdds: board.CommentOdds, points: board.Points, prizes: board.Prizes}
}

// activeFixedWindow 는 now(KST) 를 포함하는 첫 고정 시간대를 돌려준다(겹치면 배열 앞쪽).
// 형식이 틀렸거나 이름·확률·지급할 것이 없는 줄은 건너뛴다 — 저장 시 검증하지만, DB 를 직접 고친 값이
// 들어와도 엉뚱한 단계로 지급하지 않게 여기서 한 번 더 거른다.
func activeFixedWindow(cfg *v2repo.LuckyConfig, now time.Time) (v2repo.LuckyFixedWindow, bool) {
	if cfg == nil || len(cfg.FixedWindows) == 0 {
		return v2repo.LuckyFixedWindow{}, false
	}
	k := now.In(luckyKST)
	minute := k.Hour()*60 + k.Minute()
	for _, f := range cfg.FixedWindows {
		if strings.TrimSpace(f.Name) == "" || f.Odds < 1 || (f.Points < 1 && !v2repo.HasPayablePrize(f.Prizes)) {
			continue
		}
		start, ok1 := ParseLuckyClock(f.Start)
		end, ok2 := ParseLuckyClock(f.End)
		if !ok1 || !ok2 {
			continue
		}
		if fixedWindowContains(start, end, minute) {
			return f, true
		}
	}
	return v2repo.LuckyFixedWindow{}, false
}

// fixedWindowContains 는 KST 0시 기준 분 minute 이 [start, end) 안인지 본다. end<start 면 자정을 넘는 구간이다.
// start==end 는 빈 구간이다(24시간짜리를 뜻하려면 00:00~23:59 처럼 명시해야 한다 — 실수로 하루 종일 열리는 일을 막는다).
func fixedWindowContains(start, end, minute int) bool {
	switch {
	case start < end:
		return minute >= start && minute < end
	case start > end:
		return minute >= start || minute < end
	default:
		return false
	}
}

// luckyClockRe 는 "HH:MM"(00:00~23:59, 두 자리 고정) 형식이다.
var luckyClockRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)

// ParseLuckyClock 은 KST "HH:MM" 을 0시 기준 분으로 바꾼다. 형식이 다르면 ok=false.
// "9:00"·"24:00" 처럼 애매한 값을 받지 않는 것은 관리자 화면과 판정이 같은 시각을 보게 하려는 것이다.
func ParseLuckyClock(s string) (int, bool) {
	m := luckyClockRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	mm, _ := strconv.Atoi(m[2])
	return h*60 + mm, true
}

// luckyWindowSlot 은 그날 열리는 단계 구간 하나다. ⛔ 패키지 밖(응답·로그·API)으로 내보내지 않는다.
type luckyWindowSlot struct {
	name        string
	start, end  time.Time // KST
	odds        int
	commentOdds int // 1 미만이면 0(이 단계에서 댓글 미발동)
	points      int
	prizes      []v2repo.LuckyPrize
}

// luckyWindowsForDay 는 dayStart(KST 0시) 날짜의 단계 구간들을 결정적으로 계산한다.
//
// 각 단계의 시작 분은 HMAC-SHA256(key, "lucky-window|YYYY-MM-DD|index") 로 뽑는다 — 같은 키·날짜면
// 어느 파드·언제 계산해도 같다. 구간은 [start_hour, end_hour) 안에 통째로 들어가고, 앞 단계와 겹치면
// "…|index|attempt" 로 다음 후보를 뽑는다. 그래도 다 겹치면 첫 후보부터 1분씩 밀며 찾고, 들어갈 자리가
// 없으면 그 단계는 그날 열지 않는다(겹쳐서 여는 일은 없다).
// key 가 비거나 단계가 없거나 시 범위가 잘못되면 nil(평소 단계만).
func luckyWindowsForDay(key []byte, dayStart time.Time, cfg *v2repo.LuckyConfig) []luckyWindowSlot {
	if len(key) == 0 || cfg == nil || len(cfg.Windows) == 0 {
		return nil
	}
	sh, eh := cfg.WindowStartHour, cfg.WindowEndHour
	if sh < 0 || eh > 24 || sh >= eh {
		return nil
	}
	rangeStart, rangeEnd := sh*60, eh*60
	day := dayStart.In(luckyKST)
	date := day.Format("2006-01-02")

	var placed []luckySpan
	var out []luckyWindowSlot
	for i, w := range cfg.Windows {
		if w.Minutes < 1 || w.Odds < 1 || (w.Points < 1 && !v2repo.HasPayablePrize(w.Prizes)) {
			continue
		}
		start, ok := placeLuckyWindow(key, date, i, w.Minutes, rangeStart, rangeEnd, placed)
		if !ok {
			continue // 자리가 없다 — 겹쳐서 열지 않는다
		}
		placed = append(placed, luckySpan{start, start + w.Minutes})
		s := day.Add(time.Duration(start) * time.Minute)
		commentOdds := w.CommentOdds
		if commentOdds < 1 {
			commentOdds = 0
		}
		out = append(out, luckyWindowSlot{
			name:        w.Name,
			start:       s,
			end:         s.Add(time.Duration(w.Minutes) * time.Minute),
			odds:        w.Odds,
			commentOdds: commentOdds,
			points:      w.Points,
			prizes:      w.Prizes,
		})
	}
	return out
}

// luckySpan 은 KST 0시 기준 분 단위 반열린 구간 [a, b) 다.
type luckySpan struct{ a, b int }

// luckySpansOverlap 은 [a, b) 가 이미 놓인 구간 중 하나와 겹치는지 본다.
func luckySpansOverlap(placed []luckySpan, a, b int) bool {
	for _, p := range placed {
		if a < p.b && p.a < b {
			return true
		}
	}
	return false
}

// placeLuckyWindow 는 index 번 단계(길이 minutes 분)의 시작 분을 [rangeStart, rangeEnd) 안에서 결정적으로 고른다.
// 해시 후보 luckyWindowMaxAttempts 개를 차례로 보고, 다 겹치면 첫 후보부터 1분씩 민다. 자리가 없으면 ok=false.
func placeLuckyWindow(key []byte, date string, index, minutes, rangeStart, rangeEnd int, placed []luckySpan) (start int, ok bool) {
	slots := rangeEnd - rangeStart - minutes + 1 // 가능한 시작 분 개수
	if slots < 1 {
		return 0, false
	}
	first := luckyHashMod(luckyWindowHash(key, date, index, 0), slots)
	for attempt := 0; attempt < luckyWindowMaxAttempts; attempt++ {
		off := first
		if attempt > 0 {
			off = luckyHashMod(luckyWindowHash(key, date, index, attempt), slots)
		}
		if !luckySpansOverlap(placed, rangeStart+off, rangeStart+off+minutes) {
			return rangeStart + off, true
		}
	}
	for k := 1; k < slots; k++ {
		off := (first + k) % slots
		if !luckySpansOverlap(placed, rangeStart+off, rangeStart+off+minutes) {
			return rangeStart + off, true
		}
	}
	return 0, false
}

// luckyHashMod 는 해시값을 [0, slots) 의 시작 분 후보로 줄인다. slots<1 이면 0.
func luckyHashMod(h uint64, slots int) int {
	if slots < 1 {
		return 0
	}
	return int(h % uint64(slots)) // #nosec G115 -- h % uint64(slots) < slots 이므로 int 범위 안
}

// luckyWindowHash 는 단계 시작 후보 하나를 뽑는 결정적 해시다(HMAC 앞 8바이트).
func luckyWindowHash(key []byte, date string, index, attempt int) uint64 {
	msg := "lucky-window|" + date + "|" + strconv.Itoa(index)
	if attempt > 0 {
		msg += "|" + strconv.Itoa(attempt)
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return binary.BigEndian.Uint64(m.Sum(nil)[:8])
}

// 댓글 길이 계산용 패턴. HTML 태그와 이모티콘 숏코드({emo:...}, {이모티콘:...})를 지운다.
var (
	luckyHTMLTagRe  = regexp.MustCompile(`<[^>]*>`)
	luckyEmoticonRe = regexp.MustCompile(`\{(?:emo|이모티콘):[^}]*\}`)
)

// LuckyCommentChars 는 댓글 본문의 「정리 길이」를 돌려준다(min_comment_chars 판정용, 순수 함수).
// 정리 = HTML 태그 제거 → HTML 엔티티 풀기(&nbsp; 등이 글자로 세지지 않게) → 이모티콘 숏코드 제거 → 앞뒤 공백 제거,
// 길이는 rune 수다. 이모티콘·공백만 있는 댓글은 0 이다.
func LuckyCommentChars(content string) int {
	s := luckyHTMLTagRe.ReplaceAllString(content, "")
	s = html.UnescapeString(s)
	s = luckyEmoticonRe.ReplaceAllString(s, "")
	return utf8.RuneCountInString(strings.TrimSpace(s))
}
