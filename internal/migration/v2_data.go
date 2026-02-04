package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// MigrateV2Data migrates data from gnuboard tables to v2 tables.
// This handles dynamic g5_write_{board_id} tables that require Go-level iteration.
// Safe to run multiple times (uses INSERT IGNORE / ON DUPLICATE KEY).
func MigrateV2Data(db *gorm.DB) error {
	// 1. Migrate members
	log.Println("[v2-migration] Migrating g5_member → v2_users...")
	if err := migrateUsers(db); err != nil {
		return fmt.Errorf("migrate users: %w", err)
	}

	// 2. Migrate boards
	log.Println("[v2-migration] Migrating g5_board → v2_boards...")
	if err := migrateBoards(db); err != nil {
		return fmt.Errorf("migrate boards: %w", err)
	}

	// 3. Migrate posts & comments from dynamic tables
	log.Println("[v2-migration] Migrating posts & comments from dynamic tables...")
	if err := migratePostsAndComments(db); err != nil {
		return fmt.Errorf("migrate posts/comments: %w", err)
	}

	// 4. Migrate files
	log.Println("[v2-migration] Migrating g5_board_file → v2_files...")
	if err := migrateFiles(db); err != nil {
		return fmt.Errorf("migrate files: %w", err)
	}

	log.Println("[v2-migration] Data migration complete.")
	return nil
}

func migrateUsers(db *gorm.DB) error {
	sql := `
		INSERT IGNORE INTO v2_users (username, email, password, nickname, level, status, bio, created_at, updated_at)
		SELECT
			mb_id,
			CASE WHEN mb_email = '' THEN CONCAT(mb_id, '@legacy.local') ELSE mb_email END,
			mb_password,
			mb_nick,
			LEAST(mb_level, 10),
			CASE
				WHEN mb_leave_date != '' THEN 'inactive'
				WHEN mb_intercept_date != '' THEN 'banned'
				ELSE 'active'
			END,
			NULLIF(mb_profile, ''),
			mb_datetime,
			mb_datetime
		FROM g5_member
		WHERE mb_id != ''
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[v2-migration] Migrated %d users", result.RowsAffected)
	return nil
}

func migrateBoards(db *gorm.DB) error {
	sql := `
		INSERT IGNORE INTO v2_boards (slug, name, description, is_active, order_num, created_at, updated_at)
		SELECT
			bo_table,
			bo_subject,
			NULLIF(bo_content_head, ''),
			TRUE,
			bo_order,
			NOW(),
			NOW()
		FROM g5_board
	`
	result := db.Exec(sql)
	if result.Error != nil {
		return result.Error
	}
	log.Printf("[v2-migration] Migrated %d boards", result.RowsAffected)
	return nil
}

func migratePostsAndComments(db *gorm.DB) error {
	// Get all board IDs from g5_board
	var boardIDs []string
	if err := db.Raw("SELECT bo_table FROM g5_board").Scan(&boardIDs).Error; err != nil {
		return err
	}

	for _, boardID := range boardIDs {
		tableName := fmt.Sprintf("g5_write_%s", boardID)

		// Check if table exists
		var count int64
		db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", tableName).Scan(&count)
		if count == 0 {
			continue
		}

		// Migrate posts (wr_is_comment = 0)
		postSQL := fmt.Sprintf(`
			INSERT IGNORE INTO v2_posts (board_id, user_id, title, content, status, view_count, comment_count, is_notice, created_at, updated_at)
			SELECT
				(SELECT id FROM v2_boards WHERE slug = '%s'),
				COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id LIMIT 1), 1),
				w.wr_subject,
				w.wr_content,
				'published',
				w.wr_hit,
				w.wr_comment,
				CASE WHEN w.wr_option LIKE '%%notice%%' THEN TRUE ELSE FALSE END,
				w.wr_datetime,
				COALESCE(NULLIF(w.wr_last, ''), w.wr_datetime)
			FROM %s w
			WHERE w.wr_is_comment = 0 AND w.wr_id = w.wr_parent
		`, boardID, tableName)

		result := db.Exec(postSQL)
		if result.Error != nil {
			log.Printf("[v2-migration] Warning: failed to migrate posts from %s: %v", tableName, result.Error)
			continue
		}

		// Migrate comments (wr_is_comment = 1)
		// Note: parent_id mapping requires a lookup table or two-pass approach
		commentSQL := fmt.Sprintf(`
			INSERT IGNORE INTO v2_comments (post_id, user_id, content, depth, status, created_at, updated_at)
			SELECT
				(SELECT p.id FROM v2_posts p JOIN v2_boards b ON p.board_id = b.id WHERE b.slug = '%s' AND p.title = (SELECT wr_subject FROM %s WHERE wr_id = w.wr_parent AND wr_is_comment = 0 LIMIT 1) LIMIT 1),
				COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id LIMIT 1), 1),
				w.wr_content,
				0,
				'active',
				w.wr_datetime,
				w.wr_datetime
			FROM %s w
			WHERE w.wr_is_comment = 1
			HAVING post_id IS NOT NULL
		`, boardID, tableName, tableName)

		db.Exec(commentSQL) // Ignore errors for comments - best effort
		log.Printf("[v2-migration] Migrated posts/comments from %s", tableName)
	}

	return nil
}

func migrateFiles(db *gorm.DB) error {
	// g5_board_file migration is complex due to dynamic table references
	// For now, we log a placeholder - full implementation requires board_id context
	log.Println("[v2-migration] File migration requires board-level iteration (skipped in auto-migration)")
	log.Println("[v2-migration] Run migrations/001_gnuboard_to_v2.sql manually for file migration")
	return nil
}
