package v2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// 관리자 설정 검증 한도. 운영값이 아니라 「오타가 그대로 저장되는 것」을 막는 바깥 울타리다.
const (
	luckyAdminCapMax        = 10000   // 하루 상한(회원·글·댓글) 최대
	luckyAdminOddsMax       = 1000000 // 확률 분모 최대
	luckyAdminAmountMax     = 1000000 // 포인트·경험치·가중치 최대
	luckyAdminMinCommentMax = 1000    // 댓글 최소 글자 수 최대
	luckyAdminExpireDaysMax = 3650    // 포인트 유효기간(일) 최대
	luckyAdminWindowMinMax  = 600     // 무작위 window 길이(분) 최대
	luckyAdminNameMaxRunes  = 20      // 단계 이름 최대 글자 수
	luckyAdminMaxWindows    = 10      // 무작위 window 개수 최대
	luckyAdminMaxFixed      = 24      // 고정 시간대 개수 최대
	luckyAdminMaxPrizes     = 50      // 상품 표 줄 수 최대
	luckyAdminMaxBoards     = 200     // 게시판 일괄 변경 최대 개수
	luckyAdminBoardIDMaxLen = 20      // v2_board_extended_settings.board_id 길이
	luckyAdminHistoryInView = 10      // GET 에 싣는 최근 이력 수
	luckyAdminRecentInStats = 30      // stats 에 싣는 최근 당첨 수
)

// luckyReservedTierNames 는 단계 이름으로 쓸 수 없는 이름이다. 레거시 내역 문구(「나리야 럭키 포인트」)와
// 같은 접두사가 되면 과거 레거시 당첨에 배지 단계가 잘못 붙는다.
var luckyReservedTierNames = map[string]bool{"나리야": true}

// LuckyFieldError 는 검증 실패 한 건이다. Field 는 JSON 경로(예: fixed_windows[0].start)다.
type LuckyFieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// LuckyValidationError 는 필드별 검증 실패 묶음이다. 핸들러가 400 + 필드별 메시지로 돌려준다.
type LuckyValidationError struct {
	Fields []LuckyFieldError
}

// Error 는 첫 실패를 요약한다(로그용).
func (e *LuckyValidationError) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "lucky: invalid input"
	}
	return fmt.Sprintf("lucky: invalid input: %s: %s", e.Fields[0].Field, e.Fields[0].Message)
}

// luckyErrs 는 검증 중 실패를 모으는 도우미다.
type luckyErrs []LuckyFieldError

func (e *luckyErrs) add(field, msg string) {
	*e = append(*e, LuckyFieldError{Field: field, Message: msg})
}

func (e luckyErrs) err() error {
	if len(e) == 0 {
		return nil
	}
	return &LuckyValidationError{Fields: e}
}

// decodeStrict 는 알 수 없는 키를 거부하며 JSON 하나를 디코드한다. 뒤에 다른 값이 붙어 있어도 거부한다.
// 알 수 없는 키를 조용히 버리면 오타 난 확률 키가 「설정 안 함=꽝」이 되므로 반드시 막는다.
func decodeStrict(body []byte, v any) error {
	if t := bytes.TrimSpace(body); len(t) == 0 || t[0] != '{' {
		// null·배열·빈 본문을 받으면 「전부 기본값」으로 저장될 수 있다 — 객체만 받는다.
		return &LuckyValidationError{Fields: []LuckyFieldError{{Field: "", Message: "JSON 객체여야 합니다"}}}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return decodeErrToField(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return &LuckyValidationError{Fields: []LuckyFieldError{{Field: "", Message: "JSON 값이 하나여야 합니다"}}}
	}
	return nil
}

// decodeErrToField 는 encoding/json 에러를 필드별 메시지로 바꾼다.
func decodeErrToField(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			field = "(root)"
		}
		return &LuckyValidationError{Fields: []LuckyFieldError{{Field: field, Message: "형식이 맞지 않습니다(" + typeErr.Value + " 은 쓸 수 없음)"}}}
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "json: unknown field ") {
		name := strings.Trim(strings.TrimPrefix(msg, "json: unknown field "), `"`)
		return &LuckyValidationError{Fields: []LuckyFieldError{{Field: name, Message: "알 수 없는 키입니다"}}}
	}
	return &LuckyValidationError{Fields: []LuckyFieldError{{Field: "", Message: "JSON 형식이 올바르지 않습니다"}}}
}

// luckyConfigPlain 은 UnmarshalJSON(관대한 파싱) 없이 LuckyConfig 를 엄격하게 디코드하려는 별칭이다.
type luckyConfigPlain v2repo.LuckyConfig

// DecodeLuckyConfig 는 관리자 PUT 본문을 엄격하게 읽고 검증한다. 빠진 키는 기본값이다(전체 교체).
// 저장 경로의 UnmarshalJSON 은 음수·깨진 값을 조용히 기본값으로 바꾸지만, 관리자 입력은 바꾸지 않고 거부한다 —
// 화면에서 넣은 값과 실제 적용 값이 달라지면 안 된다.
func DecodeLuckyConfig(body []byte) (*v2repo.LuckyConfig, error) {
	v := luckyConfigPlain(*v2repo.DefaultLuckyConfig())
	if err := decodeStrict(body, &v); err != nil {
		return nil, err
	}
	cfg := v2repo.LuckyConfig(v)
	if err := ValidateLuckyConfig(&cfg); err != nil {
		return nil, err
	}
	normalizeLuckyConfig(&cfg)
	return &cfg, nil
}

// normalizeLuckyConfig 는 저장 전 모양을 고정한다(이름 앞뒤 공백 제거, nil 목록은 빈 목록).
// 읽는 쪽은 nil 과 빈 목록을 같게 보지만, 이력의 before/after 비교가 모양 차이로 흔들리지 않게 한다.
func normalizeLuckyConfig(c *v2repo.LuckyConfig) {
	if c.Windows == nil {
		c.Windows = []v2repo.LuckyWindow{}
	}
	if c.FixedWindows == nil {
		c.FixedWindows = []v2repo.LuckyFixedWindow{}
	}
	for i := range c.Windows {
		c.Windows[i].Name = strings.TrimSpace(c.Windows[i].Name)
		if c.Windows[i].Prizes == nil {
			c.Windows[i].Prizes = []v2repo.LuckyPrize{}
		}
	}
	for i := range c.FixedWindows {
		c.FixedWindows[i].Name = strings.TrimSpace(c.FixedWindows[i].Name)
		if c.FixedWindows[i].Prizes == nil {
			c.FixedWindows[i].Prizes = []v2repo.LuckyPrize{}
		}
	}
}

// ValidateLuckyConfig 는 범위·형식을 검사해 위반을 필드별로 모은다(첫 위반에서 멈추지 않는다 — 화면이 한 번에 다 보여 주게).
func ValidateLuckyConfig(c *v2repo.LuckyConfig) error {
	var errs luckyErrs
	if c == nil {
		errs.add("", "설정이 비었습니다")
		return errs.err()
	}
	checkRange(&errs, "member_daily_cap", c.MemberDailyCap, 0, luckyAdminCapMax)
	checkRange(&errs, "daily_cap", c.DailyCap, 0, luckyAdminCapMax)
	checkRange(&errs, "daily_cap_post", c.DailyCapPost, 0, luckyAdminCapMax)
	checkRange(&errs, "daily_cap_comment", c.DailyCapComment, 0, luckyAdminCapMax)
	checkRange(&errs, "min_comment_chars", c.MinCommentChars, 0, luckyAdminMinCommentMax)
	checkRange(&errs, "expire_days", c.ExpireDays, 0, luckyAdminExpireDaysMax)
	checkRange(&errs, "window_start_hour", c.WindowStartHour, 0, 23)
	checkRange(&errs, "window_end_hour", c.WindowEndHour, 1, 24)
	rangeOK := c.WindowStartHour >= 0 && c.WindowEndHour <= 24 && c.WindowStartHour < c.WindowEndHour
	if !rangeOK {
		errs.add("window_end_hour", "window_end_hour 는 window_start_hour 보다 커야 합니다")
	}

	names := map[string]string{} // 이름 → 처음 쓴 필드(중복 이름이면 배지 단계가 어느 쪽인지 모른다)
	checkName := func(field, name string) {
		n := strings.TrimSpace(name)
		switch {
		case n == "":
			errs.add(field, "이름이 비었습니다")
			return
		case utf8.RuneCountInString(n) > luckyAdminNameMaxRunes:
			errs.add(field, fmt.Sprintf("이름은 %d자 이하여야 합니다", luckyAdminNameMaxRunes))
		case n == LuckyBaseTierName || luckyReservedTierNames[n]:
			errs.add(field, "쓸 수 없는 이름입니다")
		case strings.Contains(n, " 럭키 "):
			errs.add(field, "이름에 「 럭키 」를 넣을 수 없습니다")
		}
		if prev, dup := names[n]; dup {
			errs.add(field, "이름이 "+prev+" 와 겹칩니다")
			return
		}
		names[n] = field
	}

	if len(c.Windows) > luckyAdminMaxWindows {
		errs.add("windows", fmt.Sprintf("무작위 단계는 %d개 이하여야 합니다", luckyAdminMaxWindows))
	}
	for i, w := range c.Windows {
		p := fmt.Sprintf("windows[%d]", i)
		checkName(p+".name", w.Name)
		checkRange(&errs, p+".minutes", w.Minutes, 1, luckyAdminWindowMinMax)
		if rangeOK && w.Minutes > (c.WindowEndHour-c.WindowStartHour)*60 {
			errs.add(p+".minutes", "길이가 window_start_hour~window_end_hour 범위보다 깁니다")
		}
		checkOdds(&errs, p+".odds", w.Odds)
		checkOdds(&errs, p+".comment_odds", w.CommentOdds)
		checkRange(&errs, p+".points", w.Points, 0, luckyAdminAmountMax)
		checkPrizes(&errs, p+".prizes", w.Prizes)
	}

	if len(c.FixedWindows) > luckyAdminMaxFixed {
		errs.add("fixed_windows", fmt.Sprintf("고정 시간대는 %d개 이하여야 합니다", luckyAdminMaxFixed))
	}
	for i, f := range c.FixedWindows {
		p := fmt.Sprintf("fixed_windows[%d]", i)
		checkName(p+".name", f.Name)
		start, okS := ParseLuckyClock(f.Start)
		if !okS {
			errs.add(p+".start", "KST \"HH:MM\"(00:00~23:59) 형식이어야 합니다")
		}
		end, okE := ParseLuckyClock(f.End)
		if !okE {
			errs.add(p+".end", "KST \"HH:MM\"(00:00~23:59) 형식이어야 합니다")
		}
		if okS && okE && start == end {
			errs.add(p+".end", "시작과 끝이 같으면 열리지 않습니다")
		}
		checkOdds(&errs, p+".odds", f.Odds)
		checkOdds(&errs, p+".comment_odds", f.CommentOdds)
		checkRange(&errs, p+".points", f.Points, 0, luckyAdminAmountMax)
		checkPrizes(&errs, p+".prizes", f.Prizes)
	}
	return errs.err()
}

// ValidateBoardLucky 는 게시판 lucky 값 하나를 검사한다.
func ValidateBoardLucky(prefix string, b v2repo.BoardLucky) error {
	var errs luckyErrs
	checkOdds(&errs, prefix+".odds", b.Odds)
	checkOdds(&errs, prefix+".comment_odds", b.CommentOdds)
	checkRange(&errs, prefix+".points", b.Points, 0, luckyAdminAmountMax)
	checkPrizes(&errs, prefix+".prizes", b.Prizes)
	return errs.err()
}

func checkRange(errs *luckyErrs, field string, v, lo, hi int) {
	if v < lo || v > hi {
		errs.add(field, fmt.Sprintf("%d~%d 사이여야 합니다", lo, hi))
	}
}

// checkOdds 는 확률 분모를 본다. 0 은 「끔」이고, 1 이상이어야 그 종류가 발동한다.
func checkOdds(errs *luckyErrs, field string, v int) {
	if v < 0 || v > luckyAdminOddsMax {
		errs.add(field, fmt.Sprintf("0(끔) 또는 1~%d 이어야 합니다", luckyAdminOddsMax))
	}
}

func checkPrizes(errs *luckyErrs, field string, prizes []v2repo.LuckyPrize) {
	if len(prizes) > luckyAdminMaxPrizes {
		errs.add(field, fmt.Sprintf("상품 표는 %d줄 이하여야 합니다", luckyAdminMaxPrizes))
	}
	for i, p := range prizes {
		row := fmt.Sprintf("%s[%d]", field, i)
		checkRange(errs, row+".weight", p.Weight, 0, luckyAdminAmountMax)
		checkRange(errs, row+".points", p.Points, 0, luckyAdminAmountMax)
		checkRange(errs, row+".exp", p.Exp, 0, luckyAdminAmountMax)
	}
}

// LuckyBoardsRequest 는 게시판 일괄 변경 본문이다.
type LuckyBoardsRequest struct {
	BoardIDs []string           `json:"board_ids"`
	Lucky    *v2repo.BoardLucky `json:"lucky"`
}

// DecodeLuckyBoardsRequest 는 게시판 일괄 변경 본문을 엄격하게 읽고 형식을 검사한다(게시판 존재 확인은 서비스가 한다).
func DecodeLuckyBoardsRequest(body []byte) (*LuckyBoardsRequest, error) {
	var req LuckyBoardsRequest
	if err := decodeStrict(body, &req); err != nil {
		return nil, err
	}
	var errs luckyErrs
	switch {
	case len(req.BoardIDs) == 0:
		errs.add("board_ids", "게시판을 하나 이상 골라야 합니다")
	case len(req.BoardIDs) > luckyAdminMaxBoards:
		errs.add("board_ids", fmt.Sprintf("한 번에 %d개 이하여야 합니다", luckyAdminMaxBoards))
	}
	seen := map[string]bool{}
	for i, id := range req.BoardIDs {
		f := fmt.Sprintf("board_ids[%d]", i)
		switch {
		case id == "" || len(id) > luckyAdminBoardIDMaxLen:
			errs.add(f, "게시판 ID 형식이 올바르지 않습니다")
		case seen[id]:
			errs.add(f, "중복된 게시판입니다")
		}
		seen[id] = true
	}
	if req.Lucky == nil {
		errs.add("lucky", "lucky 값이 필요합니다")
	} else if err := ValidateBoardLucky("lucky", *req.Lucky); err != nil {
		var ve *LuckyValidationError
		if errors.As(err, &ve) {
			errs = append(errs, ve.Fields...)
		}
	}
	if err := errs.err(); err != nil {
		return nil, err
	}
	return &req, nil
}

// LuckyAdminService 는 관리자 화면 API 의 판단(검증·집계)을 맡는다. HTTP 와 DB 는 바깥에 둔다.
// ⛔ 무작위 window 의 그날 시각은 여기서 계산하지도 싣지도 않는다 — 시각 키를 아예 받지 않는다.
type LuckyAdminService struct {
	repo v2repo.LuckyAdminRepository
	now  func() time.Time
}

// NewLuckyAdminService 는 LuckyAdminService 를 만든다.
func NewLuckyAdminService(repo v2repo.LuckyAdminRepository) *LuckyAdminService {
	return &LuckyAdminService{repo: repo, now: time.Now}
}

// LuckyAdminConfigView 는 GET 응답이다.
type LuckyAdminConfigView struct {
	Config  *v2repo.LuckyConfig              `json:"config"`
	Boards  []v2repo.LuckyBoardSetting       `json:"boards"`
	History []v2repo.LuckyConfigHistoryEntry `json:"history"` // 최신순
	// CacheTTLSeconds 는 다른 파드가 새 설정을 보기까지 걸릴 수 있는 최대 시간이다(화면 안내용).
	CacheTTLSeconds int `json:"cache_ttl_seconds"`
	// StoredParseError 는 저장된 원문이 관리자 PUT 과 같은 엄격 검증을 통과하지 못했다는 표식이다.
	// 지급 경로는 그런 값을 조용히 기본값으로 바꾸거나 보정하므로, Config 는 원래 의도와 다를 수 있다.
	// 이 상태로 저장하면 원문이 덮어써지므로 화면이 경고하고 원문을 보여 줄 수 있게 싣는다.
	StoredParseError bool `json:"stored_parse_error"`
	// StoredParseMessage 는 엄격 검증의 필드별 메시지다(StoredParseError 일 때만).
	StoredParseMessage []LuckyFieldError `json:"stored_parse_message,omitempty"`
	// StoredRaw 는 저장된 lucky_config 원문 그대로다(StoredParseError 일 때만).
	StoredRaw json.RawMessage `json:"stored_raw,omitempty"`
}

// checkStoredLuckyConfig 는 저장된 원문을 관리자 PUT 과 같은 엄격 디코더로 다시 읽어 본다.
// 원문이 없으면(설정한 적 없음) 문제없음이다. 지급 경로의 파싱은 바꾸지 않는다 — 여기서는 표식만 만든다.
func checkStoredLuckyConfig(raw json.RawMessage) (bool, []LuckyFieldError) {
	if len(raw) == 0 {
		return false, nil
	}
	if _, err := DecodeLuckyConfig(raw); err != nil {
		var ve *LuckyValidationError
		if errors.As(err, &ve) {
			return true, ve.Fields
		}
		return true, []LuckyFieldError{{Field: "", Message: err.Error()}}
	}
	return false, nil
}

// luckyConfigCacheTTLSeconds 는 v2repo 의 lucky_config 캐시 TTL 과 같은 값이다(안내용).
const luckyConfigCacheTTLSeconds = 30

// GetConfig 는 저장된 설정(기본값 채움)·게시판별 lucky·최근 이력을 돌려준다.
func (s *LuckyAdminService) GetConfig() (*LuckyAdminConfigView, error) {
	cfg, hist, raw, err := s.repo.GetStoredLuckyConfig()
	if err != nil {
		return nil, err
	}
	normalizeLuckyConfig(cfg)
	parseErr, parseMsg := checkStoredLuckyConfig(raw)
	boards, err := s.repo.ListBoardLucky()
	if err != nil {
		return nil, err
	}
	recent := make([]v2repo.LuckyConfigHistoryEntry, 0, luckyAdminHistoryInView)
	for i := len(hist) - 1; i >= 0 && len(recent) < luckyAdminHistoryInView; i-- {
		recent = append(recent, hist[i])
	}
	if boards == nil {
		boards = []v2repo.LuckyBoardSetting{}
	}
	view := &LuckyAdminConfigView{Config: cfg, Boards: boards, History: recent, CacheTTLSeconds: luckyConfigCacheTTLSeconds}
	if parseErr {
		view.StoredParseError, view.StoredParseMessage, view.StoredRaw = true, parseMsg, raw
	}
	return view, nil
}

// PutConfig 는 본문을 검증해 lucky_config 를 통째로 바꾼다. 검증 실패면 *LuckyValidationError 다.
func (s *LuckyAdminService) PutConfig(body []byte, by string) (*v2repo.LuckyConfig, error) {
	cfg, err := DecodeLuckyConfig(body)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveLuckyConfig(cfg, by, s.now()); err != nil {
		return nil, err
	}
	return cfg, nil
}

// PutBoards 는 본문을 검증하고 게시판이 모두 존재할 때만 lucky 키를 바꾼다(하나라도 없으면 아무것도 안 바꾼다).
// 돌려주는 맵은 바뀐 게시판별 settings 전체다.
func (s *LuckyAdminService) PutBoards(body []byte) (*LuckyBoardsRequest, map[string]string, error) {
	req, err := DecodeLuckyBoardsRequest(body)
	if err != nil {
		return nil, nil, err
	}
	exists, err := s.repo.ExistingBoardIDs(req.BoardIDs)
	if err != nil {
		return nil, nil, err
	}
	var errs luckyErrs
	for i, id := range req.BoardIDs {
		if !exists[id] {
			errs.add(fmt.Sprintf("board_ids[%d]", i), "없는 게시판입니다: "+id)
		}
	}
	if err := errs.err(); err != nil {
		return nil, nil, err
	}
	changed, err := s.repo.SetBoardLucky(req.BoardIDs, *req.Lucky, s.now())
	if err != nil {
		return nil, nil, err
	}
	return req, changed, nil
}

// LuckyKindStats 는 종류(글/댓글) 하나의 하루 집계다. Cap 0 은 무제한이고 그때 Remaining 은 null 이다.
type LuckyKindStats struct {
	Wins      int  `json:"wins"`
	Cap       int  `json:"cap"`
	Remaining *int `json:"remaining"`
}

// LuckyTierCount 는 단계별 당첨 수다. 단계를 알 수 없는 문구는 Tier 가 빈 문자열이다.
type LuckyTierCount struct {
	Tier  string `json:"tier"`
	Count int    `json:"count"`
}

// LuckyRecentGrant 는 최근 당첨 한 건이다. ⛔ 회원 ID 는 싣지 않는다 — 화면 캡처가 돌아도 계정이 드러나지 않게 닉네임만.
type LuckyRecentGrant struct {
	BoardID  string `json:"board_id"`
	WrID     string `json:"wr_id"`
	Kind     string `json:"kind"` // post | comment
	Amount   int    `json:"amount"`
	Exp      int    `json:"exp"`
	Tier     string `json:"tier"`
	At       string `json:"at"` // 지급 시각(KST RFC3339). 단계가 열리는 시각이 아니다
	Nickname string `json:"nickname"`
}

// LuckyAdminStats 는 KST 하루 집계다.
type LuckyAdminStats struct {
	Date        string             `json:"date"`
	Post        LuckyKindStats     `json:"post"`
	Comment     LuckyKindStats     `json:"comment"`
	PointsTotal int                `json:"points_total"`
	ExpTotal    int                `json:"exp_total"`
	Members     int                `json:"members"`
	Tiers       []LuckyTierCount   `json:"tiers"`
	Recent      []LuckyRecentGrant `json:"recent"`
}

// Stats 는 date(YYYY-MM-DD, 비면 오늘) 의 KST 00:00~다음날 00:00 원장을 집계한다.
// 상한은 지금 저장된 설정 값이다(과거 날짜면 그날 실제 상한과 다를 수 있다).
func (s *LuckyAdminService) Stats(date string) (*LuckyAdminStats, error) {
	var day time.Time
	if date == "" {
		k := s.now().In(luckyKST)
		day = time.Date(k.Year(), k.Month(), k.Day(), 0, 0, 0, 0, luckyKST)
	} else {
		d, err := time.ParseInLocation("2006-01-02", date, luckyKST)
		if err != nil {
			return nil, &LuckyValidationError{Fields: []LuckyFieldError{{Field: "date", Message: "YYYY-MM-DD 형식이어야 합니다"}}}
		}
		day = d
	}
	start := day.UTC()
	end := day.AddDate(0, 0, 1).UTC()

	cfg, _, _, err := s.repo.GetStoredLuckyConfig()
	if err != nil {
		return nil, err
	}
	grants, err := s.repo.ListLuckyGrantsBetween(start, end)
	if err != nil {
		return nil, err
	}
	tierNames := cfg.TierNames()

	st := &LuckyAdminStats{
		Date:    day.Format("2006-01-02"),
		Post:    LuckyKindStats{Cap: cfg.DailyCapPost},
		Comment: LuckyKindStats{Cap: cfg.DailyCapComment},
		Tiers:   []LuckyTierCount{},
		Recent:  []LuckyRecentGrant{},
	}
	members := map[string]struct{}{}
	tierCount := map[string]int{}
	tierOrder := make([]string, 0, 4)
	sort.SliceStable(grants, func(i, j int) bool { return grants[i].CreatedAt.After(grants[j].CreatedAt) })
	for _, g := range grants {
		kind := "post"
		if g.Kind == v2repo.LuckyKindComment {
			kind = "comment"
			st.Comment.Wins++
		} else {
			st.Post.Wins++
		}
		st.PointsTotal += g.Amount
		st.ExpTotal += g.Exp
		members[g.MbID] = struct{}{}
		content := g.PointContent
		if content == "" {
			content = g.ExpContent
		}
		tier, _ := gnurepo.LuckyTierFromContent(content, tierNames)
		if _, seen := tierCount[tier]; !seen {
			tierOrder = append(tierOrder, tier)
		}
		tierCount[tier]++
		if len(st.Recent) < luckyAdminRecentInStats {
			st.Recent = append(st.Recent, LuckyRecentGrant{
				BoardID:  g.SourceTable,
				WrID:     g.SourceID,
				Kind:     kind,
				Amount:   g.Amount,
				Exp:      g.Exp,
				Tier:     tier,
				At:       g.CreatedAt.In(luckyKST).Format(time.RFC3339),
				Nickname: g.Nick,
			})
		}
	}
	st.Members = len(members)
	for _, t := range tierOrder {
		st.Tiers = append(st.Tiers, LuckyTierCount{Tier: t, Count: tierCount[t]})
	}
	st.Post.Remaining = remainingOf(st.Post)
	st.Comment.Remaining = remainingOf(st.Comment)
	return st, nil
}

// remainingOf 는 상한 대비 남은 수다. 상한 0(무제한)이면 nil, 넘쳤으면 0.
func remainingOf(k LuckyKindStats) *int {
	if k.Cap <= 0 {
		return nil
	}
	r := k.Cap - k.Wins
	if r < 0 {
		r = 0
	}
	return &r
}
