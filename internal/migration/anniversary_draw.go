package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

func CreateAnniversaryDrawEntriesTable(db *gorm.DB) error {
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'event_anniversary_draw_entries'
	`).Scan(&count)

	if count > 0 {
		log.Printf("[Migration] event_anniversary_draw_entries table already exists, skipping")
		return nil
	}

	sql := `
		CREATE TABLE event_anniversary_draw_entries (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			event_code VARCHAR(64) NOT NULL,
			mb_id VARCHAR(64) NOT NULL,
			draw_result VARCHAR(120) NOT NULL,
			point_amount INT NOT NULL,
			granted_at DATETIME NULL,
			point_po_id INT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_event_member (event_code, mb_id),
			KEY idx_event_granted (event_code, granted_at, id),
			KEY idx_point_po_id (point_po_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create event_anniversary_draw_entries table: %w", err)
	}

	log.Printf("[Migration] Created event_anniversary_draw_entries table")
	return nil
}
