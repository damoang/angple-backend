package migration

import (
	"gorm.io/gorm"
)

// AddRestrictionScopeColumn adds restriction_scope column to g5_da_member_discipline
// for fine-grained restriction control (all, write, comment, reaction).
func AddRestrictionScopeColumn(db *gorm.DB) error {
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'g5_da_member_discipline'
		  AND COLUMN_NAME = 'restriction_scope'
	`).Scan(&count)

	if count > 0 {
		return nil
	}

	return db.Exec(`
		ALTER TABLE g5_da_member_discipline
		ADD COLUMN restriction_scope VARCHAR(20) NOT NULL DEFAULT 'all'
		COMMENT 'Restriction scope: all, write, comment, reaction'
	`).Error
}
