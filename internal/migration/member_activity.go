package migration

import "gorm.io/gorm"

// CreateMemberActivityTables creates the read-model tables used by member activity queries.
// These tables are append/update targets for denormalized activity reads and must exist
// before handlers rely on member_activity_feed as the primary read path.
func CreateMemberActivityTables(db *gorm.DB) error {
	feedDDL := `
CREATE TABLE IF NOT EXISTS member_activity_feed (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  member_id VARCHAR(64) NOT NULL,
  board_id VARCHAR(64) NOT NULL,
  write_table VARCHAR(128) NOT NULL,
  write_id BIGINT UNSIGNED NOT NULL,
  parent_write_id BIGINT UNSIGNED DEFAULT NULL,
  activity_type TINYINT UNSIGNED NOT NULL COMMENT '1=post, 2=comment',
  is_public TINYINT(1) NOT NULL DEFAULT 1,
  is_deleted TINYINT(1) NOT NULL DEFAULT 0,
  title VARCHAR(255) DEFAULT NULL,
  content_preview VARCHAR(255) DEFAULT NULL,
  parent_title VARCHAR(255) DEFAULT NULL,
  author_name VARCHAR(255) DEFAULT NULL,
  wr_option VARCHAR(255) DEFAULT NULL,
  view_count INT NOT NULL DEFAULT 0,
  like_count INT NOT NULL DEFAULT 0,
  dislike_count INT NOT NULL DEFAULT 0,
  comment_count INT NOT NULL DEFAULT 0,
  has_file TINYINT(1) NOT NULL DEFAULT 0,
  source_created_at DATETIME NOT NULL,
  source_updated_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_table_write_type (write_table, write_id, activity_type),
  KEY idx_member_type_deleted_created (member_id, activity_type, is_deleted, source_created_at DESC, id DESC),
  KEY idx_member_public_deleted_created (member_id, is_public, is_deleted, source_created_at DESC, id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

	statsDDL := `
CREATE TABLE IF NOT EXISTS member_activity_stats (
  member_id VARCHAR(64) NOT NULL,
  board_id VARCHAR(64) NOT NULL DEFAULT '',
  post_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  comment_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  public_post_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  public_comment_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (member_id, board_id),
  KEY idx_member (member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

	if err := db.Exec(feedDDL).Error; err != nil {
		return err
	}
	alterDDLs := map[string]string{
		"view_count":    "ALTER TABLE member_activity_feed ADD COLUMN view_count INT NOT NULL DEFAULT 0",
		"like_count":    "ALTER TABLE member_activity_feed ADD COLUMN like_count INT NOT NULL DEFAULT 0",
		"dislike_count": "ALTER TABLE member_activity_feed ADD COLUMN dislike_count INT NOT NULL DEFAULT 0",
		"comment_count": "ALTER TABLE member_activity_feed ADD COLUMN comment_count INT NOT NULL DEFAULT 0",
		"has_file":      "ALTER TABLE member_activity_feed ADD COLUMN has_file TINYINT(1) NOT NULL DEFAULT 0",
	}
	for columnName, ddl := range alterDDLs {
		exists, err := tableColumnExists(db, "member_activity_feed", columnName)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := db.Exec(ddl).Error; err != nil {
			return err
		}
	}
	return db.Exec(statsDDL).Error
}

func tableColumnExists(db *gorm.DB, tableName, columnName string) (bool, error) {
	var count int64
	err := db.Raw(`
		SELECT COUNT(*)
		  FROM information_schema.columns
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
		   AND column_name = ?`,
		tableName, columnName,
	).Scan(&count).Error
	return count > 0, err
}
