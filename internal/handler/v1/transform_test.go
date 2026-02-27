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
