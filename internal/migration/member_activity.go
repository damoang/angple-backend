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
	if err := db.Exec(statsDDL).Error; err != nil {
		return err
	}

	// 프로필 활동피드 조회(board_id + write_id IN + activity_type + is_deleted)가
	// board_id 에 매칭 인덱스가 없어 풀스캔(EXPLAIN type=ALL, ~800만 행)이던 문제 해소.
	// uk_table_write_type 는 write_table(테이블명) 기준이라 board_id(슬러그)엔 무효.
	// 등가조건 3(board_id, activity_type, is_deleted) + IN범위 1(write_id) 복합 인덱스.
	// 운영 대용량 테이블 대비 온라인 DDL(LOCK=NONE) 명시. 멱등(존재 시 스킵) —
	// 운영은 저트래픽 윈도우에 별도 온라인 DDL 로 선반영하고, 이 마이그레이션은 기록·
	// 신규 환경 재현용이다.
	if err := addIndexIfMissing(db, "member_activity_feed", "idx_board_type_del_write",
		"ALTER TABLE member_activity_feed ADD INDEX idx_board_type_del_write "+
			"(board_id, activity_type, is_deleted, write_id), ALGORITHM=INPLACE, LOCK=NONE"); err != nil {
		return err
	}
	return nil
}

// addIndexIfMissing 은 인덱스가 없을 때만 주어진 DDL 을 실행한다(멱등).
func addIndexIfMissing(db *gorm.DB, tableName, indexName, ddl string) error {
	exists, err := tableIndexExists(db, tableName, indexName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return db.Exec(ddl).Error
}

// tableIndexExists 는 테이블에 해당 인덱스가 존재하는지 반환한다.
func tableIndexExists(db *gorm.DB, tableName, indexName string) (bool, error) {
	var count int64
	err := db.Raw(`
		SELECT COUNT(*)
		  FROM information_schema.statistics
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
		   AND index_name = ?`,
		tableName, indexName,
	).Scan(&count).Error
	return count > 0, err
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
