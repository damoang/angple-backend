package v2

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// ⛔ 이 파일이 지키는 계약 (럭키 상품 표 — 포인트·경험치 지급):
//
//   - 포인트만 / 경험치만 / 둘 다, 세 경우 모두 원장(g5_da_lucky_grant)은 정확히 1행이다(C16).
//   - 포인트는 g5_point + mb_point, 경험치는 g5_na_xp + as_exp(+as_level 재계산) — 원장과 같은 트랜잭션.
//   - ⛔ mb_level 은 어떤 경우에도 바뀌지 않는다(as_level 만, P3).
//   - 같은 (source_table, source_id, kind) 두 번째 지급은 포인트·경험치 모두 0 이다(C17).
//   - 경험치 기록이 실패하면 원장·포인트도 함께 롤백된다(반쯤 쓰고 멈추지 않는다).

type prizeMemberRow struct {
	MbPoint int `gorm:"column:mb_point"`
	MbLevel int `gorm:"column:mb_level"`
	AsExp   int `gorm:"column:as_exp"`
	AsLevel int `gorm:"column:as_level"`
}

func seedPrizeMember(t *testing.T, db *gorm.DB, mbID string, mbLevel, asExp, asLevel int) {
	t.Helper()
	if err := db.Exec(`INSERT INTO g5_member (mb_id, mb_point, mb_level, as_exp, as_level) VALUES (?, 0, ?, ?, ?)`,
		mbID, mbLevel, asExp, asLevel).Error; err != nil {
		t.Fatalf("회원 생성 실패: %v", err)
	}
}

func prizeMember(t *testing.T, db *gorm.DB, mbID string) prizeMemberRow {
	t.Helper()
	var m prizeMemberRow
	if err := db.Table("g5_member").Select("mb_point, mb_level, as_exp, as_level").
		Where("mb_id = ?", mbID).Scan(&m).Error; err != nil {
		t.Fatalf("회원 조회 실패: %v", err)
	}
	return m
}

type prizeXPRow struct {
	MbID        string `gorm:"column:mb_id"`
	XpPoint     int    `gorm:"column:xp_point"`
	XpContent   string `gorm:"column:xp_content"`
	XpRelTable  string `gorm:"column:xp_rel_table"`
	XpRelID     string `gorm:"column:xp_rel_id"`
	XpRelAction string `gorm:"column:xp_rel_action"`
}

func prizeXPRows(t *testing.T, db *gorm.DB) []prizeXPRow {
	t.Helper()
	var rows []prizeXPRow
	if err := db.Table("g5_na_xp").Order("xp_id").Find(&rows).Error; err != nil {
		t.Fatalf("g5_na_xp 조회 실패: %v", err)
	}
	return rows
}

func prizeLedger(t *testing.T, db *gorm.DB) []LuckyGrant {
	t.Helper()
	var rows []LuckyGrant
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("원장 조회 실패: %v", err)
	}
	return rows
}

// prizeNow 는 KST 10/01 12:00 이다(하루 경계와 무관한 시각).
var prizeNow = time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)

// TestGrantPrize_PointsOnly — C16(포인트만): g5_point 1행·mb_point 증가, g5_na_xp 0행, as_exp·as_level·mb_level 불변.
func TestGrantPrize_PointsOnly(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	seedPrizeMember(t, db, "member_a", 3, 120, 1)

	out, err := r.GrantWithOptions("member_a", "free", "11", LuckyKindPost, 73, GrantOptions{TierName: "단계A", Now: prizeNow})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("지급돼야 한다: out=%s err=%v", out, err)
	}

	ledger := prizeLedger(t, db)
	if len(ledger) != 1 || ledger[0].Amount != 73 || ledger[0].PoID == nil {
		t.Fatalf("원장 1행·amount=포인트·po_id 연결, got %+v", ledger)
	}
	if n := countRows(t, db, "g5_point"); n != 1 {
		t.Errorf("g5_point 1행, got %d", n)
	}
	if rows := prizeXPRows(t, db); len(rows) != 0 {
		t.Errorf("포인트만이면 g5_na_xp 를 쓰지 않는다, got %+v", rows)
	}
	m := prizeMember(t, db, "member_a")
	if m.MbPoint != 73 || m.AsExp != 120 || m.AsLevel != 1 || m.MbLevel != 3 {
		t.Errorf("mb_point=73, as_exp/as_level/mb_level 불변이어야 한다, got %+v", m)
	}
}

// TestGrantPrize_ExpOnly — C16(경험치만): 원장 1행(amount=0, po_id 없음), g5_point 0행,
// g5_na_xp 1행(@lucky·slug·wr_id·「<단계> 럭키 경험치」), as_exp 증가·as_level 재계산, mb_level 불변.
func TestGrantPrize_ExpOnly(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	// levelExp(2)=1000 — 990 에서 37 을 받으면 1027 로 레벨 2 가 된다.
	seedPrizeMember(t, db, "member_a", 3, 990, 1)

	out, err := r.GrantWithOptions("member_a", "free", "12", LuckyKindPost, 0, GrantOptions{TierName: "단계A", Now: prizeNow, Exp: 37})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("지급돼야 한다: out=%s err=%v", out, err)
	}

	ledger := prizeLedger(t, db)
	if len(ledger) != 1 || ledger[0].Amount != 0 || ledger[0].PoID != nil || ledger[0].Kind != LuckyKindPost {
		t.Fatalf("원장 1행·amount=0·po_id 없음·kind 그대로, got %+v", ledger)
	}
	if n := countRows(t, db, "g5_point"); n != 0 {
		t.Errorf("경험치만이면 g5_point 를 쓰지 않는다, got %d", n)
	}
	rows := prizeXPRows(t, db)
	want := prizeXPRow{MbID: "member_a", XpPoint: 37, XpContent: "단계A 럭키 경험치", XpRelTable: "free", XpRelID: "12", XpRelAction: luckyRelAction}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("g5_na_xp 1행 %+v 이어야 한다, got %+v", want, rows)
	}
	m := prizeMember(t, db, "member_a")
	if m.MbPoint != 0 || m.AsExp != 1027 || m.AsLevel != 2 {
		t.Errorf("as_exp=1027·as_level=2·mb_point=0, got %+v", m)
	}
	if m.MbLevel != 3 {
		t.Errorf("⛔ mb_level 은 바뀌면 안 된다(as_level 만), got %d", m.MbLevel)
	}
}

// TestGrantPrize_Both_Comment — C16(둘 다, 댓글): 원장 1행(amount=포인트), g5_point 1행, g5_na_xp 1행(「(댓글)」),
// mb_point·as_exp 증가, mb_level 불변.
func TestGrantPrize_Both_Comment(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	seedPrizeMember(t, db, "member_a", 5, 0, 1)

	out, err := r.GrantWithOptions("member_a", "free", "13", LuckyKindComment, 58, GrantOptions{TierName: "단계B", Now: prizeNow, Exp: 41})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("지급돼야 한다: out=%s err=%v", out, err)
	}

	ledger := prizeLedger(t, db)
	if len(ledger) != 1 || ledger[0].Amount != 58 || ledger[0].PoID == nil || ledger[0].Kind != LuckyKindComment {
		t.Fatalf("원장 1행·amount=포인트·po_id 연결, got %+v", ledger)
	}
	if n := countRows(t, db, "g5_point"); n != 1 {
		t.Errorf("g5_point 1행, got %d", n)
	}
	rows := prizeXPRows(t, db)
	if len(rows) != 1 || rows[0].XpPoint != 41 || rows[0].XpContent != "단계B 럭키 경험치(댓글)" ||
		rows[0].XpRelTable != "free" || rows[0].XpRelID != "13" || rows[0].XpRelAction != luckyRelAction {
		t.Fatalf("g5_na_xp 1행(댓글 문구), got %+v", rows)
	}
	m := prizeMember(t, db, "member_a")
	if m.MbPoint != 58 || m.AsExp != 41 || m.AsLevel != 1 || m.MbLevel != 5 {
		t.Errorf("mb_point=58·as_exp=41·as_level=1·mb_level=5, got %+v", m)
	}
}

// TestGrantPrize_NoDoublePay — C17: 같은 (source_table, source_id, kind) 두 번째 지급은 원장·포인트·경험치 모두 그대로.
func TestGrantPrize_NoDoublePay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int
		exp    int
	}{
		{"포인트만", 19, 0},
		{"경험치만", 0, 37},
		{"둘 다", 19, 37},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, db := newLuckyTestRepo(t)
			seedPrizeMember(t, db, "member_a", 2, 0, 1)
			opt := GrantOptions{TierName: "단계A", Now: prizeNow, Exp: tc.exp}

			if out, err := r.GrantWithOptions("member_a", "free", "21", LuckyKindPost, tc.amount, opt); err != nil || out != GrantOutcomeGranted {
				t.Fatalf("첫 지급: out=%s err=%v", out, err)
			}
			before := prizeMember(t, db, "member_a")

			// 다른 금액·경험치로 다시 와도(재시도·동시 요청) 아무것도 쓰지 않는다.
			opt.Exp = tc.exp * 3
			out, err := r.GrantWithOptions("member_a", "free", "21", LuckyKindPost, tc.amount*3, opt)
			if err != nil || out != GrantOutcomeDuplicate {
				t.Fatalf("두 번째는 duplicate no-op: out=%s err=%v", out, err)
			}
			if n := countRows(t, db, "g5_da_lucky_grant"); n != 1 {
				t.Errorf("원장 1행, got %d", n)
			}
			wantPoint := int64(0)
			if tc.amount > 0 {
				wantPoint = 1
			}
			if n := countRows(t, db, "g5_point"); n != wantPoint {
				t.Errorf("g5_point %d행, got %d", wantPoint, n)
			}
			wantXP := int64(0)
			if tc.exp > 0 {
				wantXP = 1
			}
			if n := countRows(t, db, "g5_na_xp"); n != wantXP {
				t.Errorf("g5_na_xp %d행, got %d", wantXP, n)
			}
			if after := prizeMember(t, db, "member_a"); after != before {
				t.Errorf("두 번째 지급 뒤 회원 값이 바뀌면 안 된다: before=%+v after=%+v", before, after)
			}
		})
	}
}

// TestGrantPrize_ExpFailureRollsBackAll — 경험치 기록이 실패하면(회원 행 없음) 원장·포인트도 남지 않는다.
func TestGrantPrize_ExpFailureRollsBackAll(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	// 회원 행을 만들지 않는다 — addExpTx 의 회원 조회가 실패한다.
	if _, err := r.GrantWithOptions("ghost", "free", "31", LuckyKindPost, 19, GrantOptions{Now: prizeNow, Exp: 37}); err == nil {
		t.Fatal("경험치 기록 실패는 에러여야 한다")
	}
	for _, table := range []string{"g5_da_lucky_grant", "g5_point", "g5_na_xp"} {
		if n := countRows(t, db, table); n != 0 {
			t.Errorf("%s 는 롤백돼야 한다, got %d", table, n)
		}
	}
}

// TestGrantPrize_NothingToGrant — 포인트·경험치 모두 0 이면 아무것도 쓰지 않고 에러다.
func TestGrantPrize_NothingToGrant(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	seedPrizeMember(t, db, "member_a", 2, 0, 1)
	if _, err := r.GrantWithOptions("member_a", "free", "41", LuckyKindPost, 0, GrantOptions{Now: prizeNow}); err == nil {
		t.Fatal("줄 것이 없으면 에러여야 한다")
	}
	if n := countRows(t, db, "g5_da_lucky_grant"); n != 0 {
		t.Errorf("원장을 쓰면 안 된다, got %d", n)
	}
}

// TestGrantPrize_HighLevelFollowsAddExpRule — 경험치는 AddExp 와 같은 규칙을 따른다:
// as_level 80 이상은 로그인 외 적립이 막혀 g5_na_xp·as_exp 가 그대로다(원장·포인트는 정상). mb_level 불변.
func TestGrantPrize_HighLevelFollowsAddExpRule(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	seedPrizeMember(t, db, "member_a", 4, levelExp(80), 80)

	out, err := r.GrantWithOptions("member_a", "free", "51", LuckyKindPost, 19, GrantOptions{Now: prizeNow, Exp: 37})
	if err != nil || out != GrantOutcomeGranted {
		t.Fatalf("지급(포인트)은 돼야 한다: out=%s err=%v", out, err)
	}
	if n := countRows(t, db, "g5_na_xp"); n != 0 {
		t.Errorf("레벨 80 이상은 경험치 적립 없음(AddExp 규칙), got %d", n)
	}
	m := prizeMember(t, db, "member_a")
	if m.AsExp != levelExp(80) || m.AsLevel != 80 || m.MbPoint != 19 || m.MbLevel != 4 {
		t.Errorf("as_exp/as_level/mb_level 불변·포인트만, got %+v", m)
	}
}

// TestGrantPrize_LegacyGrantUnchanged — C19(저장소 쪽): Exp 를 주지 않으면 v4 와 같다(g5_point 만, g5_na_xp 0행).
func TestGrantPrize_LegacyGrantUnchanged(t *testing.T) {
	r, db := newLuckyTestRepo(t)
	seedPrizeMember(t, db, "member_a", 2, 0, 1)
	if ok, err := r.Grant("member_a", "free", "61", LuckyKindPost, 23); err != nil || !ok {
		t.Fatalf("레거시 Grant: ok=%v err=%v", ok, err)
	}
	if n := countRows(t, db, "g5_point"); n != 1 {
		t.Errorf("g5_point 1행, got %d", n)
	}
	if n := countRows(t, db, "g5_na_xp"); n != 0 {
		t.Errorf("g5_na_xp 0행, got %d", n)
	}
	if m := prizeMember(t, db, "member_a"); m.MbPoint != 23 || m.AsExp != 0 || m.MbLevel != 2 {
		t.Errorf("포인트만 지급, got %+v", m)
	}
}

// TestLuckyExpContentFor — 경험치 내역 문구.
func TestLuckyExpContentFor(t *testing.T) {
	cases := []struct {
		tier    string
		comment bool
		want    string
	}{
		{"", false, "나리야 럭키 경험치"},
		{"단계A", false, "단계A 럭키 경험치"},
		{" 단계A ", true, "단계A 럭키 경험치(댓글)"},
	}
	for _, c := range cases {
		if got := luckyExpContentFor(c.tier, c.comment); got != c.want {
			t.Errorf("luckyExpContentFor(%q,%v)=%q want %q", c.tier, c.comment, got, c.want)
		}
	}
}

// TestBoardLuckyOddsOf_Prizes — 레거시 points 가 없어도 주는 줄이 있는 상품 표면 발동한다. 줄 게 없는 표는 꺼짐.
func TestBoardLuckyOddsOf_Prizes(t *testing.T) {
	prizes := []LuckyPrize{{Weight: 3, Exp: 37}}
	got := boardLuckyOddsOf(&BoardLucky{Enabled: true, Odds: 7, Prizes: prizes})
	if got.Odds != 7 || len(got.Prizes) != 1 || !got.Payable() {
		t.Errorf("상품 표만 있어도 발동, got %+v", got)
	}
	empty := boardLuckyOddsOf(&BoardLucky{Enabled: true, Odds: 7, Prizes: []LuckyPrize{{Weight: 0, Exp: 37}, {Weight: 5}}})
	if empty.Odds != 0 || empty.Payable() {
		t.Errorf("줄 게 없는 표·points 없음 = 꺼짐, got %+v", empty)
	}
}
