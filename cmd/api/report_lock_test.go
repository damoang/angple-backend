package main

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약:
//
//	reportLockedIDs 는 조회 실패를 **빈 집합으로 삼키지 않는다.**
//
// 반환값이 「가려야 할 항목 목록」이라, 실패가 빈 집합으로 나가면 호출부에서
// 「잠긴 것 없음」으로 읽힌다. 그러면 비로그인 게이트가 통째로 무력화된다.
// 예전 코드는 `_ = db...Error` 로 에러를 버렸고, 그래서 게이트를 붙일 수가 없었다.
//
// ⭐ 판별 기준은 `!= nil` 이 아니라 **`err == nil` 인데 값이 비어 있을 수 있는가** 다.
//
// 2026-08-21 실측 — 무엇이 걸려 있나(free 기준, 비밀글도 근거글도 아닌 순증):
//
//	잠긴 댓글 628건 · 잠긴 글 28건. 그리고 댓글 API 에는 Referer 게이트가 없어
//	봇이 직접 호출할 수 있다(글 상세는 403). 실질 노출면은 댓글이다.
//
// ⚠️ 한계 — 호출부의 익명 분기(isAnon 일 때만 본문을 비운다)는 거대한 핸들러 안에 있어
// 여기서 못 돈다. 이 파일은 **집합 계산과 마스킹 함수**까지만 보장한다.
// 익명/로그인 분기는 배포 후 실제 요청으로 검증한다.

func newLockTestDB(t *testing.T, withTable bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	if !withTable {
		// ⭐ 테이블이 없으면 조회가 반드시 실패한다. 여기서는 그게 검증 수단이다.
		return db
	}
	if err := db.Exec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY, wr_is_comment INTEGER, wr_7 TEXT)`).Error; err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}
	rows := []struct {
		id      int
		comment int
		wr7     any
	}{
		{10, 0, "lock"}, // 잠긴 글
		{11, 0, "1"},    // ⭐ free 의 wr_7 은 숫자도 담는다(실측: '1' 이 10,101건). 잠금이 아니다.
		{12, 0, ""},
		{13, 0, nil}, // NULL → COALESCE 로 ""
		{20, 1, "lock"},
		{21, 1, "3"},
		{22, 1, nil},
	}
	for _, r := range rows {
		if err := db.Exec(
			"INSERT INTO g5_write_free (wr_id, wr_is_comment, wr_7) VALUES (?,?,?)",
			r.id, r.comment, r.wr7,
		).Error; err != nil {
			t.Fatalf("행 삽입 실패: %v", err)
		}
	}
	return db
}

func TestReportLockedIDs_PropagatesError(t *testing.T) {
	set, err := reportLockedIDs(newLockTestDB(t, false), "free", []int{10, 11}, false)

	// ⛔ 여기가 핵심이다. err == nil 이면 호출부가 "잠긴 것 없음" 으로 읽고 원문을 내보낸다.
	if err == nil {
		t.Fatalf("error 를 기대했다. set=%v — 이 조합이 게이트를 무력화한다", set)
	}
	if set != nil {
		t.Errorf("실패 시에는 집합을 주면 안 된다. set=%v", set)
	}
}

// TestReportLockedIDs_RejectsBadSlug 는 slug 가 테이블명에 직접 들어가므로
// 형식을 **쿼리를 치기 전에** 강제하는지 본다.
//
// ⛔ "error 가 났다" 로 판정하면 안 된다 — 검증을 지워도 g5_write_{쓰레기} 는 어차피
//
//	"그런 테이블 없음" 으로 에러가 난다. 2026-08-21 대조군에서 실제로 이 테스트가
//	**검증을 제거해도 통과**했다. 그래서 에러의 **종류**를 본다.
func TestReportLockedIDs_RejectsBadSlug(t *testing.T) {
	for _, bad := range []string{"free; DROP TABLE x", "free-1", "../etc", "free`", "free free"} {
		set, err := reportLockedIDs(newLockTestDB(t, true), bad, []int{10}, false)
		if err == nil {
			t.Errorf("잘못된 slug 를 통과시켰다: %q (set=%v)", bad, set)
			continue
		}
		if !strings.Contains(err.Error(), "잘못된 보드 slug") {
			t.Errorf("⛔ %q 가 형식 검증이 아니라 DB 에서 걸렀다 — 검증이 없어도 이렇게 된다: %v", bad, err)
		}
	}
}

// TestReportLockedIDs_AcceptsValidSlug 는 위 테스트가 전부를 막아버리는
// 과잉 검증이 되지 않았는지 본다(대조군 없는 금지 테스트는 무의미하다).
func TestReportLockedIDs_AcceptsValidSlug(t *testing.T) {
	for _, ok := range []string{"free", "free_2", "MaClien", "vote20250603"} {
		if _, err := reportLockedIDs(newLockTestDB(t, false), ok, []int{10}, false); err != nil &&
			strings.Contains(err.Error(), "잘못된 보드 slug") {
			t.Errorf("정상 slug 를 막았다: %q", ok)
		}
	}
}

// TestReportLockedIDs_EmptyInputIsNotAnError 는 「조회할 것 없음」과 「실패」를 구분한다.
func TestReportLockedIDs_EmptyInputIsNotAnError(t *testing.T) {
	db := newLockTestDB(t, false) // 테이블이 없어도 쿼리 자체를 안 쳐야 한다
	for _, tc := range []struct {
		name string
		slug string
		ids  []int
	}{
		{"보드 없음", "", []int{10}},
		{"id 없음", "free", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := reportLockedIDs(db, tc.slug, tc.ids, false)
			if err != nil {
				t.Fatalf("조회할 것이 없으면 error 가 아니어야 한다: %v", err)
			}
			if set == nil {
				t.Error("빈 map 을 줘야 한다 (nil 이 아니라)")
			}
		})
	}
}

// TestReportLockedIDs_OnlyLockValueCounts 는 wr_7 이 다목적 컬럼이라는 실측을 못 박는다.
// free 의 wr_7 에는 '1'(10,101건) '2'(1,820건) 같은 숫자가 들어 있고, 'lock' 은 135건뿐이다.
// 「wr_7 이 비어 있지 않으면 잠금」으로 구현하면 1만 건이 잘못 가려진다.
func TestReportLockedIDs_OnlyLockValueCounts(t *testing.T) {
	set, err := reportLockedIDs(newLockTestDB(t, true), "free", []int{10, 11, 12, 13}, false)
	if err != nil {
		t.Fatalf("정상 조회인데 error: %v", err)
	}
	if _, ok := set[10]; !ok {
		t.Error("wr_7='lock' 인 글이 빠졌다")
	}
	for _, id := range []int{11, 12, 13} {
		if _, ok := set[id]; ok {
			t.Errorf("⛔ 잠금이 아닌 글이 잠금으로 잡혔다: wr_id=%d (wr_7 은 다목적 컬럼이다)", id)
		}
	}
}

// TestReportLockedIDs_SeparatesCommentsFromPosts 는 글/댓글 구분을 본다.
func TestReportLockedIDs_SeparatesCommentsFromPosts(t *testing.T) {
	db := newLockTestDB(t, true)

	posts, err := reportLockedIDs(db, "free", []int{10, 20}, false)
	if err != nil {
		t.Fatalf("글 조회 실패: %v", err)
	}
	if _, ok := posts[20]; ok {
		t.Error("글 조회에 댓글(wr_is_comment=1)이 섞였다")
	}

	comments, err := reportLockedIDs(db, "free", []int{10, 20, 21, 22}, true)
	if err != nil {
		t.Fatalf("댓글 조회 실패: %v", err)
	}
	if _, ok := comments[20]; !ok {
		t.Error("잠긴 댓글이 빠졌다")
	}
	if _, ok := comments[10]; ok {
		t.Error("댓글 조회에 글(wr_is_comment=0)이 섞였다")
	}
	if len(comments) != 1 {
		t.Errorf("잠긴 댓글은 1건이어야 한다. got=%v", comments)
	}
}

// ⛔ maskLockedContent 는 **키를 지우지 않는다.** 소비자가 무가드로 읽기 때문이다
// (삭제 댓글 tombstone 이 같은 원칙을 쓴다 — transform.go:441).
// 그리고 제목은 건드리지 않는다 — 목록이 A형을 가리지 않으므로 상세만 지우면 어긋난다.
func TestMaskLockedContent(t *testing.T) {
	m := map[string]any{
		"id":            7091384,
		"title":         "결국 이가가 탄핵되어야 할 듯 싶네요",
		"content":       "<p>원문 본문</p>",
		"author":        "홍길동",
		"is_restricted": true,
	}
	maskLockedContent(m)

	if _, ok := m["content"]; !ok {
		t.Error("⛔ content 키를 지웠다 — 소비자가 무가드로 읽는다")
	}
	if m["content"] != "" {
		t.Errorf("본문이 남았다: %v", m["content"])
	}
	if m["title"] != "결국 이가가 탄핵되어야 할 듯 싶네요" {
		t.Errorf("⛔ 제목을 건드렸다. got=%v", m["title"])
	}
	if m["author"] != "홍길동" || m["id"] != 7091384 {
		t.Error("본문 외 필드가 바뀌었다")
	}
}

// TestMaskLockedContent_OnlyExistingKeys 는 없던 키를 만들지 않는지 본다.
// 댓글 응답에는 wr_content 가 없다 — 만들어 내면 프런트가 빈 필드를 렌더할 수 있다.
func TestMaskLockedContent_OnlyExistingKeys(t *testing.T) {
	m := map[string]any{"id": 1, "content": "x"}
	maskLockedContent(m)
	if _, ok := m["wr_content"]; ok {
		t.Error("없던 wr_content 키를 만들었다")
	}
}
