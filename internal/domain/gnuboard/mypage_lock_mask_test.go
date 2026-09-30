package gnuboard

import (
	"testing"
	"time"
)

func TestMaskLockedActivityPost(t *testing.T) {
	deleted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   ActivityPost
		want string
	}{
		{"잠금 아님 — 제목 그대로", ActivityPost{WrSubject: "원제목"}, "원제목"},
		{"잠금 — 제목을 가린다", ActivityPost{WrSubject: "원제목", IsLocked: true}, LockedPostSubjectMask},
		{"잠금이지만 삭제됨 — 자리표시자(빈 제목) 유지", ActivityPost{WrSubject: "", IsLocked: true, DeletedAt: &deleted}, ""},
		{"삭제만 — 건드리지 않는다", ActivityPost{WrSubject: "", DeletedAt: &deleted}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskLockedActivityPost(tc.in)
			if got.WrSubject != tc.want {
				t.Fatalf("WrSubject = %q, want %q", got.WrSubject, tc.want)
			}
			if got.IsLocked != tc.in.IsLocked {
				t.Fatalf("IsLocked 가 바뀌면 안 된다: got %v", got.IsLocked)
			}
		})
	}
}

func TestMaskLockedParentActivityComment(t *testing.T) {
	deleted := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		in          ActivityComment
		wantContent string
		wantKind    string
		wantBadge   bool
	}{
		{"잠금 없음 — 그대로, 배지 없음", ActivityComment{WrContent: "원문", ContentKind: "text"}, "원문", "text", false},
		// #14041: 잠긴 글에 댓글을 단 것뿐인 회원에게 배지가 붙으면 안 된다.
		{"부모만 잠금 — 가리되 배지 없음", ActivityComment{WrContent: "<img src=x>", ContentKind: "image", ParentLocked: true}, LockedParentCommentMask, "", false},
		// 부모가 근거글이어도 예외가 없다 — 마스킹 판정에 근거글 여부가 들어가지 않는다.
		{"부모 근거글+잠금 — 예외 없이 가리고 배지 없음", ActivityComment{WrContent: "원문", ContentKind: "text", WrParent: 10, ParentLocked: true}, LockedParentCommentMask, "", false},
		{"댓글 자신 잠금 — [신고잠금 댓글]+배지", ActivityComment{WrContent: "원문", ContentKind: "text", SelfLocked: true}, LockedCommentMask, "", true},
		{"둘 다 잠금 — 자신 잠금 문구가 우선+배지", ActivityComment{WrContent: "원문", ContentKind: "emoticon", SelfLocked: true, ParentLocked: true}, LockedCommentMask, "", true},
		{"부모 잠금이지만 댓글 삭제됨 — 자리표시자 유지", ActivityComment{WrContent: "", ContentKind: "empty", ParentLocked: true, DeletedAt: &deleted}, "", "empty", false},
		{"자신 잠금이지만 댓글 삭제됨 — 자리표시자 유지(배지는 잠금 그대로)", ActivityComment{WrContent: "", ContentKind: "empty", SelfLocked: true, DeletedAt: &deleted}, "", "empty", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskLockedParentActivityComment(tc.in)
			if got.WrContent != tc.wantContent {
				t.Fatalf("WrContent = %q, want %q", got.WrContent, tc.wantContent)
			}
			if got.ContentKind != tc.wantKind {
				t.Fatalf("ContentKind = %q, want %q", got.ContentKind, tc.wantKind)
			}
			if got.ShowsLockBadge() != tc.wantBadge {
				t.Fatalf("ShowsLockBadge = %v, want %v", got.ShowsLockBadge(), tc.wantBadge)
			}
			if got.ParentLocked != tc.in.ParentLocked || got.SelfLocked != tc.in.SelfLocked {
				t.Fatalf("잠금 플래그가 바뀌면 안 된다: parent=%v self=%v", got.ParentLocked, got.SelfLocked)
			}
		})
	}
}
