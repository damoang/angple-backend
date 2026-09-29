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
		evidence    bool
		wantContent string
		wantKind    string
	}{
		{"부모 잠금 아님 — 그대로", ActivityComment{WrContent: "원문", ContentKind: "text"}, false, "원문", "text"},
		{"부모 잠금 — 내용을 가리고 종류를 비운다", ActivityComment{WrContent: "<img src=x>", ContentKind: "image", ParentLocked: true}, false, LockedParentCommentMask, ""},
		{"부모 잠금이지만 근거글 — 기존 동작 유지", ActivityComment{WrContent: "원문", ContentKind: "text", ParentLocked: true}, true, "원문", "text"},
		{"부모 잠금이지만 댓글 삭제됨 — 자리표시자 유지", ActivityComment{WrContent: "", ContentKind: "empty", ParentLocked: true, DeletedAt: &deleted}, false, "", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskLockedParentActivityComment(tc.in, tc.evidence)
			if got.WrContent != tc.wantContent {
				t.Fatalf("WrContent = %q, want %q", got.WrContent, tc.wantContent)
			}
			if got.ContentKind != tc.wantKind {
				t.Fatalf("ContentKind = %q, want %q", got.ContentKind, tc.wantKind)
			}
			if got.ParentLocked != tc.in.ParentLocked {
				t.Fatalf("ParentLocked 가 바뀌면 안 된다: got %v", got.ParentLocked)
			}
		})
	}
}
