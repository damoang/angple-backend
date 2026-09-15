package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// CreateLuckyGrantTable creates g5_da_lucky_grant, the idempotency ledger for
// "나리야 럭키 포인트" 지급. The UNIQUE (source_table, source_id, kind) key is what
// guarantees at-most-once payout per post/comment even under retries or concurrent
// goroutines — the actual point write is bound to the ledger insert in one tx.
//
// 테이블 생성만 수행한다. 기존 g5_point / g5_member 데이터는 일절 건드리지 않는다.
// CreateWriteAfterEventsTable 과 동일하게 INFORMATION_SCHEMA 존재체크로 멱등하게 동작한다.
func CreateLuckyGrantTable(db *gorm.DB) error {
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'g5_da_lucky_grant'
	`).Scan(&count)

	if count > 0 {
		log.Printf("[Migration] g5_da_lucky_grant table already exists, skipping")
		return nil
	}

	sql := `
		CREATE TABLE g5_da_lucky_grant (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			mb_id VARCHAR(20) NOT NULL,
			source_table VARCHAR(60) NOT NULL,
			source_id VARCHAR(20) NOT NULL,
			kind VARCHAR(12) NOT NULL,
			amount INT NOT NULL,
			po_id BIGINT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uq_lucky_source (source_table, source_id, kind),
			INDEX idx_mb (mb_id),
			INDEX idx_created (created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create g5_da_lucky_grant table: %w", err)
	}

	log.Printf("[Migration] Created g5_da_lucky_grant table")
	return nil
}
