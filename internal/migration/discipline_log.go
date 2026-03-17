package migration

import (
	"gorm.io/gorm"
)

// AddDisciplineLogPenaltyMbIDColumn adds a generated column and index for
// penalty_mb_id extracted from JSON wr_content, replacing slow JSON_EXTRACT
// full-table scans with an indexed column lookup.
func AddDisciplineLogPenaltyMbIDColumn(db *gorm.DB) error {
	// Check if column already exists
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'g5_write_disciplinelog'
		  AND COLUMN_NAME = 'penalty_mb_id'
	`).Scan(&count)

	if count > 0 {
		return nil // already migrated
	}

	// Add generated column (STORED so it can be indexed)
	if err := db.Exec(`
		ALTER TABLE g5_write_disciplinelog
		ADD COLUMN penalty_mb_id VARCHAR(100)
		GENERATED ALWAYS AS (
			CASE WHEN JSON_VALID(wr_content) THEN JSON_UNQUOTE(JSON_EXTRACT(wr_content, '$.penalty_mb_id')) ELSE NULL END
		) STORED
	`).Error; err != nil {
		return err
	}

	// Add index for fast lookups
	if err := db.Exec(`
		ALTER TABLE g5_write_disciplinelog
		ADD INDEX idx_penalty_mb_id (penalty_mb_id)
	`).Error; err != nil {
		return err
	}

	return nil
}
