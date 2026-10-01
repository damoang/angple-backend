package v2

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약 (앙팡 관리자 API 의 판단부):
//
//   - A2 알 수 없는 키(최상위·중첩)는 거부한다. 범위·형식 위반은 필드별 메시지로 모두 돌려준다. 위반이면 저장하지 않는다.
//   - A3 게시판 일괄 변경은 없는 게시판이 하나라도 있으면 아무것도 바꾸지 않는다.
//   - A4 stats 는 KST 00:00~다음날 00:00 을 원장 조회 범위로 넘기고, 응답에 회원 ID 가 없다(닉네임만).
//   - F4 GET·stats 응답 어디에도 무작위 window 의 그날 시각이 없다.
//
// 값은 모두 임의의 테스트 값이다.

type fakeLuckyAdminRepo struct {
	cfg        *v2repo.LuckyConfig
	hist       []v2repo.LuckyConfigHistoryEntry
	boards     []v2repo.LuckyBoardSetting
	existing   map[string]bool
	grants     []v2repo.LuckyGrantDetail
	saved      []*v2repo.LuckyConfig
	savedBy    []string
	setIDs     [][]string
	setLucky   []v2repo.BoardLucky
	rangeStart time.Time
	rangeEnd   time.Time
}

func (f *fakeLuckyAdminRepo) GetStoredLuckyConfig() (*v2repo.LuckyConfig, []v2repo.LuckyConfigHistoryEntry, error) {
	if f.cfg == nil {
		return v2repo.DefaultLuckyConfig(), f.hist, nil
	}
	cp := *f.cfg
	return &cp, f.hist, nil
}

func (f *fakeLuckyAdminRepo) SaveLuckyConfig(cfg *v2repo.LuckyConfig, by string, _ time.Time) error {
	f.saved = append(f.saved, cfg)
	f.savedBy = append(f.savedBy, by)
	return nil
}

func (f *fakeLuckyAdminRepo) ListBoardLucky() ([]v2repo.LuckyBoardSetting, error) {
	return f.boards, nil
}

func (f *fakeLuckyAdminRepo) ExistingBoardIDs(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if f.existing[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (f *fakeLuckyAdminRepo) SetBoardLucky(ids []string, lucky v2repo.BoardLucky, _ time.Time) (map[string]string, error) {
	f.setIDs = append(f.setIDs, ids)
	f.setLucky = append(f.setLucky, lucky)
	return map[string]string{}, nil
}

func (f *fakeLuckyAdminRepo) ListLuckyGrantsBetween(start, end time.Time) ([]v2repo.LuckyGrantDetail, error) {
	f.rangeStart, f.rangeEnd = start, end
	return f.grants, nil
}

func validationFields(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *LuckyValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("검증 에러여야 한다, got %v", err)
	}
	out := map[string]string{}
	for _, f := range ve.Fields {
		out[f.Field] = f.Message
	}
	return out
}

// TestDecodeLuckyConfig_UnknownKeys — A2: 오타 키는 최상위든 중첩이든 거부한다.
func TestDecodeLuckyConfig_UnknownKeys(t *testing.T) {
	for _, body := range []string{
		`{"enabled":true,"odd":3}`,
		`{"windows":[{"name":"가","minutes":10,"odds":3,"point":5}]}`,
		`{"fixed_windows":[{"name":"가","start":"01:00","end":"02:00","odds":3,"points":5,"prizes":[{"weight":1,"pts":3}]}]}`,
	} {
		_, err := DecodeLuckyConfig([]byte(body))
		fields := validationFields(t, err)
		found := false
		for _, msg := range fields {
			if strings.Contains(msg, "알 수 없는 키") {
				found = true
			}
		}
		if !found {
			t.Errorf("알 수 없는 키 메시지가 있어야 한다: %s → %v", body, fields)
		}
	}
}

// TestDecodeLuckyConfig_RejectsNonObject — null·배열·빈 본문·값 두 개는 거부(전부 기본값으로 저장되는 사고 방지).
func TestDecodeLuckyConfig_RejectsNonObject(t *testing.T) {
	for _, body := range []string{``, `null`, `[]`, `{"enabled":true}{"enabled":false}`, `{"enabled":"yes"}`} {
		if _, err := DecodeLuckyConfig([]byte(body)); err == nil {
			t.Errorf("거부돼야 한다: %q", body)
		}
	}
}

// TestDecodeLuckyConfig_FieldErrors — A2: 범위·형식 위반을 필드별로 한 번에 돌려준다.
func TestDecodeLuckyConfig_FieldErrors(t *testing.T) {
	body := `{
		"member_daily_cap": -1,
		"daily_cap_post": 10001,
		"window_start_hour": 20, "window_end_hour": 10,
		"windows": [
			{"name": "", "minutes": 0, "odds": -2, "points": 1},
			{"name": "스무자를넘는아주긴단계이름입니다스물한자요", "minutes": 601, "odds": 1, "points": 1}
		],
		"fixed_windows": [
			{"name": "고정가", "start": "7:00", "end": "24:00", "odds": 1, "points": 1},
			{"name": "고정나", "start": "03:00", "end": "03:00", "odds": 1, "points": 1,
			 "prizes": [{"weight": -1, "points": 1, "exp": -1}]},
			{"name": "고정가", "start": "01:00", "end": "02:00", "odds": 1, "points": 1},
			{"name": "나리야", "start": "01:00", "end": "02:00", "odds": 1, "points": 1},
			{"name": "앙복타임", "start": "01:00", "end": "02:00", "odds": 1, "points": 1}
		]
	}`
	_, err := DecodeLuckyConfig([]byte(body))
	fields := validationFields(t, err)
	for _, want := range []string{
		"member_daily_cap", "daily_cap_post", "window_end_hour",
		"windows[0].name", "windows[0].minutes", "windows[0].odds",
		"windows[1].name", "windows[1].minutes",
		"fixed_windows[0].start", "fixed_windows[0].end",
		"fixed_windows[1].end", "fixed_windows[1].prizes[0].weight", "fixed_windows[1].prizes[0].exp",
		"fixed_windows[2].name", "fixed_windows[3].name", "fixed_windows[4].name",
	} {
		if _, ok := fields[want]; !ok {
			t.Errorf("%s 위반이 보고돼야 한다: %v", want, fields)
		}
	}
}

// TestDecodeLuckyConfig_ValidKeepsValues — 정상 입력은 값 그대로(조용히 기본값으로 바꾸지 않고) 돌려준다. 빠진 키는 기본값.
func TestDecodeLuckyConfig_ValidKeepsValues(t *testing.T) {
	body := `{"enabled":true,"daily_cap_post":0,"windows":[{"name":"무작위가","minutes":30,"odds":0,"comment_odds":0,"points":0,
		"prizes":[{"weight":2,"points":3,"exp":0}]}],
		"fixed_windows":[{"name":" 고정가 ","start":"22:30","end":"04:15","odds":7,"comment_odds":0,"points":4}]}`
	cfg, err := DecodeLuckyConfig([]byte(body))
	if err != nil {
		t.Fatalf("정상 입력이 거부됐다: %v", err)
	}
	if !cfg.Enabled || cfg.DailyCapPost != 0 {
		t.Errorf("0 은 무제한 그대로: %+v", cfg)
	}
	if cfg.MemberDailyCap != v2repo.DefaultLuckyConfig().MemberDailyCap {
		t.Errorf("빠진 키는 기본값: %d", cfg.MemberDailyCap)
	}
	if len(cfg.FixedWindows) != 1 || cfg.FixedWindows[0].Name != "고정가" || cfg.FixedWindows[0].Start != "22:30" {
		t.Errorf("고정 시간대가 정리돼 들어가야 한다: %+v", cfg.FixedWindows)
	}
	if cfg.FixedWindows[0].Prizes == nil || cfg.Windows[0].Prizes == nil {
		t.Error("상품 표는 null 이 아니라 빈 표로 정리돼야 한다")
	}
}

// TestLuckyAdmin_PutConfigSavesOnlyValid — A2: 위반이면 저장 호출이 없고, 정상이면 by 와 함께 저장한다.
func TestLuckyAdmin_PutConfigSavesOnlyValid(t *testing.T) {
	repo := &fakeLuckyAdminRepo{}
	svc := NewLuckyAdminService(repo)
	if _, err := svc.PutConfig([]byte(`{"enabled":true,"typo_key":1}`), "admin_a"); err == nil {
		t.Fatal("알 수 없는 키는 거부")
	}
	if len(repo.saved) != 0 {
		t.Fatal("거부된 입력은 저장하면 안 된다")
	}
	if _, err := svc.PutConfig([]byte(`{"enabled":true}`), "admin_a"); err != nil {
		t.Fatalf("정상 입력 저장 실패: %v", err)
	}
	if len(repo.saved) != 1 || !repo.saved[0].Enabled || repo.savedBy[0] != "admin_a" {
		t.Fatalf("저장 값·작성자: %+v %v", repo.saved, repo.savedBy)
	}
}

// TestLuckyAdmin_PutBoardsValidation — A3 판단부: 형식·존재 검증. 없는 게시판이 섞이면 아무것도 안 바꾼다.
func TestLuckyAdmin_PutBoardsValidation(t *testing.T) {
	repo := &fakeLuckyAdminRepo{existing: map[string]bool{"board_a": true, "board_b": true}}
	svc := NewLuckyAdminService(repo)

	bad := []string{
		`{"board_ids":[],"lucky":{"enabled":true,"odds":3,"points":5}}`,
		`{"board_ids":["board_a","board_a"],"lucky":{"enabled":true,"odds":3,"points":5}}`,
		`{"board_ids":["board_a"]}`,
		`{"board_ids":["board_a"],"lucky":{"enabled":true,"odds":-1,"points":5}}`,
		`{"board_ids":["board_a"],"lucky":{"enabled":true,"odds":3,"points":5,"bonus":1}}`,
		`{"board_ids":["board_a"],"lucky":{"enabled":true,"odds":3,"points":5},"extra":1}`,
		`{"board_ids":["board_a","nope"],"lucky":{"enabled":true,"odds":3,"points":5}}`,
	}
	for _, body := range bad {
		if _, _, err := svc.PutBoards([]byte(body)); err == nil {
			t.Errorf("거부돼야 한다: %s", body)
		}
	}
	if len(repo.setIDs) != 0 {
		t.Fatalf("거부된 요청은 쓰기가 없어야 한다: %v", repo.setIDs)
	}

	req, _, err := svc.PutBoards([]byte(`{"board_ids":["board_a","board_b"],"lucky":{"enabled":true,"odds":3,"comment_odds":0,"points":5,"prizes":[{"weight":1,"points":2,"exp":1}]}}`))
	if err != nil {
		t.Fatalf("정상 요청 실패: %v", err)
	}
	if len(repo.setIDs) != 1 || len(repo.setIDs[0]) != 2 || repo.setLucky[0].Odds != 3 || len(req.BoardIDs) != 2 {
		t.Fatalf("두 게시판에 그대로 써야 한다: %v %+v", repo.setIDs, repo.setLucky)
	}
}

// TestLuckyAdmin_StatsKSTDayAndNoMemberID — A4: 조회 범위가 KST 하루이고, 집계가 맞고, 회원 ID 가 응답에 없다.
func TestLuckyAdmin_StatsKSTDayAndNoMemberID(t *testing.T) {
	cfg := v2repo.DefaultLuckyConfig()
	cfg.DailyCapPost, cfg.DailyCapComment = 5, 0
	cfg.FixedWindows = []v2repo.LuckyFixedWindow{{Name: "고정가", Start: "01:00", End: "02:00", Odds: 7, Points: 3}}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 1, h, m, 0, 0, luckyKST) }
	repo := &fakeLuckyAdminRepo{cfg: cfg, grants: []v2repo.LuckyGrantDetail{
		{MbID: "secret_id_1", Nick: "닉가", SourceTable: "free", SourceID: "11", Kind: v2repo.LuckyKindPost, Amount: 7, PointContent: "고정가 럭키 포인트", CreatedAt: at(1, 30)},
		{MbID: "secret_id_2", Nick: "닉나", SourceTable: "free", SourceID: "12", Kind: v2repo.LuckyKindComment, Amount: 0, Exp: 4, ExpContent: "앙복타임 럭키 경험치(댓글)", CreatedAt: at(9, 0)},
		{MbID: "secret_id_1", Nick: "닉가", SourceTable: "qa", SourceID: "13", Kind: v2repo.LuckyKindPost, Amount: 2, PointContent: "나리야 럭키 포인트", CreatedAt: at(23, 59)},
	}}
	svc := NewLuckyAdminService(repo)

	st, err := svc.Stats("2026-10-01")
	if err != nil {
		t.Fatalf("stats 실패: %v", err)
	}
	wantStart := time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	if !repo.rangeStart.Equal(wantStart) || !repo.rangeEnd.Equal(wantEnd) {
		t.Errorf("KST 하루 = UTC 전날 15:00 ~ 당일 15:00, got %s ~ %s", repo.rangeStart, repo.rangeEnd)
	}
	if st.Post.Wins != 2 || st.Comment.Wins != 1 || st.PointsTotal != 9 || st.ExpTotal != 4 || st.Members != 2 {
		t.Errorf("집계: %+v", st)
	}
	if st.Post.Remaining == nil || *st.Post.Remaining != 3 || st.Comment.Remaining != nil {
		t.Errorf("남은 수: post=%v comment=%v", st.Post.Remaining, st.Comment.Remaining)
	}
	tiers := map[string]int{}
	for _, tc := range st.Tiers {
		tiers[tc.Tier] = tc.Count
	}
	if tiers["고정가"] != 1 || tiers["앙복타임"] != 1 || tiers[""] != 1 {
		t.Errorf("단계별: %v", st.Tiers)
	}
	if len(st.Recent) != 3 || st.Recent[0].WrID != "13" || st.Recent[0].At != "2026-10-01T23:59:00+09:00" || st.Recent[2].Nickname != "닉가" {
		t.Errorf("최근 목록(최신순·KST 시각·닉네임): %+v", st.Recent)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "secret_id") || strings.Contains(string(raw), "mb_id") {
		t.Errorf("응답에 회원 ID 가 있으면 안 된다: %s", raw)
	}

	// 날짜 형식이 틀리면 검증 에러.
	if _, err := svc.Stats("2026/10/01"); err == nil {
		t.Error("날짜 형식 오류는 거부")
	}
	// 날짜를 비우면 오늘(KST). UTC 로 아직 전날이어도 KST 날짜를 쓴다.
	svc.now = func() time.Time { return time.Date(2026, 10, 1, 16, 0, 0, 0, time.UTC) } // KST 10/02 01:00
	st, _ = svc.Stats("")
	if st.Date != "2026-10-02" || !repo.rangeStart.Equal(wantEnd) {
		t.Errorf("오늘은 KST 날짜: %s %s", st.Date, repo.rangeStart)
	}
}

// TestLuckyAdmin_NoRandomWindowTimes — F4: GET·stats 응답 어디에도 무작위 window 의 그날 시각이 없다.
func TestLuckyAdmin_NoRandomWindowTimes(t *testing.T) {
	cfg := angpangCfg()
	day := kstDay(2026, 10, 1)
	slots := luckyWindowsForDay(DeriveLuckyWindowKey("test-secret"), day, cfg)
	if len(slots) == 0 {
		t.Fatal("비교할 무작위 구간이 있어야 한다")
	}
	// 응답의 시각은 모두 KST RFC3339 라 「T시:분」 꼴로 찾는다. 무작위 구간은 window_start_hour~window_end_hour
	// (기본 9~23시) 안에만 놓이므로, 그 밖 시각(지급 03:07·이력 05:00)은 우연히 겹칠 수 없다.
	var forbidden []string
	for _, s := range slots {
		for _, tm := range []time.Time{s.start, s.end} {
			forbidden = append(forbidden, "T"+tm.Format("15:04"), tm.Format(time.RFC3339))
		}
	}
	grantAt := day.Add(3*time.Hour + 7*time.Minute)
	repo := &fakeLuckyAdminRepo{cfg: cfg, grants: []v2repo.LuckyGrantDetail{
		{MbID: "m", Nick: "n", SourceTable: "free", SourceID: "1", Kind: v2repo.LuckyKindPost, Amount: 1, PointContent: "앙팡타임 럭키 포인트", CreatedAt: grantAt},
	}, hist: []v2repo.LuckyConfigHistoryEntry{{At: "2026-09-30T05:00:00+09:00", By: "admin_a", Before: json.RawMessage(`null`), After: json.RawMessage(`{"enabled":true}`)}}}
	svc := NewLuckyAdminService(repo)
	svc.now = func() time.Time { return day.Add(12 * time.Hour) }

	view, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig 실패: %v", err)
	}
	st, err := svc.Stats("")
	if err != nil {
		t.Fatalf("Stats 실패: %v", err)
	}
	for name, v := range map[string]any{"config": view, "stats": st} {
		raw, _ := json.Marshal(v)
		for _, f := range forbidden {
			if strings.Contains(string(raw), f) {
				t.Errorf("%s 응답에 무작위 구간 시각 %q 가 있다: %s", name, f, raw)
			}
		}
	}
	// 무작위 window 설정에는 시각 키 자체가 없어야 한다(길이·확률만).
	var decoded struct {
		Config struct {
			Windows []map[string]any `json:"windows"`
		} `json:"config"`
	}
	raw, _ := json.Marshal(view)
	_ = json.Unmarshal(raw, &decoded)
	for i, w := range decoded.Config.Windows {
		for k := range w {
			if k == "start" || k == "end" || k == "start_at" || k == "opens_at" {
				t.Errorf("windows[%d] 에 시각 키 %q 가 있으면 안 된다", i, k)
			}
		}
	}
}
