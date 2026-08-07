package migration

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// RunV2Schema creates all v2 Core tables, Meta tables via AutoMigrate.
// Tables are prefixed with v2_ to coexist with gnuboard g5_* tables.
// This is safe to run multiple times (AutoMigrate is idempotent).
func RunV2Schema(db *gorm.DB) error {
	return db.AutoMigrate(
		// Core tables
		&v2.V2User{},
		&v2.V2Board{},
		&v2.V2Post{},
		&v2.V2Comment{},
		&v2.V2Category{},
		&v2.V2Tag{},
		&v2.V2PostTag{},
		&v2.V2File{},
		&v2.V2Notification{},
		&v2.V2Session{},

		// Scrap, Memo, Message
		&v2.V2Scrap{},
		&v2.V2Memo{},
		&v2.V2Message{},

		// Content revisions
		&v2.V2ContentRevision{},

		// Push notification outbox cursor (PushNotifyWorker watermark)
		&v2.V2PushCursor{},

		// Board display settings - table already exists, skip auto-migration
		// &v2.V2BoardDisplaySettings{},

		// Meta tables (plugin extensibility)
		&v2.UserMeta{},
		&v2.PostMeta{},
		&v2.CommentMeta{},
		&v2.OptionMeta{},
	)
}
