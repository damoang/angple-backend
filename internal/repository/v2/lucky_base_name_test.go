package v2

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

// testLuckyBaseName 는 테스트용 임의 평소 단계 이름이다(운영 값 아님).
const testLuckyBaseName = "임의평소"

// ⛔ 이 파일이 지키는 계약 (평소 단계 이름 base_name):
//
//   - N1 lucky_config.base_name 이 없거나 비면 「앙팡」이다. 평소 단계 지급 문구는 「<base_name> 럭키 포인트/경험치」,
//     댓글이면 끝에 「(댓글)」.
//   - 배지용 이름 묶음(BadgeTierNames·LuckyTierNamesFrom)에 base_name 이 실린다 — 설정 읽기 실패면 기본값.
//
// 이름 「앙팡」 외의 값은 모두 임의의 테스트 값이다.

// TestLuckyConfig_BaseNameDefault — 키 없음·빈 문자열·공백이면 「앙팡」, 값이 있으면 앞뒤 공백을 뗀 값.
func TestLuckyConfig_BaseNameDefault(t *testing.T) {
	if got := DefaultLuckyConfig().BaseName; got != gnurepo.LuckyDefaultBaseName {
		t.Errorf("기본값은 「앙팡」, got %q", got)
	}
	cases := []struct {
		raw  string
		want string
	}{
		{`{"enabled":true}`, gnurepo.LuckyDefaultBaseName},
		{`{"base_name":""}`, gnurepo.LuckyDefaultBaseName},
		{`{"base_name":"   "}`, gnurepo.LuckyDefaultBaseName},
		{`{"base_name":" 임의평소 "}`, testLuckyBaseName},
	}
	for _, c := range cases {
		var cfg LuckyConfig
		if err := json.Unmarshal([]byte(c.raw), &cfg); err != nil {
			t.Fatalf("파싱 실패 %s: %v", c.raw, err)
		}
		if cfg.BaseName != c.want || cfg.BaseTierName() != c.want {
			t.Errorf("%s: BaseName=%q BaseTierName=%q want %q", c.raw, cfg.BaseName, cfg.BaseTierName(), c.want)
		}
	}
	// 코드로 만든 설정(BaseName 비움)이나 nil 이어도 지급 이름은 기본값이다.
	if got := (&LuckyConfig{}).BaseTierName(); got != gnurepo.LuckyDefaultBaseName {
		t.Errorf("빈 설정: %q", got)
	}
	var nilCfg *LuckyConfig
	if got := nilCfg.BaseTierName(); got != gnurepo.LuckyDefaultBaseName {
		t.Errorf("nil 설정: %q", got)
	}
}

// TestLuckyConfig_BadgeTierNames — 배지 이름 묶음에 base_name 과 설정 단계 이름이 실린다.
func TestLuckyConfig_BadgeTierNames(t *testing.T) {
	cfg := DefaultLuckyConfig()
	cfg.BaseName = testLuckyBaseName
	cfg.Windows = []LuckyWindow{{Name: "무작위가", Minutes: 10, Odds: 3, Points: 4}}
	cfg.FixedWindows = []LuckyFixedWindow{{Name: "고정가", Start: "01:00", End: "02:00", Odds: 5, Points: 6}}
	n := cfg.BadgeTierNames()
	if n.Base != testLuckyBaseName || len(n.Names) != 2 || n.Names[0] != "무작위가" || n.Names[1] != "고정가" {
		t.Errorf("이름 묶음: %+v", n)
	}

	got := LuckyTierNamesFrom(func() (*LuckyConfig, error) { return cfg, nil })
	if got.Base != testLuckyBaseName || len(got.Names) != 2 {
		t.Errorf("LuckyTierNamesFrom: %+v", got)
	}
	// 읽기 실패·nil 이면 제로 값 — 평소 단계는 기본 「앙팡」으로 본다.
	for _, get := range []func() (*LuckyConfig, error){
		nil,
		func() (*LuckyConfig, error) { return nil, errors.New("boom") },
		func() (*LuckyConfig, error) { return nil, nil },
	} {
		z := LuckyTierNamesFrom(get)
		if z.Base != "" || len(z.Names) != 0 || z.BaseName() != gnurepo.LuckyDefaultBaseName {
			t.Errorf("실패면 제로 값(기본 「앙팡」): %+v", z)
		}
	}
}

// TestLuckyContent_BaseName — 새 지급 문구: 「앙팡 럭키 포인트」「앙팡 럭키 경험치(댓글)」. 배지가 같은 문구를 「앙팡」으로 읽는다.
func TestLuckyContent_BaseName(t *testing.T) {
	base := DefaultLuckyConfig().BaseTierName()
	cases := []struct {
		got, want string
	}{
		{luckyPointContentFor(base, false), "앙팡 럭키 포인트"},
		{luckyPointContentFor(base, true), "앙팡 럭키 포인트(댓글)"},
		{luckyExpContentFor(base, false), "앙팡 럭키 경험치"},
		{luckyExpContentFor(base, true), "앙팡 럭키 경험치(댓글)"},
		{luckyPointContentFor(testLuckyBaseName, true), "임의평소 럭키 포인트(댓글)"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("문구: got %q want %q", c.got, c.want)
		}
	}
	names := DefaultLuckyConfig().BadgeTierNames()
	for _, content := range []string{cases[0].got, cases[1].got, cases[2].got, cases[3].got} {
		if tier, ok := gnurepo.LuckyTierFromContent(content, names); !ok || tier != gnurepo.LuckyDefaultBaseName {
			t.Errorf("배지가 새 문구를 「앙팡」으로 읽어야 한다: %q → %q,%v", content, tier, ok)
		}
	}
}

// TestGrant_BaseNameInPointContent — 평소 단계 이름으로 지급하면 g5_point 문구가 「<base_name> 럭키 포인트(댓글)」다.
func TestGrant_BaseNameInPointContent(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	addLuckyMember(t, db, "member_a")
	now := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	if _, err := r.GrantWithOptions("member_a", "free", "31", LuckyKindComment, 6, GrantOptions{TierName: DefaultLuckyConfig().BaseTierName(), Now: now}); err != nil {
		t.Fatalf("지급 실패: %v", err)
	}
	var contents []string
	if err := db.Table("g5_point").Order("po_id").Pluck("po_content", &contents).Error; err != nil {
		t.Fatalf("내역 조회 실패: %v", err)
	}
	if len(contents) != 1 || contents[0] != "앙팡 럭키 포인트(댓글)" {
		t.Errorf("새 지급 문구: %v", contents)
	}
}
