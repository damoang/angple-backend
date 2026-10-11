package service

import (
	"strings"
	"testing"
	"time"
)

var testKST = time.FixedZone("KST", 9*60*60)

func strp(s string) *string { return &s }

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, testKST)
	if err != nil {
		panic(err)
	}
	return t
}

func upd(version int, by, content, when string) RevisionRecord {
	return RevisionRecord{Version: version, ChangeType: "update", Content: strp(content), EditedBy: by, EditedAt: at(when)}
}

func contents(revs []PublicRevision) []string {
	out := make([]string, 0, len(revs))
	for _, r := range revs {
		out = append(out, r.Content)
	}
	return out
}

func assertContents(t *testing.T, revs []PublicRevision, want ...string) {
	t.Helper()
	got := contents(revs)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("contents = %q, want %q", got, want)
	}
}

func TestBuildPublicRevisions_NoEditsIsEmpty(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CurrentContent: "C0",
		Records: []RevisionRecord{
			{Version: 1, ChangeType: "create", Content: strp("C0"), EditedBy: "author", EditedAt: at("2026-10-02 10:00")},
		},
	}
	if got := BuildPublicRevisions(src); len(got) != 0 {
		t.Fatalf("수정이 없으면 빈 목록이어야 한다: %+v", got)
	}
	if CountPublicEdits(src) != 0 {
		t.Fatal("수정 횟수 0 이어야 한다")
	}
}

// 수정 행은 「수정 직전 내용」을 담는다. 현재 내용은 게시판 테이블에서 온다.
func TestBuildPublicRevisions_AuthorEditsReconstructSnapshots(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CreatedAt:      at("2026-10-02 09:59"),
		CurrentTitle:   strp("T2"),
		CurrentContent: "C2",
		Records: []RevisionRecord{
			{Version: 1, ChangeType: "create", Title: strp("T0"), Content: strp("C0"), EditedBy: "author", EditedAt: at("2026-10-02 10:00")},
			{Version: 2, ChangeType: "update", Title: strp("T0"), Content: strp("C0"), EditedBy: "author", EditedAt: at("2026-10-02 10:05")},
			{Version: 3, ChangeType: "update", Title: strp("T1"), Content: strp("C1"), EditedBy: "author", EditedAt: at("2026-10-02 10:30")},
		},
	}
	revs := BuildPublicRevisions(src)
	assertContents(t, revs, "C0", "C1", "C2")
	if revs[0].Version != 2 || revs[1].Version != 3 || revs[2].Version != PublicRevisionCurrentVersion {
		t.Fatalf("버전 매핑이 다르다: %+v", revs)
	}
	if !revs[2].Current || revs[0].Current || revs[1].Current {
		t.Fatal("마지막 항목만 현재 내용이어야 한다")
	}
	// 원본 시각은 create 행 시각, 이후는 그 내용을 만든 수정 시각.
	if revs[0].EditedAt == nil || !revs[0].EditedAt.Equal(at("2026-10-02 10:00")) {
		t.Fatalf("원본 시각 = %v", revs[0].EditedAt)
	}
	if !revs[1].EditedAt.Equal(at("2026-10-02 10:05")) || !revs[2].EditedAt.Equal(at("2026-10-02 10:30")) {
		t.Fatalf("수정 시각 매핑이 다르다: %v %v", revs[1].EditedAt, revs[2].EditedAt)
	}
	if *revs[0].Title != "T0" || *revs[2].Title != "T2" {
		t.Fatal("제목 매핑이 다르다")
	}
	if CountPublicEdits(src) != 2 {
		t.Fatalf("수정 횟수 = %d, want 2", CountPublicEdits(src))
	}
}

// 날짜 기준이 없다 — 오래된 수정도 처음부터 공개였던 것처럼 보인다.
func TestBuildPublicRevisions_NoDateCutoff(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CreatedAt:      at("2024-03-01 08:00"),
		CurrentContent: "C2",
		Records: []RevisionRecord{
			upd(1, "author", "C0", "2024-03-01 09:00"),
			upd(2, "author", "C1", "2025-12-31 23:59"),
		},
	}
	revs := BuildPublicRevisions(src)
	assertContents(t, revs, "C0", "C1", "C2")
	if !revs[0].EditedAt.Equal(at("2024-03-01 08:00")) {
		t.Fatalf("create 행이 없으면 글 작성 시각을 쓴다: %v", revs[0].EditedAt)
	}
}

// 작성자가 아닌 사람의 수정이 있으면, 마지막 비작성자 수정 이전 내용은 전부 숨긴다.
func TestBuildPublicRevisions_HidesEverythingBeforeLastNonAuthorEdit(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CurrentContent: "C4",
		Records: []RevisionRecord{
			upd(2, "author", "C0", "2026-10-02 10:00"),
			upd(3, "staff", "C1-private@example.com", "2026-10-02 11:00"),
			upd(4, "author", "C2", "2026-10-02 12:00"),
			upd(5, "staff", "C3-private-again", "2026-10-02 13:00"),
			upd(6, "author", "C4-prev", "2026-10-02 14:00"),
		},
	}
	revs := BuildPublicRevisions(src)
	assertContents(t, revs, "C4-prev", "C4")
	for _, r := range revs {
		if strings.Contains(r.Content, "private") || r.Content == "C0" || r.Content == "C2" {
			t.Fatalf("비작성자 수정 이전 내용이 노출됐다: %q", r.Content)
		}
	}
	// 기준 내용의 시각 = 비작성자 수정 시각이라 싣지 않는다.
	if revs[0].EditedAt != nil {
		t.Fatalf("비작성자 수정 직후 기준 내용은 시각이 없어야 한다: %v", revs[0].EditedAt)
	}
	if revs[1].EditedAt == nil || !revs[1].EditedAt.Equal(at("2026-10-02 14:00")) {
		t.Fatalf("작성자 수정 시각 = %v", revs[1].EditedAt)
	}
}

func TestBuildPublicRevisions_LastEditByNonAuthorIsEmpty(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CurrentContent: "C2",
		Records: []RevisionRecord{
			upd(2, "author", "C0", "2026-10-02 10:00"),
			upd(3, "staff", "C1", "2026-10-02 11:00"),
		},
	}
	if got := BuildPublicRevisions(src); len(got) != 0 {
		t.Fatalf("마지막 수정이 비작성자면 공개할 작성자 수정이 없다: %+v", got)
	}
}

func TestBuildPublicRevisions_HiddenVersionsExcluded(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CurrentContent: "C3",
		Records: []RevisionRecord{
			upd(2, "author", "C0", "2026-10-02 10:00"),
			upd(3, "author", "C1-hidden", "2026-10-02 11:00"),
			upd(4, "author", "C2", "2026-10-02 12:00"),
		},
		Hidden: map[int]bool{3: true},
	}
	assertContents(t, BuildPublicRevisions(src), "C0", "C2", "C3")

	// 현재 내용만 남으면 빈 목록.
	src.Hidden = map[int]bool{2: true, 3: true, 4: true}
	if got := BuildPublicRevisions(src); len(got) != 0 {
		t.Fatalf("현재 내용만 남으면 빈 목록: %+v", got)
	}
	// 현재 내용(버전 0)은 숨김 대상이 아니다.
	src.Hidden = map[int]bool{PublicRevisionCurrentVersion: true}
	assertContents(t, BuildPublicRevisions(src), "C0", "C1-hidden", "C2", "C3")
}

func TestBuildPublicRevisions_MergesUnchangedSaves(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "author",
		CurrentContent: "C1",
		Records: []RevisionRecord{
			upd(2, "author", "C0", "2026-10-02 10:00"),
			upd(3, "author", "C0", "2026-10-02 10:01"), // 바꾸지 않고 저장
		},
	}
	revs := BuildPublicRevisions(src)
	assertContents(t, revs, "C0", "C1")
	if CountPublicEdits(src) != 1 {
		t.Fatal("내용이 그대로인 저장은 수정 횟수에 들지 않는다")
	}

	// 마지막 저장이 아무것도 안 바꿨으면 현재 내용 하나 → 빈 목록.
	src.CurrentContent = "C0"
	if got := BuildPublicRevisions(src); len(got) != 0 {
		t.Fatalf("내용 변화가 없으면 빈 목록: %+v", got)
	}
}

func TestBuildPublicRevisions_GuestAuthorIsEmpty(t *testing.T) {
	src := PublicRevisionSource{
		AuthorID:       "",
		CurrentContent: "C1",
		Records:        []RevisionRecord{upd(2, "", "C0", "2026-10-02 10:00")},
	}
	if got := BuildPublicRevisions(src); len(got) != 0 {
		t.Fatalf("작성자 ID 가 없으면 공개하지 않는다: %+v", got)
	}
}

func TestPublicRevisionConfig_StaffAuthor(t *testing.T) {
	cfg := PublicRevisionConfig{Boards: map[string]bool{"claim": true}, StaffIDs: map[string]bool{"role1": true}}
	cases := []struct {
		name       string
		author     string
		level      int
		boardAdmin string
		want       bool
	}{
		{"사이트 관리자", "someone", 10, "", true},
		{"설정 목록", "role1", 4, "", true},
		{"게시판 관리자", "boardadm", 4, "boardadm", true},
		{"게시판 관리자 여럿", "b2", 4, "b1, b2", true},
		{"일반 회원", "member", 4, "boardadm", false},
		{"빈 작성자", "", 10, "", false},
	}
	for _, tc := range cases {
		if got := cfg.IsStaffAuthor(tc.author, tc.level, tc.boardAdmin); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestLoadPublicRevisionConfig(t *testing.T) {
	t.Setenv("PUBLIC_REVISION_BOARDS", "")
	t.Setenv("PUBLIC_REVISION_STAFF_IDS", "")
	cfg := LoadPublicRevisionConfig()
	if !cfg.BoardEnabled("claim") || cfg.BoardEnabled("free") || len(cfg.StaffIDs) != 0 {
		t.Fatalf("기본값은 claim 하나, 운영진 목록 없음: %+v", cfg)
	}

	t.Setenv("PUBLIC_REVISION_BOARDS", "claim, qa")
	t.Setenv("PUBLIC_REVISION_STAFF_IDS", "role1,role2")
	cfg = LoadPublicRevisionConfig()
	if !cfg.BoardEnabled("qa") || !cfg.StaffIDs["role2"] || cfg.BoardEnabled("free") {
		t.Fatalf("환경 변수 값이 반영돼야 한다: %+v", cfg)
	}
}

func TestIsHideableVersion(t *testing.T) {
	records := []RevisionRecord{
		{Version: 1, ChangeType: "create"},
		upd(2, "author", "C0", "2026-10-02 10:00"),
	}
	if !IsHideableVersion(records, 2) {
		t.Fatal("update 행 버전은 숨길 수 있다")
	}
	if IsHideableVersion(records, 1) || IsHideableVersion(records, 0) || IsHideableVersion(records, 9) {
		t.Fatal("create 행·현재 내용·없는 버전은 숨길 수 없다")
	}
}
