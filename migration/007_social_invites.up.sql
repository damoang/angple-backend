-- One-time social login reconnect invites for restored accounts.
CREATE TABLE IF NOT EXISTS social_invites (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    token VARCHAR(32) NOT NULL,
    target_mb_id VARCHAR(20) NOT NULL,
    target_mb_nick VARCHAR(50) NOT NULL,
    created_by VARCHAR(20) NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at DATETIME NULL,
    used_by VARCHAR(20) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_social_invites_token (token),
    KEY idx_social_invites_target_mb_id (target_mb_id),
    KEY idx_social_invites_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
