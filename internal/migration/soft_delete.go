package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// AddSoftDeleteColumnsToAllBoards adds wr_deleted_at and wr_deleted_by columns
// to all g5_write_* tables for soft delete support
func AddSoftDeleteColumnsToAllBoards(db *gorm.DB) error {
	// Get all board IDs from g5_board
	var boardIDs []string
	if err := db.Table("g5_board").Pluck("bo_table", &boardIDs).Error; err != nil {
		return fmt.Errorf("failed to get board IDs: %w", err)
	}

	log.Printf("[Migration] Found %d boards to migrate", len(boardIDs))

	successCount := 0
	for _, boardID := range boardIDs {
		if err := AddSoftDeleteColumns(db, boardID); err != nil {
			log.Printf("[Migration] Warning: Failed to add soft delete columns to g5_write_%s: %v", boardID, err)
			continue
		}
		successCount++
	}

	log.Printf("[Migration] Successfully added soft delete columns to %d/%d boards", successCount, len(boardIDs))
	return nil
}

// AddSoftDeleteColumns adds wr_deleted_at and wr_deleted_by columns to a specific board table
func AddSoftDeleteColumns(db *gorm.DB, boardID string) error {
	table := fmt.Sprintf("g5_write_%s", boardID)

	// Check if columns already exist
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = ?
		AND COLUMN_NAME = 'wr_deleted_at'
	`, table).Scan(&count)

	if count > 0 {
		// Column already exists, skip
		return nil
	}

	// Add columns
	sql := fmt.Sprintf(`
		ALTER TABLE %s
		ADD COLUMN wr_deleted_at DATETIME NULL DEFAULT NULL,
		ADD COLUMN wr_deleted_by VARCHAR(20) NULL DEFAULT NULL,
		ADD INDEX idx_wr_deleted_at (wr_deleted_at)
	`, table)

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to alter table %s: %w", table, err)
	}

	return nil
}

// CreateWriteRevisionsTable creates the g5_write_revisions table for post history tracking
func CreateWriteRevisionsTable(db *gorm.DB) error {
	// Check if table already exists
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'g5_write_revisions'
	`).Scan(&count)

	if count > 0 {
		log.Printf("[Migration] g5_write_revisions table already exists, skipping")
		return nil
	}

	sql := `
		CREATE TABLE g5_write_revisions (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			board_id VARCHAR(20) NOT NULL,
			wr_id INT NOT NULL,
			version INT NOT NULL DEFAULT 1,
			change_type VARCHAR(20) NOT NULL COMMENT 'create, update, soft_delete, restore, permanent_delete',
			title VARCHAR(255) NULL,
			content MEDIUMTEXT NULL,
			edited_by VARCHAR(20) NOT NULL,
			edited_by_name VARCHAR(100) NULL,
			edited_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			metadata JSON NULL COMMENT 'Additional change metadata',
			INDEX idx_board_wr (board_id, wr_id),
			INDEX idx_edited_at (edited_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create g5_write_revisions table: %w", err)
	}

	log.Printf("[Migration] Created g5_write_revisions table")
	return nil
}
