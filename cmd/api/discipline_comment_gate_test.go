package main

import "testing"

// ⛔ 이 파일이 지키는 계약:
//
//	이용제한 근거 댓글은 **비로그인에게 본문이 나가지 않는다.**
//
// #693(신고잠금 게이트)은 `is_restricted` 만 익명 마스킹하고 `is_discipline_related` 는
// 손대지 않았다. 그 결과 free 기준 **2,322건**이 익명에게 원문으로 나갔다 —
// 잠긴 댓글 628건보다 크다.
//
// ⭐ 왜 못 봤나: 근거 **글** 941건은 be 가 이미 막고 있었다. 그래서 「이용제한 쪽은 되고 있다」로
// 뭉뚱그려 검증 표면에서 통째로 뺐다. **글은 되고 댓글은 안 되고 있었다.**
// 검증 표면은 「내가 고친 것」이 아니라 **「지시가 가리키는 것 전체」** 로 잡아야 한다.
//
// ⚠️ 사람이 보는 페이지의 댓글은 web 프록시가 막는다(web #2166). 이 코드가 닫는 것은
// Referer 게이트가 없어 **봇이 직접 칠 수 있는** be 댓글 API 경로다.

func cmt(id int, content string, flags map[string]any) map[string]any {
	m := map[string]any{"id": id, "content": content, "author": "홍길동"}
	for k, v := range flags {
		m[k] = v
	}
	return m
}

func contentOf(m map[string]any) string {
	s, _ := m["content"].(string)
	return s
}

func TestGateDisciplinedComments_AnonymousMasked(t *testing.T) {
	items := []map[string]any{
		cmt(1, "근거 댓글 원문", map[string]any{"is_discipline_related": true}),
		cmt(2, "평범한 댓글", nil),
		cmt(3, "잠긴 댓글 원문", map[string]any{"is_restricted": true}),
	}

	gateDisciplinedComments(items, true, false)

	// ⛔ 여기가 핵심이다. 예전에는 플래그만 붙고 원문이 그대로 나갔다.
	if contentOf(items[0]) != "" {
		t.Errorf("⛔ 근거 댓글 원문이 익명에게 나갔다: %q", contentOf(items[0]))
	}
	if _, ok := items[0]["content"]; !ok {
		t.Error("⛔ content 키를 지웠다 — 소비자가 무가드로 읽는다")
	}
	// ⛔ 무고한 댓글을 삼키면 안 된다. 2026-08-21 실측: 잠긴 글 134건의 댓글 4,805건 중
	//    실제 근거·잠금은 20건뿐이다. 과잉 마스킹은 무고한 1,454명을 가린다.
	if contentOf(items[1]) != "평범한 댓글" {
		t.Errorf("⛔ 평범한 댓글이 가려졌다: %q", contentOf(items[1]))
	}
	// 잠금은 이 함수 소관이 아니다(신고잠금 분기가 이미 처리한다) — 여기서 건드리면 이중 처리다.
	if contentOf(items[2]) != "잠긴 댓글 원문" {
		t.Errorf("잠금 댓글은 이 함수가 건드리지 않는다: %q", contentOf(items[2]))
	}
}

// TestGateDisciplinedComments_MemberKeepsOriginal 은 **진짜 회귀 위험**을 문다.
// 로그인 사용자까지 막으면 근거 댓글 2,322건이 회원 화면에서 사라진다.
// 원문을 받아 프런트 가림막(DisciplinedContent)이 토글하는 것이 원래 설계다.
func TestGateDisciplinedComments_MemberKeepsOriginal(t *testing.T) {
	for _, failed := range []bool{false, true} {
		items := []map[string]any{
			cmt(1, "근거 댓글 원문", map[string]any{"is_discipline_related": true}),
			cmt(2, "평범한 댓글", nil),
		}
		gateDisciplinedComments(items, false, failed)
		if contentOf(items[0]) != "근거 댓글 원문" {
			t.Errorf("⛔ 로그인 사용자에게서 원문을 뺏었다 (lookupFailed=%v): %q", failed, contentOf(items[0]))
		}
		if contentOf(items[1]) != "평범한 댓글" {
			t.Errorf("평범한 댓글이 바뀌었다 (lookupFailed=%v)", failed)
		}
	}
}

// TestGateDisciplinedComments_FailClosedForAnon 은 「모른다」와 「근거 댓글 없음」을 구분한다.
// 게이트가 조회 실패 하나로 무력화되면 안 된다.
func TestGateDisciplinedComments_FailClosedForAnon(t *testing.T) {
	items := []map[string]any{
		cmt(1, "무엇인지 모르는 댓글", nil),
		cmt(2, "이것도 모른다", nil),
	}

	gateDisciplinedComments(items, true, true)

	for _, it := range items {
		if contentOf(it) != "" {
			t.Errorf("⛔ 판정 실패인데 본문이 나갔다: %q", contentOf(it))
		}
		// ⛔ 판정을 못 한 댓글에 「이용제한 근거」를 붙이면 허위 낙인이다.
		if _, ok := it["is_discipline_related"]; ok {
			t.Errorf("⛔ 판정 실패인데 근거 플래그를 세웠다: %v", it)
		}
	}
}

func TestGateDisciplinedComments_EmptyAndNil(t *testing.T) {
	gateDisciplinedComments(nil, true, true) // panic 나면 실패
	items := []map[string]any{nil, cmt(1, "x", nil)}
	gateDisciplinedComments(items, true, false)
	if contentOf(items[1]) != "x" {
		t.Error("nil 항목 때문에 뒤 항목 처리가 어긋났다")
	}
}
