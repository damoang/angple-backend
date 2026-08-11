package v1handler

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// 뷰어 등급별 IP 노출 계약:
//
//	비로그인 → ""            (마스킹된 형태조차 내리지 않는다)
//	로그인   → "1.♡.3.4"     (OverrideIPMasked*)
//	관리자   → "1.2.3.4"     (OverrideIPForAdmin*)
//
// 종전엔 transform 이 곧바로 MaskIP 를 넣어 **비로그인도** 4옥텟 중 3옥텟을 받았다.
func newWrite(ip string) *gnuboard.G5Write {
	return &gnuboard.G5Write{
		WrID:       1,
		WrSubject:  "제목",
		WrName:     "글쓴이",
		WrIP:       ip,
		WrDatetime: time.Now(),
	}
}

func TestPostIPHiddenFromAnonymousByDefault(t *testing.T) {
	item := TransformToV1Post(newWrite("1.2.3.4"), false)

	// 키는 반드시 유지된다 — 지우면 소비자가 죽는다(#604).
	got, ok := item["author_ip"]
	if !ok {
		t.Fatal("author_ip 키가 사라졌다 — 값만 중립화해야 한다")
	}
	if got != "" {
		t.Errorf("비로그인 기본값 = %q, 빈 문자열이어야 한다", got)
	}
}

func TestPostIPMaskedForMemberFullForAdmin(t *testing.T) {
	w := newWrite("1.2.3.4")

	member := TransformToV1Post(w, false)
	OverrideIPMaskedSingle(member, w)
	if member["author_ip"] != "1.♡.3.4" {
		t.Errorf("로그인 회원 = %q, 마스킹 IP 여야 한다", member["author_ip"])
	}

	admin := TransformToV1Post(w, false)
	OverrideIPForAdminSingle(admin, w)
	if admin["author_ip"] != "1.2.3.4" {
		t.Errorf("관리자 = %q, 원본 IP 여야 한다", admin["author_ip"])
	}
}

func TestDeletedPostKeepsEmptyIPForEveryone(t *testing.T) {
	deletedAt := time.Now()
	w := newWrite("1.2.3.4")
	w.WrDeletedAt = &deletedAt

	// 삭제글 tombstone 은 어떤 override 도 IP 를 채우지 않는다(#13174).
	item := TransformToV1Post(w, false)
	OverrideIPMaskedSingle(item, w)
	if item["author_ip"] != "" {
		t.Errorf("삭제글 + 회원 = %q, 빈 문자열이어야 한다", item["author_ip"])
	}
}

func TestCommentIPHiddenFromAnonymousByDefault(t *testing.T) {
	w := newWrite("1.2.3.4")
	w.WrParent = 1
	item := TransformToV1Comment(w)

	got, ok := item["author_ip"]
	if !ok {
		t.Fatal("댓글 author_ip 키가 사라졌다")
	}
	if got != "" {
		t.Errorf("비로그인 댓글 기본값 = %q, 빈 문자열이어야 한다", got)
	}
}

func TestOverrideIPMaskedSkipsIndexOverflow(t *testing.T) {
	w := newWrite("1.2.3.4")
	items := []map[string]any{
		TransformToV1Comment(w),
		TransformToV1Comment(w),
	}
	// posts 가 items 보다 짧아도 패닉 없이 앞쪽만 채운다.
	OverrideIPMasked(items, []*gnuboard.G5Write{w})

	if items[0]["author_ip"] != "1.♡.3.4" {
		t.Errorf("첫 항목 = %q", items[0]["author_ip"])
	}
	if items[1]["author_ip"] != "" {
		t.Errorf("짝이 없는 항목 = %q, 빈 값으로 남아야 한다", items[1]["author_ip"])
	}
}
