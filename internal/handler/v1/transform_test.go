package v1handler

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

func TestTransformToV1Post(t *testing.T) {
	post := &gnuboard.G5Write{
		WrID:       123,
		WrSubject:  "Test Title",
		WrName:     "TestUser",
		MbID:       "testuser",
		CaName:     "General",
		WrHit:      100,
		WrGood:     10,
		WrNogood:   2,
		WrComment:  5,
		WrFile:     1,
		WrDatetime: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		WrLast:     "2024-01-15 11:00:00",
	}

	result := TransformToV1Post(post, true)

	if result["id"] != 123 {
		t.Errorf("expected id=123, got %v", result["id"])
	}
	if result["title"] != "Test Title" {
		t.Errorf("expected title='Test Title', got %v", result["title"])
	}
	if result["author"] != "TestUser" {
		t.Errorf("expected author='TestUser', got %v", result["author"])
	}
	if result["author_id"] != "testuser" {
		t.Errorf("expected author_id='testuser', got %v", result["author_id"])
	}
	if result["views"] != 100 {
		t.Errorf("expected views=100, got %v", result["views"])
	}
	if result["likes"] != 10 {
		t.Errorf("expected likes=10, got %v", result["likes"])
	}
	if result["is_notice"] != true {
		t.Errorf("expected is_notice=true, got %v", result["is_notice"])
	}
	if result["has_file"] != true {
		t.Errorf("expected has_file=true, got %v", result["has_file"])
	}
}

func TestTransformToV1PostDetail(t *testing.T) {
	post := &gnuboard.G5Write{
		WrID:       123,
		WrSubject:  "Test Title",
		WrContent:  "<p>Test content here</p>",
		WrName:     "TestUser",
		MbID:       "testuser",
		WrDatetime: time.Now(),
	}

	result := TransformToV1PostDetail(post, false)

	if result["content"] != "<p>Test content here</p>" {
		t.Errorf("expected content='<p>Test content here</p>', got %v", result["content"])
	}
	if result["is_notice"] != false {
		t.Errorf("expected is_notice=false, got %v", result["is_notice"])
	}
}

func TestTransformToV1Comment(t *testing.T) {
	comment := &gnuboard.G5Write{
		WrID:           456,
		WrParent:       123,
		WrContent:      "This is a comment",
		WrName:         "Commenter",
		MbID:           "commenter1",
		WrGood:         3,
		WrNogood:       1,
		WrCommentReply: "AA", // depth 2
		WrDatetime:     time.Now(),
	}

	result := TransformToV1Comment(comment)

	if result["id"] != 456 {
		t.Errorf("expected id=456, got %v", result["id"])
	}
	if result["post_id"] != 123 {
		t.Errorf("expected post_id=123, got %v", result["post_id"])
	}
	if result["content"] != "This is a comment" {
		t.Errorf("expected content='This is a comment', got %v", result["content"])
	}
	if result["depth"] != 2 {
		t.Errorf("expected depth=2, got %v", result["depth"])
	}
}

func TestBuildNoticeIDMap(t *testing.T) {
	ids := []int{1, 5, 10, 15}
	m := BuildNoticeIDMap(ids)

	for _, id := range ids {
		if !m[id] {
			t.Errorf("expected %d to be in map", id)
		}
	}

	if m[2] {
		t.Error("expected 2 to not be in map")
	}
	if m[100] {
		t.Error("expected 100 to not be in map")
	}
}

func TestTransformToV1Board(t *testing.T) {
	board := &gnuboard.G5Board{
		BoTable:        "free",
		BoSubject:      "자유게시판",
		GrID:           "community",
		BoListLevel:    0,
		BoReadLevel:    1,
		BoWriteLevel:   2,
		BoCommentLevel: 1,
		BoUseCategory:  1,
		BoCategoryList: "일반,질문,정보",
		BoUseGood:      1,
		BoUseNogood:    0,
		BoCountWrite:   1000,
		BoCountComment: 5000,
	}

	result := TransformToV1Board(board)

	if result["id"] != "free" {
		t.Errorf("expected id='free', got %v", result["id"])
	}
	if result["slug"] != "free" {
		t.Errorf("expected slug='free', got %v", result["slug"])
	}
	if result["name"] != "자유게시판" {
		t.Errorf("expected name='자유게시판', got %v", result["name"])
	}
	if result["use_category"] != true {
		t.Errorf("expected use_category=true, got %v", result["use_category"])
	}
	if result["use_good"] != true {
		t.Errorf("expected use_good=true, got %v", result["use_good"])
	}
	if result["use_nogood"] != false {
		t.Errorf("expected use_nogood=false, got %v", result["use_nogood"])
	}
}

// #13174: 삭제글 tombstone — 민감 필드는 서버가 drop 한다.
func TestTransformToV1PostDeletedTombstone(t *testing.T) {
	deletedAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	author := "victim_author"
	post := &gnuboard.G5Write{
		WrID:        77,
		WrSubject:   "진짜 제목",
		WrName:      "닉네임",
		MbID:        author,
		WrHit:       3600,
		WrGood:      21,
		WrComment:   12,
		WrIP:        "1.2.3.4",
		Wr10:        "https://cdn.example/thumb.png",
		WrDatetime:  time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC),
		WrDeletedAt: &deletedAt,
		WrDeletedBy: &author,
	}

	result := TransformToV1Post(post, false)

	// 유지 키 집합이 정확히 이 7개여야 한다 — 늘어나면 유출, 줄어들면 프론트 파손.
	want := map[string]bool{
		"id": true, "title": true, "is_notice": true, "comments_count": true,
		"created_at": true, "deleted_at": true, "self_deleted": true,
	}
	for k := range result {
		if !want[k] {
			t.Errorf("tombstone 에 예상 밖 키 %q 가 실렸다 (값 %v)", k, result[k])
		}
	}
	for k := range want {
		if _, ok := result[k]; !ok {
			t.Errorf("tombstone 에 필수 키 %q 가 없다", k)
		}
	}
	if result["title"] != "" {
		t.Errorf("삭제글 원제가 남았다: %v", result["title"])
	}
	if result["self_deleted"] != true {
		t.Errorf("deleted_by==author 인데 self_deleted=%v", result["self_deleted"])
	}
}

func TestTransformToV1PostSelfDeletedJudgement(t *testing.T) {
	deletedAt := time.Now()
	admin := "admin"
	cases := []struct {
		name      string
		deletedBy *string
		want      bool
	}{
		{"관리자 삭제", &admin, false},
		{"삭제자 미상(nil)", nil, false},
	}
	for _, tc := range cases {
		post := &gnuboard.G5Write{
			WrID: 1, MbID: "writer", WrDatetime: time.Now(),
			WrDeletedAt: &deletedAt, WrDeletedBy: tc.deletedBy,
		}
		got := TransformToV1Post(post, false)["self_deleted"]
		if got != tc.want {
			t.Errorf("%s: self_deleted=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTransformToV1PostsSummaryDeletedNoMediaFlags(t *testing.T) {
	deletedAt := time.Now()
	posts := []*gnuboard.G5Write{{
		WrID: 5, WrDatetime: time.Now(), Wr9: "video", Wr10: "img",
		WrDeletedAt: &deletedAt,
	}}
	items := TransformToV1PostsSummary(posts, map[int]bool{})
	if _, ok := items[0]["has_image"]; ok {
		t.Error("tombstone 에 has_image 가 주입됐다")
	}
	if _, ok := items[0]["has_video"]; ok {
		t.Error("tombstone 에 has_video 가 주입됐다")
	}
}

// #13174: 목록 핸들러가 admin IP override 를 공유 캐시 저장 전에 실행하므로,
// 삭제글 스킵 가드가 없으면 tombstone+IP 가 캐시에 실려 전원에게 나간다.
func TestOverrideIPForAdminSkipsDeleted(t *testing.T) {
	deletedAt := time.Now()
	live := &gnuboard.G5Write{WrID: 1, WrIP: "10.0.0.1", WrDatetime: time.Now()}
	dead := &gnuboard.G5Write{WrID: 2, WrIP: "10.0.0.2", WrDatetime: time.Now(), WrDeletedAt: &deletedAt}
	items := TransformToV1Posts([]*gnuboard.G5Write{live, dead}, map[int]bool{})

	OverrideIPForAdmin(items, []*gnuboard.G5Write{live, dead})
	if items[0]["author_ip"] != "10.0.0.1" {
		t.Errorf("생존 글 IP override 실패: %v", items[0]["author_ip"])
	}
	if _, ok := items[1]["author_ip"]; ok {
		t.Error("tombstone 에 author_ip 가 실렸다")
	}

	single := TransformToV1Post(dead, false)
	OverrideIPForAdminSingle(single, dead)
	if _, ok := single["author_ip"]; ok {
		t.Error("OverrideIPForAdminSingle 이 tombstone 에 IP 를 실었다")
	}
}

func TestTransformToV1PostDetailUnmaskedKeepsOriginal(t *testing.T) {
	deletedAt := time.Now()
	deleter := "admin"
	post := &gnuboard.G5Write{
		WrID: 9, WrSubject: "원제", WrName: "닉", MbID: "writer",
		WrContent: "<p>본문</p>", WrHit: 42,
		WrDatetime: time.Now(), WrDeletedAt: &deletedAt, WrDeletedBy: &deleter,
	}
	result := TransformToV1PostDetailUnmasked(post, false)
	if result["title"] != "원제" {
		t.Errorf("Unmasked 인데 title=%v", result["title"])
	}
	if result["views"] != 42 {
		t.Errorf("Unmasked 인데 views=%v", result["views"])
	}
	if result["content"] == "" || result["content"] == nil {
		t.Error("Unmasked 인데 content 가 비었다")
	}
	if result["self_deleted"] != false {
		t.Errorf("관리자 삭제인데 self_deleted=%v", result["self_deleted"])
	}
	// 일반 상세는 tombstone 이어야 한다
	masked := TransformToV1PostDetail(post, false)
	if _, ok := masked["author"]; ok {
		t.Error("일반 상세 tombstone 에 author 가 실렸다")
	}
	if _, ok := masked["content"]; ok {
		t.Error("일반 상세 tombstone 에 content 가 실렸다")
	}
}

// #13174 후속: 삭제 댓글 tombstone — 원문·작성자·IP 서버 drop
func TestTransformToV1CommentDeletedTombstone(t *testing.T) {
	deletedAt := time.Now()
	author := "writer"
	c := &gnuboard.G5Write{
		WrID: 3, WrParent: 100, WrContent: "원문", WrName: "닉", MbID: author,
		WrIP: "1.2.3.4", WrDatetime: time.Now(),
		WrDeletedAt: &deletedAt, WrDeletedBy: &author,
	}
	result := TransformToV1Comment(c)
	want := map[string]bool{
		"id": true, "post_id": true, "content": true, "depth": true,
		"created_at": true, "deleted_at": true, "self_deleted": true,
	}
	for k := range result {
		if !want[k] {
			t.Errorf("삭제 댓글 tombstone 에 예상 밖 키 %q (값 %v)", k, result[k])
		}
	}
	if result["content"] != "" {
		t.Errorf("삭제 댓글 원문이 남았다: %v", result["content"])
	}
	if result["self_deleted"] != true {
		t.Errorf("자진삭제인데 self_deleted=%v", result["self_deleted"])
	}
}
