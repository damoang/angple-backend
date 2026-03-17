package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

func CreateWriteAfterEventsTable(db *gorm.DB) error {
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'g5_write_after_events'
	`).Scan(&count)

	if count > 0 {
		log.Printf("[Migration] g5_write_after_events table already exists, skipping")
		return nil
	}

	sql := `
		CREATE TABLE g5_write_after_events (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			event_type VARCHAR(40) NOT NULL,
			board_slug VARCHAR(20) NOT NULL,
			write_id INT NOT NULL,
			post_id INT NULL,
			parent_id INT NULL,
			member_id VARCHAR(255) NOT NULL DEFAULT '',
			author VARCHAR(255) NOT NULL DEFAULT '',
			subject VARCHAR(255) NOT NULL DEFAULT '',
			occurred_at DATETIME NOT NULL,
			available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			retry_count INT NOT NULL DEFAULT 0,
			last_error TEXT NULL,
			claimed_at DATETIME NULL,
			processed_at DATETIME NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX idx_status_available (status, available_at, id),
			INDEX idx_board_write (board_slug, write_id),
			INDEX idx_post_status (post_id, status),
			INDEX idx_processed_at (processed_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create g5_write_after_events table: %w", err)
	}

	log.Printf("[Migration] Created g5_write_after_events table")
	return nil
}
