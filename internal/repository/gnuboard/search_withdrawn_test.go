package gnuboard

import (
	"strings"
	"testing"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newWithdrawnSearchTestRepo 는 g5_member 만 있는 sqlite 저장소를 만든다.
// withMember=false 면 테이블이 없어 판정 조회가 반드시 실패한다(실패 경로 검증용).
func newWithdrawnSearchTestRepo(t *testing.T, withMember bool) *writeRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	if withMember {
		stmts := []string{
			"CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_nick TEXT, mb_leave_date TEXT NOT NULL DEFAULT '')",
			"INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date) VALUES ('active1', '활동회원', '')",
			"INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date) VALUES ('gone1', '떠난회원', '20261001')",
			"INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date) VALUES ('cooling1', '숙려회원', '20261012')",
		}
		for _, s := range stmts {
			if err := db.Exec(s).Error; err != nil {
				t.Fatalf("스키마 준비 실패: %v", err)
			}
		}
	}
	return &writeRepository{db: db}
}

func TestIsAuthorSearchField(t *testing.T) {
	for _, f := range []string{"author", "author_nick", "author_id", "comment_author", "comment_nick", "comment_id"} {
		if !isAuthorSearchField(f) {
			t.Errorf("%q 는 작성자 계열이어야 한다", f)
		}
	}
	for _, f := range []string{"", "title", "content", "title_content", "comment", "mb_id,1"} {
		if isAuthorSearchField(f) {
			t.Errorf("%q 는 작성자 계열이 아니다 — 일반 검색에 필터가 새면 안 된다", f)
		}
	}
}

func TestBlockedAuthorSearch(t *testing.T) {
	r := newWithdrawnSearchTestRepo(t, true)
	cases := []struct {
		name, field, query string
		want               bool
	}{
		{name: "탈퇴 회원 아이디", field: "author", query: "gone1", want: true},
		{name: "탈퇴 회원 닉네임", field: "author_nick", query: "떠난회원", want: true},
		{name: "숙려 중도 탈퇴로 본다", field: "author_id", query: " cooling1 ", want: true},
		{name: "댓글 작성자 검색", field: "comment_author", query: "gone1", want: true},
		{name: "일반 회원", field: "author", query: "active1", want: false},
		{name: "부분 일치는 결과 필터가 맡는다", field: "author", query: "gone", want: false},
		{name: "제목 검색은 그대로", field: "title", query: "gone1", want: false},
		{name: "제목+본문 검색은 그대로", field: "title_content", query: "떠난회원", want: false},
	}
	for _, c := range cases {
		if got := r.blockedAuthorSearch(c.field, c.query); got != c.want {
			t.Errorf("%s: blockedAuthorSearch(%q, %q) = %v, want %v", c.name, c.field, c.query, got, c.want)
		}
	}
}

func TestBlockedAuthorSearch_LookupFailure(t *testing.T) {
	r := newWithdrawnSearchTestRepo(t, false)
	if !r.blockedAuthorSearch("author", "anyone") {
		t.Fatal("판정 실패 시 작성자 검색은 빈 결과여야 한다(비노출 우선)")
	}
	if r.blockedAuthorSearch("title", "anyone") {
		t.Fatal("판정 실패가 일반 검색을 막으면 안 된다")
	}
}

func TestApplyAuthorSearchFilter(t *testing.T) {
	r := newWithdrawnSearchTestRepo(t, true)
	posts := []*gnuboard.G5Write{
		{WrID: 5, MbID: "gone1"},
		{WrID: 4, MbID: "active1"},
		{WrID: 3, MbID: ""}, // 비회원 글
		{WrID: 2, MbID: "cooling1"},
		{WrID: 1, MbID: "active1"},
	}

	kept, total := r.applyAuthorSearchFilter("author", posts, 50)
	var ids []int
	for _, p := range kept {
		ids = append(ids, p.WrID)
	}
	if len(ids) != 3 || ids[0] != 4 || ids[1] != 3 || ids[2] != 1 {
		t.Fatalf("탈퇴 회원 글만 빠지고 순서가 유지돼야 한다: %v", ids)
	}
	if total != 48 {
		t.Errorf("총 건수는 뺀 만큼 줄어야 한다: got %d", total)
	}

	// 일반 검색은 손대지 않는다.
	same, sameTotal := r.applyAuthorSearchFilter("title_content", posts, 50)
	if len(same) != len(posts) || sameTotal != 50 {
		t.Errorf("일반 검색 결과가 바뀌었다: %d건, total %d", len(same), sameTotal)
	}
}

func TestApplyAuthorSearchFilter_LookupFailure(t *testing.T) {
	r := newWithdrawnSearchTestRepo(t, false)
	posts := []*gnuboard.G5Write{{WrID: 1, MbID: "someone"}}
	kept, total := r.applyAuthorSearchFilter("author", posts, 10)
	if len(kept) != 0 || total != 0 {
		t.Fatalf("판정 실패 시 작성자 검색은 빈 결과여야 한다: %d건, total %d", len(kept), total)
	}
	kept, total = r.applyAuthorSearchFilter("title", posts, 10)
	if len(kept) != 1 || total != 10 {
		t.Fatalf("일반 검색은 판정과 무관해야 한다: %d건, total %d", len(kept), total)
	}
}

func TestSearchPosts_WithdrawnTermShortCircuits(t *testing.T) {
	// sphinx 가 없어도 탈퇴 회원 검색어는 오류 없이 빈 결과 — 「결과 없음」과 같은 모양.
	r := newWithdrawnSearchTestRepo(t, true)
	posts, total, err := r.SearchPosts("free", "author", "gone1", 1, 20)
	if err != nil || len(posts) != 0 || total != 0 {
		t.Fatalf("got posts=%d total=%d err=%v", len(posts), total, err)
	}
	// 일반 회원 검색어는 원래 경로로 간다(sphinx 없음 → 기존 오류).
	if _, _, err := r.SearchPosts("free", "author", "active1", 1, 20); err == nil {
		t.Fatal("일반 회원 검색은 기존 경로를 타야 한다")
	}
}

func TestWithdrawnAuthorNotExistsSQL(t *testing.T) {
	got := withdrawnAuthorNotExistsSQL("g5_write_free")
	for _, want := range []string{"NOT EXISTS", "g5_write_free.mb_id", "mb_leave_date <> ''"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 에 %q 가 없다", got, want)
		}
	}
}

func TestSubtractRemoved(t *testing.T) {
	if subtractRemoved(3, 5) != 0 {
		t.Error("0 미만이 되면 안 된다")
	}
	if subtractRemoved(10, 2) != 8 {
		t.Error("뺀 만큼 줄어야 한다")
	}
}
