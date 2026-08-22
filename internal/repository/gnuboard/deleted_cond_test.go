package gnuboard

import (
	"strings"
	"testing"
)

// ⛔ 이 파일이 지키는 계약:
//
//	deletedCondFor 는 **모든 wr_deleted_at 참조에 별칭을 붙인다.**
//
// 예전에는 상수였다:
//
//	const deletedCond = "wr_deleted_at IS NOT NULL AND wr_deleted_at <> '0000-00-00...'"
//
// 참조가 **두 번**이라, 호출부가 `"AND c." + deletedCond` 로 붙이면 앞쪽에만 별칭이 붙는다.
// self-join 쿼리(삭제댓글 목록은 부모 글 제목을 얻으려 self-join 한다)에서
// 뒤쪽 미한정 참조가 **MySQL 1052 ambiguous column** 을 낸다.
//
// ⭐ 2026-08-22 bug/13675 실측 재현 (운영 DB):
//
//	미한정   → ERROR 1052: Column 'wr_deleted_at' in where clause is ambiguous
//	c. 한정  → 15,080건
//	COUNT 쿼리는 join 이 없어 **성공** → total 은 채워지고 목록만 0건.
//	화면에 "삭제한 댓글 1,508건"이라 떠 있는데 리스트가 비어 있었다.
//
// ⛔ 문자열 이어붙이기로 별칭을 다는 한 재발한다. 그래서 별칭을 **인자로** 받는다.

func TestDeletedCondFor_QualifiesEveryReference(t *testing.T) {
	got := deletedCondFor("c")

	// ⛔ 핵심: 미한정 참조가 하나라도 남으면 self-join 에서 1052 가 난다.
	//    "c.wr_deleted_at" 를 지운 뒤에도 "wr_deleted_at" 이 남아 있으면 실패다.
	stripped := strings.ReplaceAll(got, "c.wr_deleted_at", "")
	if strings.Contains(stripped, "wr_deleted_at") {
		t.Errorf("⛔ 별칭이 안 붙은 wr_deleted_at 이 남았다 — self-join 에서 ambiguous 가 된다:\n  %s", got)
	}

	if n := strings.Count(got, "c.wr_deleted_at"); n != 2 {
		t.Errorf("참조가 2개여야 한다(IS NOT NULL + <> '0000-...'). got=%d\n  %s", n, got)
	}
}

// TestDeletedCondFor_EmptyAliasStaysUnqualified 는 단일 테이블 쿼리용 경로를 지킨다.
// COUNT 쿼리와 삭제글 목록은 join 이 없어 한정자가 필요 없다 — 붙이면 오히려 틀린다.
func TestDeletedCondFor_EmptyAliasStaysUnqualified(t *testing.T) {
	got := deletedCondFor("")
	if strings.Contains(got, ".wr_deleted_at") {
		t.Errorf("별칭이 없어야 한다. got=%s", got)
	}
	if n := strings.Count(got, "wr_deleted_at"); n != 2 {
		t.Errorf("참조가 2개여야 한다. got=%d — %s", n, got)
	}
}

// TestDeletedCondFor_ShapeIsStable 는 조건의 의미가 바뀌지 않았는지 본다.
// ⛔ '0000-00-00 00:00:00' 비교를 빼면 gnuboard 의 zero-date 행이 "삭제됨"으로 잡힌다.
func TestDeletedCondFor_ShapeIsStable(t *testing.T) {
	got := deletedCondFor("x")
	for _, want := range []string{
		"x.wr_deleted_at IS NOT NULL",
		"x.wr_deleted_at <> '0000-00-00 00:00:00'",
		" AND ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("빠졌다: %q\n  got=%s", want, got)
		}
	}
}
