package gnuboard

import (
	"testing"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWriteRepositorySoftDeleteAndRestoreCommentAdjustsParentCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wr_comment_delete_restore?mode=memory&cache=shared"), &gorm.Config{})
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
		wr_deleted_by TEXT NULL
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
	if err := db.Exec(`
		INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, wr_comment, wr_comment_reply, wr_content, wr_name, mb_id, wr_deleted_at, wr_deleted_by) VALUES
		(1, 1, 0, 1, '', 'post', 'author', 'author', NULL, NULL),
		(2, 1, 1, 0, '', 'comment', 'commenter', 'commenter', NULL, NULL)
	`).Error; err != nil {
		t.Fatalf("seed g5_write_free: %v", err)
	}

	repo := NewWriteRepository(db)
	if err := repo.SoftDeleteComment("free", 2, "admin"); err != nil {
		t.Fatalf("SoftDeleteComment: %v", err)
	}

	var postCount int
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 1").Scan(&postCount).Error; err != nil {
		t.Fatalf("query post count after delete: %v", err)
	}
	if postCount != 0 {
		t.Fatalf("expected post count 0 after delete, got %d", postCount)
	}

	if err := repo.SoftDeleteComment("free", 2, "admin"); err != nil {
		t.Fatalf("SoftDeleteComment second call: %v", err)
	}
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 1").Scan(&postCount).Error; err != nil {
		t.Fatalf("query post count after second delete: %v", err)
	}
	if postCount != 0 {
		t.Fatalf("expected post count to stay 0 after second delete, got %d", postCount)
	}

	if err := repo.RestoreComment("free", 2); err != nil {
		t.Fatalf("RestoreComment: %v", err)
	}
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 1").Scan(&postCount).Error; err != nil {
		t.Fatalf("query post count after restore: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected post count 1 after restore, got %d", postCount)
	}

	comment, err := repo.FindCommentByID("free", 2)
	if err != nil {
		t.Fatalf("FindCommentByID: %v", err)
	}
	if comment.WrDeletedAt != nil || comment.WrDeletedBy != nil {
		t.Fatalf("expected comment restored, got deleted_at=%v deleted_by=%v", comment.WrDeletedAt, comment.WrDeletedBy)
	}
}

func TestWriteRepositoryCreateCommentAdjustsParentCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wr_comment_create?mode=memory&cache=shared"), &gorm.Config{})
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
		mb_id TEXT,
		wr_password TEXT,
		wr_name TEXT,
		wr_email TEXT,
		wr_homepage TEXT,
		wr_datetime DATETIME,
		wr_file INTEGER NOT NULL DEFAULT 0,
		wr_last TEXT,
		wr_ip TEXT,
		wr_9 TEXT,
		wr_10 TEXT,
		wr_deleted_at DATETIME NULL,
		wr_deleted_by TEXT NULL
	)`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}
	if err := db.Exec(`INSERT INTO g5_write_free (wr_id, wr_parent, wr_is_comment, wr_comment, wr_comment_reply, wr_datetime) VALUES (1, 1, 0, 0, '', CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatalf("seed post: %v", err)
	}

	repo := NewWriteRepository(db)
	comment := &gnudomain.G5Write{
		WrID:           2,
		WrParent:       1,
		WrIsComment:    1,
		WrCommentReply: "",
		WrDatetime:     time.Now(),
	}
	if err := repo.CreateComment("free", comment); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	var postCount int
	if err := db.Table("g5_write_free").Select("wr_comment").Where("wr_id = 1").Scan(&postCount).Error; err != nil {
		t.Fatalf("query post count: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected post count 1 after create, got %d", postCount)
	}
}
