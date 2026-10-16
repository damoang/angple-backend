package worker

import (
	"context"
	"testing"

	"github.com/damoang/angple-backend/internal/activitycache"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 피드 동기화가 실패해도(여기서는 테이블이 없어 실패) 이벤트 작성자의 활동 캐시는 지워야 한다.
func TestWriteAfterWorker_SyncFailureStillInvalidatesAuthor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	w := &WriteAfterWorker{activitySync: service.NewMemberActivitySyncService(db)}

	deleted := map[string]bool{}
	restore := activitycache.SetDeleterForTest(func(_ context.Context, keys ...string) error {
		for _, k := range keys {
			deleted[k] = true
		}
		return nil
	})
	defer restore()

	postID := 1
	if err := w.handleCommentChanged(gnudomain.WriteAfterEvent{
		EventType: gnudomain.WriteAfterEventTypeCommentDeleted,
		BoardSlug: "free",
		WriteID:   11,
		PostID:    &postID,
		MemberID:  "commenter",
	}); err != nil {
		t.Fatalf("handleCommentChanged: %v", err)
	}
	if !deleted["member_activity:commenter:5"] {
		t.Fatalf("comment author cache not invalidated on sync failure: %v", deleted)
	}

	w.handlePostChanged(gnudomain.WriteAfterEvent{
		EventType: gnudomain.WriteAfterEventTypePostDeleted,
		BoardSlug: "free",
		WriteID:   1,
		MemberID:  "author",
	})
	if !deleted["member_activity:author:l5"] {
		t.Fatalf("post author cache not invalidated on sync failure: %v", deleted)
	}
}
