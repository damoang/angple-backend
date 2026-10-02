package gnuboard

import "testing"

// ⛔ 이 파일이 지키는 계약 (평소 단계 이름 base_name):
//
//   - N2 「앙복타임 럭키 」로 시작하는 지난 당첨은 tier 를 현재 평소 단계 이름(base_name)으로 보고한다(표시 별칭).
//     base_name 이 없으면 기본 「앙팡」이다. 원장 문구는 그대로다.
//   - 판정은 긴 이름 우선 그대로이고 「 럭키 」까지 비교하므로 「앙팡」·「앙팡타임」·「앙팡팡타임」이 섞이지 않는다.
//   - 레거시 「나리야 …」·「…럭키포인트 당첨!」은 여전히 단계가 아니다.
//
// 이름 「앙팡」 외의 값은 모두 임의의 테스트 값이다.

// TestLuckyTierFromContent_LegacyBaseAlias — 「앙복타임」 문구는 현재 base_name 으로, 미설정이면 「앙팡」으로.
func TestLuckyTierFromContent_LegacyBaseAlias(t *testing.T) {
	cases := []struct {
		names   LuckyTierNames
		content string
		want    string
	}{
		{LuckyTierNames{}, "앙복타임 럭키 포인트", "앙팡"},
		{LuckyTierNames{}, "앙복타임 럭키 경험치(댓글)", "앙팡"},
		{LuckyTierNames{Base: "  "}, "앙복타임 럭키 포인트", "앙팡"}, // 공백뿐이면 미설정과 같다
		{LuckyTierNames{Base: "임의평소"}, "앙복타임 럭키 포인트", "임의평소"},
		{LuckyTierNames{Base: "임의평소"}, "앙복타임 럭키 경험치(댓글)", "임의평소"},
		{LuckyTierNames{Base: "임의평소"}, "임의평소 럭키 포인트", "임의평소"},
		{LuckyTierNames{Base: "임의평소"}, "임의평소 럭키 경험치(댓글)", "임의평소"},
	}
	for _, c := range cases {
		got, ok := LuckyTierFromContent(c.content, c.names)
		if !ok || got != c.want {
			t.Errorf("LuckyTierFromContent(%q, %+v) = %q,%v want %q", c.content, c.names, got, ok, c.want)
		}
	}
	// 별칭이 원래 이름을 새지 않는다.
	if got, _ := LuckyTierFromContent("앙복타임 럭키 포인트", LuckyTierNames{Base: "임의평소"}); got == LuckyLegacyBaseName {
		t.Errorf("「앙복타임」 그대로 보고하면 안 된다: %q", got)
	}
}

// TestLuckyTierFromContent_AngpangNamesDistinct — 「앙팡」/「앙팡타임」/「앙팡팡타임」은 구분자까지 비교해 섞이지 않는다.
func TestLuckyTierFromContent_AngpangNamesDistinct(t *testing.T) {
	for _, names := range []LuckyTierNames{{}, {Base: "앙팡"}, {Base: "앙팡", Names: []string{"앙팡타임", "앙팡팡타임"}}} {
		cases := []struct {
			content string
			want    string
			ok      bool
		}{
			{"앙팡 럭키 포인트", "앙팡", true},
			{"앙팡 럭키 경험치(댓글)", "앙팡", true},
			{"앙팡타임 럭키 포인트", "앙팡타임", true},
			{"앙팡타임 럭키 경험치(댓글)", "앙팡타임", true},
			{"앙팡팡타임 럭키 포인트", "앙팡팡타임", true},
			{"앙팡팡타임 럭키 경험치(댓글)", "앙팡팡타임", true},
			{"앙복타임 럭키 포인트", "앙팡", true},
			{"앙팡럭키 포인트", "", false},       // 구분자 없음
			{"앙팡 포인트", "", false},         // 「 럭키 」 없음
			{"나리야 럭키 포인트", "", false},     // 레거시
			{"나리야 럭키포인트 당첨!", "", false},  // 레거시 변형
			{"앙복타임 럭키포인트 당첨!", "", false}, // 구분자가 다르면 별칭도 없다
			{"앙복타임럭키 포인트", "", false},     // 구분자 없음
			{" 앙복타임 럭키 포인트", "", false},   // 앞 공백
			{"앙팡팡팡타임 럭키 포인트", "", false},  // 목록에 없는 이름
		}
		for _, c := range cases {
			got, ok := LuckyTierFromContent(c.content, names)
			if got != c.want || ok != c.ok {
				t.Errorf("names=%+v LuckyTierFromContent(%q) = %q,%v want %q,%v", names, c.content, got, ok, c.want, c.ok)
			}
		}
	}
}

// TestLuckyTierFromContent_BaseRenamedKeepsDefaultName — base_name 을 바꿔도 이미 「앙팡」으로 지급된 당첨은 단계가 남는다.
func TestLuckyTierFromContent_BaseRenamedKeepsDefaultName(t *testing.T) {
	names := LuckyTierNames{Base: "임의평소"}
	if got, ok := LuckyTierFromContent("앙팡 럭키 포인트", names); !ok || got != "앙팡" {
		t.Errorf("기본 이름으로 지급된 문구는 그대로 인정: %q,%v", got, ok)
	}
	if got, ok := LuckyTierFromContent("앙팡타임 럭키 포인트", names); !ok || got != "앙팡타임" {
		t.Errorf("기본 단계 이름은 base 와 무관: %q,%v", got, ok)
	}
}

// TestLuckyTierNames_BaseName — 미설정·공백이면 기본 「앙팡」, 값이 있으면 앞뒤 공백을 뗀 값.
func TestLuckyTierNames_BaseName(t *testing.T) {
	if got := (LuckyTierNames{}).BaseName(); got != LuckyDefaultBaseName || got != "앙팡" {
		t.Errorf("기본값: %q", got)
	}
	if got := (LuckyTierNames{Base: " 임의평소 "}).BaseName(); got != "임의평소" {
		t.Errorf("앞뒤 공백 제거: %q", got)
	}
}

// TestLuckyBadgesByWrID_LegacyBaseAlias — 배지 경로(LuckyBadgesByWrID → ApplyTo)에서 「앙복타임」 행의 lucky_tier 가
// 현재 base_name 이다. 쿼리 수는 그대로 2.
func TestLuckyBadgesByWrID_LegacyBaseAlias(t *testing.T) {
	db, counter := newLuckyBadgeTestDB(t, true)
	if err := db.Exec(`INSERT INTO g5_point (mb_id, po_datetime, po_content, po_point, po_rel_table, po_rel_id, po_rel_action) VALUES
		('m8', '2026-09-30 11:12:13', '앙복타임 럭키 포인트(댓글)', 17, 'free', '108', '@lucky'),
		('m9', '2026-10-02 06:07:08', '앙팡 럭키 포인트', 19, 'free', '109', '@lucky')`).Error; err != nil {
		t.Fatalf("시드 실패: %v", err)
	}
	counter.reset()

	got, err := LuckyBadgesByWrID(db, "free", []int{101, 108, 109}, LuckyTierNames{Base: "임의평소"})
	if err != nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if n := len(counter.snapshot()); n != 2 {
		t.Errorf("2쿼리여야 한다, got %d", n)
	}
	if b := got[108]; b.Tier != "임의평소" || b.At != "2026-09-30T11:12:13+09:00" || b.Points != 17 {
		t.Errorf("「앙복타임」 행은 현재 base_name tier: %+v", b)
	}
	if b := got[109]; b.Tier != "앙팡" || b.Points != 19 {
		t.Errorf("「앙팡」 행은 「앙팡」 tier: %+v", b)
	}
	if b := got[101]; b.Tier != "" || b.At != "" {
		t.Errorf("레거시 「나리야」는 미표시 유지: %+v", b)
	}

	item := map[string]any{}
	got[108].ApplyTo(item)
	if item["lucky_tier"] != "임의평소" {
		t.Errorf("응답 lucky_tier 는 base_name: %v", item)
	}

	// base_name 미설정이면 기본 「앙팡」.
	got, _ = LuckyBadgesByWrID(db, "free", []int{108}, LuckyTierNames{})
	if got[108].Tier != "앙팡" {
		t.Errorf("미설정이면 「앙팡」: %+v", got[108])
	}
}
