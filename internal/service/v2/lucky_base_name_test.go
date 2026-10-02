package v2

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// ⛔ 이 파일이 지키는 계약 (평소 단계 이름 base_name):
//
//   - N1 평소 단계(시간대 밖) 지급의 단계 이름은 lucky_config.base_name 이고, 미설정이면 「앙팡」이다.
//     무작위 window·고정 시간대가 걸리면 그 이름이 그대로 이긴다.
//   - N3 base_name 은 1~20자, windows/fixed_windows 이름과 중복 불가, 「 럭키 」 포함 불가, 「나리야」·「앙복타임」 불가.
//     「앙복타임」은 별칭 전용이라 어떤 단계 이름으로도 못 쓴다. 위반이면 저장하지 않는다.
//   - GET 응답 config 에 base_name 이 늘 실린다(기본값 채움). stats 단계별·최근 목록은 「앙복타임」을 base_name 으로 보인다.
//   - F4 무작위 window 시각은 base_name 을 넣어도 어떤 응답에도 없다.
//
// 이름 「앙팡」 외의 값은 모두 임의의 테스트 값이다.

// TestLuckyLive_BaseNameTier — 평소 단계 지급 이름은 base_name, 미설정이면 「앙팡」.
func TestLuckyLive_BaseNameTier(t *testing.T) {
	// 기본 설정: 「앙팡」.
	store := &fakeLuckyStore{cfg: enabledCfg(), boardOdds: 13, boardPts: 9}
	res := newTestLive(store, &fakeRoller{win: true}, nil, testNow).Process("member_a", "free", 1, false, 0)
	if len(store.grants) != 1 || store.grants[0].opt.TierName != "앙팡" || res.Tier != "앙팡" {
		t.Fatalf("기본 평소 단계 이름은 「앙팡」: res=%+v grants=%+v", res, store.grants)
	}

	// 설정한 이름.
	cfg := enabledCfg()
	cfg.BaseName = "임의평소"
	store = &fakeLuckyStore{cfg: cfg, boardOdds: 13, boardCOdds: 17, boardPts: 9}
	cfg.IncludeComments = true
	res = newTestLive(store, &fakeRoller{win: true}, nil, testNow).Process("member_a", "free", 2, true, 50)
	if len(store.grants) != 1 || store.grants[0].opt.TierName != "임의평소" || res.Tier != "임의평소" {
		t.Fatalf("평소 단계 이름은 base_name: res=%+v grants=%+v", res, store.grants)
	}

	// 코드로 만든 설정처럼 BaseName 이 비어 있어도 「앙팡」(이름 없는 문구가 생기면 안 된다).
	store = &fakeLuckyStore{cfg: &v2repo.LuckyConfig{Enabled: true, MemberDailyCap: 1, DailyCapPost: 10}, boardOdds: 13, boardPts: 9}
	res = newTestLive(store, &fakeRoller{win: true}, nil, testNow).Process("member_a", "free", 3, false, 0)
	if res.Tier != LuckyBaseTierName || LuckyBaseTierName != "앙팡" {
		t.Fatalf("BaseName 이 비면 「앙팡」: %+v", res)
	}

	// 고정 시간대에 걸리면 base_name 이 아니라 그 시간대 이름.
	fcfg := fixedCfg(v2repo.LuckyFixedWindow{Name: "고정가", Start: "11:00", End: "13:00", Odds: 6, Points: 9})
	fcfg.BaseName = "임의평소"
	store = &fakeLuckyStore{cfg: fcfg, boardOdds: 13, boardPts: 9}
	res = newTestLive(store, &fakeRoller{win: true}, nil, testNow).Process("member_a", "free", 4, false, 0)
	if res.Tier != "고정가" {
		t.Fatalf("고정 시간대가 평소 단계보다 먼저: %+v", res)
	}
}

// TestDecodeLuckyConfig_BaseNameValidation — N3: base_name 위반은 필드별 400 사유로 모이고 저장되지 않는다.
func TestDecodeLuckyConfig_BaseNameValidation(t *testing.T) {
	cases := []struct {
		body  string
		field string
	}{
		{`{"base_name":""}`, "base_name"},
		{`{"base_name":"   "}`, "base_name"},
		{`{"base_name":"스무자를넘는아주긴평소단계이름입니다스물한자"}`, "base_name"},
		{`{"base_name":"앙복타임"}`, "base_name"},
		{`{"base_name":"나리야"}`, "base_name"},
		{`{"base_name":"임의 럭키 이름"}`, "base_name"},
		{`{"base_name":"겹침가","windows":[{"name":"겹침가","minutes":10,"odds":3,"points":4}]}`, "windows[0].name"},
		{`{"base_name":"겹침나","fixed_windows":[{"name":" 겹침나 ","start":"01:00","end":"02:00","odds":3,"points":4}]}`, "fixed_windows[0].name"},
		{`{"windows":[{"name":"앙팡","minutes":10,"odds":3,"points":4}]}`, "windows[0].name"}, // 기본 base_name 과 겹침
		{`{"windows":[{"name":"앙복타임","minutes":10,"odds":3,"points":4}]}`, "windows[0].name"},
		{`{"base_name":7}`, "base_name"},
	}
	for _, c := range cases {
		_, err := DecodeLuckyConfig([]byte(c.body))
		fields := validationFields(t, err)
		if _, ok := fields[c.field]; !ok {
			t.Errorf("%s: %s 위반이 보고돼야 한다, got %v", c.body, c.field, fields)
		}

		repo := &fakeLuckyAdminRepo{}
		if _, err := NewLuckyAdminService(repo).PutConfig([]byte(c.body), "admin_a"); err == nil || len(repo.saved) != 0 {
			t.Errorf("%s: 위반이면 저장하지 않는다(err=%v saved=%d)", c.body, err, len(repo.saved))
		}
	}

	// 경계: 1자·20자는 통과.
	for _, name := range []string{"가", strings.Repeat("나", luckyAdminNameMaxRunes)} {
		cfg, err := DecodeLuckyConfig([]byte(`{"base_name":"` + name + `"}`))
		if err != nil || cfg.BaseName != name {
			t.Errorf("%q 는 통과해야 한다: cfg=%+v err=%v", name, cfg, err)
		}
	}
}

// TestLuckyAdmin_BaseNameRoundTrip — GET 은 base_name 을 늘 싣고(기본값 채움), PUT 한 값이 그대로 저장·다시 보인다.
func TestLuckyAdmin_BaseNameRoundTrip(t *testing.T) {
	baseNameOf := func(view *LuckyAdminConfigView) string {
		raw, _ := json.Marshal(view)
		var decoded struct {
			Config map[string]json.RawMessage `json:"config"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("응답 파싱 실패: %v", err)
		}
		v, ok := decoded.Config["base_name"]
		if !ok {
			t.Fatalf("config 에 base_name 이 있어야 한다: %s", raw)
		}
		var s string
		_ = json.Unmarshal(v, &s)
		return s
	}

	// 저장된 설정이 없으면 기본값.
	repo := &fakeLuckyAdminRepo{}
	svc := NewLuckyAdminService(repo)
	view, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig 실패: %v", err)
	}
	if got := baseNameOf(view); got != "앙팡" {
		t.Errorf("미설정 GET 은 「앙팡」, got %q", got)
	}

	// 저장소가 BaseName 빈 값을 주더라도 GET 은 기본값을 채운다.
	repo.cfg = &v2repo.LuckyConfig{WindowStartHour: 9, WindowEndHour: 23}
	view, _ = svc.GetConfig()
	if got := baseNameOf(view); got != "앙팡" {
		t.Errorf("빈 BaseName 도 GET 에서 「앙팡」, got %q", got)
	}

	// PUT → 저장 → GET 왕복.
	saved, err := svc.PutConfig([]byte(`{"enabled":true,"base_name":" 임의평소 "}`), "admin_a")
	if err != nil {
		t.Fatalf("정상 PUT 실패: %v", err)
	}
	if saved.BaseName != "임의평소" || len(repo.saved) != 1 || repo.saved[0].BaseName != "임의평소" {
		t.Fatalf("앞뒤 공백을 뗀 값으로 저장: %+v", repo.saved)
	}
	repo.cfg = repo.saved[0]
	view, _ = svc.GetConfig()
	if got := baseNameOf(view); got != "임의평소" {
		t.Errorf("저장한 값이 GET 에 보여야 한다, got %q", got)
	}

	// base_name 키를 빼고 PUT 하면(전체 교체) 기본값.
	saved, err = svc.PutConfig([]byte(`{"enabled":true}`), "admin_a")
	if err != nil || saved.BaseName != "앙팡" {
		t.Errorf("키가 없으면 기본값으로 저장: cfg=%+v err=%v", saved, err)
	}
}

// TestLuckyAdmin_StatsBaseNameAlias — stats 단계별 집계·최근 목록이 「앙복타임」을 현재 base_name 으로 보인다.
func TestLuckyAdmin_StatsBaseNameAlias(t *testing.T) {
	cfg := v2repo.DefaultLuckyConfig()
	cfg.BaseName = "임의평소"
	at := func(h, m int) time.Time { return time.Date(2026, 10, 2, h, m, 0, 0, luckyKST) }
	repo := &fakeLuckyAdminRepo{cfg: cfg, grants: []v2repo.LuckyGrantDetail{
		{MbID: "m1", Nick: "닉가", SourceTable: "free", SourceID: "21", Kind: v2repo.LuckyKindPost, Amount: 3, PointContent: "앙복타임 럭키 포인트", CreatedAt: at(8, 0)},
		{MbID: "m2", Nick: "닉나", SourceTable: "free", SourceID: "22", Kind: v2repo.LuckyKindComment, Exp: 2, ExpContent: "임의평소 럭키 경험치(댓글)", CreatedAt: at(9, 0)},
		{MbID: "m3", Nick: "닉다", SourceTable: "free", SourceID: "23", Kind: v2repo.LuckyKindPost, Amount: 4, PointContent: "앙팡타임 럭키 포인트", CreatedAt: at(10, 0)},
		{MbID: "m4", Nick: "닉라", SourceTable: "free", SourceID: "24", Kind: v2repo.LuckyKindPost, Amount: 5, PointContent: "나리야 럭키 포인트", CreatedAt: at(11, 0)},
	}}
	st, err := NewLuckyAdminService(repo).Stats("2026-10-02")
	if err != nil {
		t.Fatalf("stats 실패: %v", err)
	}
	tiers := map[string]int{}
	for _, tc := range st.Tiers {
		tiers[tc.Tier] = tc.Count
	}
	if tiers["임의평소"] != 2 || tiers["앙팡타임"] != 1 || tiers[""] != 1 || tiers["앙복타임"] != 0 {
		t.Errorf("단계별 집계: %+v", st.Tiers)
	}
	byWr := map[string]string{}
	for _, r := range st.Recent {
		byWr[r.WrID] = r.Tier
	}
	if byWr["21"] != "임의평소" || byWr["22"] != "임의평소" || byWr["23"] != "앙팡타임" || byWr["24"] != "" {
		t.Errorf("최근 목록 단계: %v", byWr)
	}
}

// TestLuckyAdmin_BaseNameKeepsRandomWindowHidden — F4: base_name 을 넣어도 GET·stats 에 무작위 window 시각이 없다.
func TestLuckyAdmin_BaseNameKeepsRandomWindowHidden(t *testing.T) {
	cfg := angpangCfg()
	cfg.BaseName = "임의평소"
	day := kstDay(2026, 10, 2)
	slots := luckyWindowsForDay(DeriveLuckyWindowKey("test-secret-b"), day, cfg)
	if len(slots) == 0 {
		t.Fatal("비교할 무작위 구간이 있어야 한다")
	}
	var forbidden []string
	for _, s := range slots {
		for _, tm := range []time.Time{s.start, s.end} {
			forbidden = append(forbidden, "T"+tm.Format("15:04"), tm.Format(time.RFC3339))
		}
	}
	repo := &fakeLuckyAdminRepo{cfg: cfg, grants: []v2repo.LuckyGrantDetail{
		{MbID: "m", Nick: "n", SourceTable: "free", SourceID: "1", Kind: v2repo.LuckyKindPost, Amount: 1, PointContent: "앙복타임 럭키 포인트", CreatedAt: day.Add(4*time.Hour + 11*time.Minute)},
	}}
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
}
