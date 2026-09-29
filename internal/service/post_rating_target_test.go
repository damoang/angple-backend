package service

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

type fakeRatingTargetFinder struct {
	posts    map[int]*gnuboard.G5Write
	comments map[int]*gnuboard.G5Write
}

var errFakeNotFound = errors.New("record not found")

func (f *fakeRatingTargetFinder) FindPostByIDIncludeDeleted(_ string, wrID int) (*gnuboard.G5Write, error) {
	if p, ok := f.posts[wrID]; ok {
		return p, nil
	}
	return nil, errFakeNotFound
}

func (f *fakeRatingTargetFinder) FindCommentByID(_ string, wrID int) (*gnuboard.G5Write, error) {
	if c, ok := f.comments[wrID]; ok {
		return c, nil
	}
	return nil, errFakeNotFound
}

func TestValidateRatingTarget(t *testing.T) {
	deleted := time.Now()
	finder := &fakeRatingTargetFinder{
		posts: map[int]*gnuboard.G5Write{
			10: {WrID: 10, MbID: "writer"},
			11: {WrID: 11, MbID: "writer", WrDeletedAt: &deleted},
		},
		comments: map[int]*gnuboard.G5Write{
			20: {WrID: 20, WrParent: 10, WrIsComment: 1, MbID: "reviewer"},
			21: {WrID: 21, WrParent: 10, WrIsComment: 1, MbID: "reviewer", WrDeletedAt: &deleted},
			22: {WrID: 22, WrParent: 11, WrIsComment: 1, MbID: "reviewer"},
			23: {WrID: 23, WrParent: 999, WrIsComment: 1, MbID: "reviewer"},
		},
	}

	tests := []struct {
		name string
		wrID int
		mbID string
		want error
	}{
		{"게시글은 누구나(기존 규칙)", 10, "someone", nil},
		{"삭제된 게시글도 기존처럼 허용", 11, "someone", nil},
		{"본인 댓글 허용", 20, "reviewer", nil},
		{"남의 댓글 거부", 20, "someone", ErrRatingCommentNotOwner},
		{"삭제된 댓글 거부", 21, "reviewer", ErrRatingTargetNotFound},
		{"부모 글 삭제 시 거부", 22, "reviewer", ErrRatingTargetNotFound},
		{"부모 글 없음 거부", 23, "reviewer", ErrRatingTargetNotFound},
		{"없는 wr_id 거부", 404, "reviewer", ErrRatingTargetNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRatingTarget(finder, "angtt", tt.wrID, tt.mbID)
			if !errors.Is(got, tt.want) {
				t.Fatalf("ValidateRatingTarget(%d, %q) = %v, want %v", tt.wrID, tt.mbID, got, tt.want)
			}
		})
	}
}
