package handler

import (
	"reflect"
	"testing"
)

func TestMergeGivingCandidates(t *testing.T) {
	participants := []string{"p1", "p2"}
	commenters := []givingCommenterRow{
		{MbID: "host", WrName: "주최자"},
		{MbID: "p2", WrName: "댓글이름2"},
		{MbID: "c1", WrName: "댓글이름1"},
		{MbID: "c2", WrName: " "},
	}
	nicks := map[string]string{"p1": "닉1", "p2": "닉2", "host": "주최"}

	got := mergeGivingCandidates(participants, commenters, "host", nicks)
	want := []givingCandidate{
		{MbID: "p1", Nick: "닉1"},
		{MbID: "p2", Nick: "닉2"},
		{MbID: "c1", Nick: "댓글이름1"},
		{MbID: "c2", Nick: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMergeGivingCandidatesEmpty(t *testing.T) {
	got := mergeGivingCandidates(nil, nil, "host", nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("expected empty non-nil slice, got %#v", got)
	}
}
