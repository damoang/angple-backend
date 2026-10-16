package cron

import (
	"testing"

	"github.com/damoang/angple-backend/internal/activitycache"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 홍보 글·댓글의 공개 여부가 바뀌면 행마다 후처리 이벤트가 남아야 한다
// (워커가 피드 공개 여부를 맞추고 작성자·댓글 작성자 캐시를 지운다).
func TestEnqueuePromotionVisibilityEvents_PostAndComment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`CREATE TABLE g5_write_after_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_type TEXT, board_slug TEXT, write_id INTEGER, post_id INTEGER, parent_id INTEGER,
		member_id TEXT, author TEXT, subject TEXT,
		occurred_at DATETIME, available_at DATETIME, status TEXT,
		retry_count INTEGER DEFAULT 0, last_error TEXT, claimed_at DATETIME, processed_at DATETIME,
		created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create events: %v", err)
	}
	restore := activitycache.SetDeleterForTest(nil)
	defer restore()

	enqueuePromotionVisibilityEvents(db, "advertiser", []promotionVisibilityRow{
		{WrID: 1, WrParent: 1, WrIsComment: 0, WrName: "광고주", WrSubject: "홍보"},
		{WrID: 2, WrParent: 1, WrIsComment: 1, WrName: "광고주"},
	})

	var events []gnudomain.WriteAfterEvent
	if err := db.Order("id").Find(&events).Error; err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	post, comment := events[0], events[1]
	if post.EventType != gnudomain.WriteAfterEventTypePostUpdated || post.BoardSlug != "promotion" ||
		post.WriteID != 1 || post.PostID != nil || post.MemberID != "advertiser" ||
		post.Status != gnudomain.WriteAfterEventStatusPending {
		t.Fatalf("unexpected post event: %+v", post)
	}
	if comment.EventType != gnudomain.WriteAfterEventTypeCommentUpdated || comment.WriteID != 2 ||
		comment.PostID == nil || *comment.PostID != 1 {
		t.Fatalf("unexpected comment event: %+v", comment)
	}
}
