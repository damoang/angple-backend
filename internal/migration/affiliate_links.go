package migration

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

func CreateAffiliateLinksTable(db *gorm.DB) error {
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'g5_affiliate_links'
	`).Scan(&count)

	if count > 0 {
		log.Printf("[Migration] g5_affiliate_links table already exists, skipping")
		return nil
	}

	sql := `
		CREATE TABLE g5_affiliate_links (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			board_slug VARCHAR(64) NOT NULL,
			post_id INT NOT NULL DEFAULT 0,
			comment_id INT NOT NULL DEFAULT 0,
			entity_type VARCHAR(20) NOT NULL,
			link_index INT NOT NULL DEFAULT 0,
			source_url TEXT NOT NULL,
			normalized_url TEXT NOT NULL,
			merchant_domain VARCHAR(255) NOT NULL DEFAULT '',
			platform VARCHAR(50) NOT NULL DEFAULT '',
			affiliate_url TEXT NULL,
			redirect_id VARCHAR(32) NOT NULL DEFAULT '',
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			reason_code VARCHAR(50) NOT NULL DEFAULT '',
			attempt_count INT NOT NULL DEFAULT 0,
			last_error TEXT NULL,
			last_processed_at DATETIME NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uq_affiliate_entity_link (board_slug, post_id, comment_id, entity_type, link_index),
			INDEX idx_affiliate_redirect_id (redirect_id),
			INDEX idx_affiliate_post (board_slug, post_id, entity_type),
			INDEX idx_affiliate_comment (board_slug, comment_id, entity_type),
			INDEX idx_affiliate_status_updated (status, updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create g5_affiliate_links table: %w", err)
	}

	log.Printf("[Migration] Created g5_affiliate_links table")
	return nil
}
