package migration

import "gorm.io/gorm"

// ExpandSiteLogoRecurringDateColumn allows annual recurring ranges like 03-20~04-02.
func ExpandSiteLogoRecurringDateColumn(db *gorm.DB) error {
	var needsAlter int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'site_logos'
		  AND COLUMN_NAME = 'recurring_date'
		  AND (CHARACTER_MAXIMUM_LENGTH IS NULL OR CHARACTER_MAXIMUM_LENGTH < 13)
	`).Scan(&needsAlter).Error; err != nil {
		return err
	}

	if needsAlter == 0 {
		return nil
	}

	return db.Exec(`
		ALTER TABLE site_logos
		MODIFY COLUMN recurring_date VARCHAR(13) NULL
	`).Error
}
