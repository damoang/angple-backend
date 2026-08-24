package gnuboard

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// bug/13675: 글을 소프트삭제해도 자식 댓글(특히 다른 앙님의 댓글)은 보존돼야 한다.
// 2026-03 에 원글 작성자가 자기 글을 지우며 타인 댓글까지 연쇄삭제되던 동작은 제거됐다.
// 누군가 연쇄삭제를 재도입하면(SoftDeletePost 가 자식 댓글 wr_deleted_at 을 건드리면)
// 이 테스트가 실패한다.
func TestSoftDeletePostPreservesChildComments(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wr_softdelete_preserve?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY,
		wr_num INTEGER NOT NULL DEFAULT 0,
		wr_reply TEXT NOT NULL DEFAULT '',
		wr_parent INTEGER NOT NULL DEFAULT 0,
		wr_is_comment INTEGER NOT NULL DEFAULT 0,
		wr_comment INTEGER NOT NULL DEFAULT 0,
		wr_comment_reply TEXT NOT NULL DEFAULT '',
		ca_name TEXT,
		wr_option TEXT,
		wr_subject TEXT,
		wr_content TEXT,
		wr_link1 TEXT,
		wr_link2 TEXT,
		wr_link1_hit INTEGER NOT NULL DEFAULT 0,
		wr_link2_hit INTEGER NOT NULL DEFAULT 0,
		wr_hit INTEGER NOT NULL DEFAULT 0,
		wr_good INTEGER NOT NULL DEFAULT 0,
		wr_nogood INTEGER NOT NULL DEFAULT 0,
		wr_name TEXT,
		mb_id TEXT,
		wr_password TEXT,
		wr_email TEXT,
		wr_homepage TEXT,
		wr_datetime DATETIME,
		wr_file INTEGER NOT NULL DEFAULT 0,
		wr_last TEXT,
		wr_ip TEXT,
		wr_9 TEXT,
		wr_10 TEXT,
		wr_deleted_at DATETIME NULL,
		wr_deleted_by TEXT NULL,
		wr_edit_count INTEGER NOT NULL DEFAULT 0,
		wr_last_edited_at DATETIME NULL
	)`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_da_content_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bo_table TEXT,
		wr_id INTEGER,
		wr_is_comment INTEGER,
		mb_id TEXT,
		wr_name TEXT,
		operation TEXT,
		operated_by TEXT,
		operated_at DATETIME,
		previous_data TEXT
	)`).Error; err != nil {
		t.Fatalf("create g5_da_content_history: %v", err)
	}

	// 글(author) + 댓글 2개: 하나는 글쓴이 본인(author), 하나는 다른 사람(other).
	if err := db.Exec(`
		INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, wr_comment, wr_content, wr_subject, wr_name, mb_id, wr_deleted_at, wr_deleted_by) VALUES
		(1, 1, 0, 2, 'post body',  'post title', 'author', 'author', NULL, NULL),
		(2, 1, 1, 0, 'own comment',   '', 'author', 'author', NULL, NULL),
		(3, 1, 1, 0, 'other comment', '', 'other',  'other',  NULL, NULL)
	`).Error; err != nil {
		t.Fatalf("seed g5_write_free: %v", err)
	}

	repo := NewWriteRepository(db)
	if err := repo.SoftDeletePost("free", 1, "author"); err != nil {
		t.Fatalf("SoftDeletePost: %v", err)
	}

	// 글은 소프트삭제됐어야 한다.
	var postDeletedAt *string
	if err := db.Table("g5_write_free").Select("wr_deleted_at").Where("wr_id = 1").Scan(&postDeletedAt).Error; err != nil {
		t.Fatalf("query post: %v", err)
	}
	if postDeletedAt == nil {
		t.Fatalf("글이 소프트삭제되지 않았다 (wr_deleted_at NULL)")
	}

	// 자식 댓글은 둘 다 보존돼야 한다 — 특히 타인 댓글(wr_id=3).
	for _, id := range []int{2, 3} {
		var delAt *string
		if err := db.Table("g5_write_free").Select("wr_deleted_at").Where("wr_id = ?", id).Scan(&delAt).Error; err != nil {
			t.Fatalf("query comment %d: %v", id, err)
		}
		if delAt != nil {
			t.Fatalf("댓글 %d 은 글 삭제 후에도 보존돼야 하는데 wr_deleted_at=%v (연쇄삭제 재도입?)", id, *delAt)
		}
	}
}
