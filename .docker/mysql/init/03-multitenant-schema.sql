-- ========================================
-- Angple SaaS 멀티 테넌트 스키마
-- ========================================
-- 작성일: 2026-01-07
-- 목적: Subdomain별 사이트 격리 및 하이브리드 DB 전략 구현
-- 전략:
--   - Free 플랜: site_id 컬럼으로 논리적 격리
--   - Pro/Business 플랜: 별도 스키마(database) 물리적 격리
--   - Enterprise 플랜: 독립 DB 인스턴스 (추후 구현)
-- ========================================

-- ========================================
-- 1. 사이트 메타데이터 테이블
-- ========================================

CREATE TABLE IF NOT EXISTS `sites` (
    `id` VARCHAR(36) PRIMARY KEY COMMENT '사이트 고유 ID (UUID)',
    `subdomain` VARCHAR(50) NOT NULL UNIQUE COMMENT '서브도메인 (예: mycompany)',
    `site_name` VARCHAR(100) NOT NULL COMMENT '사이트 표시 이름 (예: 우리회사 커뮤니티)',
    `owner_email` VARCHAR(255) NOT NULL COMMENT '사이트 소유자 이메일',
    `plan` ENUM('free', 'pro', 'business', 'enterprise') DEFAULT 'free' COMMENT '구독 플랜',
    `db_strategy` ENUM('shared', 'schema', 'dedicated') DEFAULT 'shared' COMMENT 'DB 격리 전략',
    `db_schema_name` VARCHAR(50) DEFAULT NULL COMMENT 'Pro+ 플랜의 전용 스키마명',
    `db_host` VARCHAR(255) DEFAULT NULL COMMENT 'Enterprise 플랜의 독립 DB 호스트',
    `db_port` INT DEFAULT 3306 COMMENT 'Enterprise 플랜의 DB 포트',
    `active` BOOLEAN DEFAULT TRUE COMMENT '사이트 활성화 여부',
    `suspended` BOOLEAN DEFAULT FALSE COMMENT '정지 여부 (결제 실패 등)',
    `trial_ends_at` TIMESTAMP NULL COMMENT '무료 체험 종료일',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '생성일',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정일',
    INDEX `idx_subdomain` (`subdomain`),
    INDEX `idx_owner_email` (`owner_email`),
    INDEX `idx_active` (`active`),
    INDEX `idx_plan` (`plan`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='멀티 테넌트 사이트 정보';

-- ========================================
-- 2. 사이트별 사용자 권한 테이블
-- ========================================

CREATE TABLE IF NOT EXISTS `site_users` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '권한 레코드 ID',
    `site_id` VARCHAR(36) NOT NULL COMMENT '사이트 ID (FK)',
    `user_id` VARCHAR(50) NOT NULL COMMENT '그누보드 회원 ID (g5_member.mb_id)',
    `role` ENUM('owner', 'admin', 'editor', 'viewer') DEFAULT 'viewer' COMMENT '역할',
    `invited_by` VARCHAR(50) DEFAULT NULL COMMENT '초대자 user_id',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '권한 부여일',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정일',
    FOREIGN KEY (`site_id`) REFERENCES `sites`(`id`) ON DELETE CASCADE,
    UNIQUE KEY `unique_site_user` (`site_id`, `user_id`),
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_role` (`role`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='사이트별 사용자 권한 매핑';

-- ========================================
-- 3. 사이트별 설정 테이블
-- ========================================

CREATE TABLE IF NOT EXISTS `site_settings` (
    `site_id` VARCHAR(36) PRIMARY KEY COMMENT '사이트 ID (FK)',
    `active_theme` VARCHAR(100) DEFAULT 'damoang-official' COMMENT '활성 테마 ID',
    `logo_url` VARCHAR(500) DEFAULT NULL COMMENT '로고 이미지 URL',
    `favicon_url` VARCHAR(500) DEFAULT NULL COMMENT '파비콘 URL',
    `primary_color` VARCHAR(7) DEFAULT '#3b82f6' COMMENT '메인 색상 (HEX)',
    `secondary_color` VARCHAR(7) DEFAULT '#8b5cf6' COMMENT '보조 색상 (HEX)',
    `site_description` TEXT DEFAULT NULL COMMENT '사이트 설명 (SEO)',
    `site_keywords` TEXT DEFAULT NULL COMMENT 'SEO 키워드',
    `google_analytics_id` VARCHAR(50) DEFAULT NULL COMMENT 'Google Analytics ID',
    `custom_domain` VARCHAR(255) DEFAULT NULL COMMENT '커스텀 도메인 (Pro+)',
    `ssl_enabled` BOOLEAN DEFAULT TRUE COMMENT 'SSL 활성화 여부',
    `settings_json` JSON DEFAULT NULL COMMENT '기타 설정 (JSON)',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '생성일',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정일',
    FOREIGN KEY (`site_id`) REFERENCES `sites`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='사이트별 상세 설정';

-- ========================================
-- 4. 사이트 리소스 사용량 추적 테이블
-- ========================================

CREATE TABLE IF NOT EXISTS `site_usage` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '사용량 레코드 ID',
    `site_id` VARCHAR(36) NOT NULL COMMENT '사이트 ID (FK)',
    `date` DATE NOT NULL COMMENT '날짜',
    `unique_visitors` INT DEFAULT 0 COMMENT '순 방문자 수',
    `page_views` INT DEFAULT 0 COMMENT '페이지뷰',
    `posts_created` INT DEFAULT 0 COMMENT '생성된 게시글 수',
    `comments_created` INT DEFAULT 0 COMMENT '생성된 댓글 수',
    `storage_used_mb` DECIMAL(10,2) DEFAULT 0 COMMENT '사용 스토리지 (MB)',
    `bandwidth_used_mb` DECIMAL(10,2) DEFAULT 0 COMMENT '사용 대역폭 (MB)',
    `api_calls` INT DEFAULT 0 COMMENT 'API 호출 수',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '생성일',
    FOREIGN KEY (`site_id`) REFERENCES `sites`(`id`) ON DELETE CASCADE,
    UNIQUE KEY `unique_site_date` (`site_id`, `date`),
    INDEX `idx_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='사이트별 리소스 사용량 추적';

-- ========================================
-- 5. 기존 테이블에 site_id 컬럼 추가 (Free 플랜용)
-- ========================================

-- 회원 테이블
ALTER TABLE `g5_member`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID (멀티 테넌트)',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- 게시판 설정 테이블
ALTER TABLE `g5_board`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- 메뉴 테이블
ALTER TABLE `menus`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- 주의: 동적 게시판 테이블(g5_write_*)은 사이트 생성 시 자동으로 site_id 컬럼 추가
-- 프로비저닝 스크립트에서 처리:
-- ALTER TABLE g5_write_free ADD COLUMN site_id VARCHAR(36) DEFAULT 'default';
-- ALTER TABLE g5_write_qna ADD COLUMN site_id VARCHAR(36) DEFAULT 'default';

-- ========================================
-- 6. 기본 사이트 데이터 삽입 (개발/테스트용)
-- ========================================

-- 기본 사이트 (다모앙)
INSERT INTO `sites` (`id`, `subdomain`, `site_name`, `owner_email`, `plan`, `db_strategy`, `active`)
VALUES
    ('default', 'www', '다모앙 커뮤니티', 'admin@damoang.net', 'enterprise', 'shared', TRUE)
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- 기본 사이트 설정
INSERT INTO `site_settings` (`site_id`, `active_theme`, `site_description`)
VALUES
    ('default', 'damoang-official', '다모앙은 개발자 커뮤니티입니다.')
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- 기본 사이트 관리자 권한
INSERT INTO `site_users` (`site_id`, `user_id`, `role`)
VALUES
    ('default', 'admin', 'owner')
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- ========================================
-- 7. 데모 사이트 데이터 (개발 환경용)
-- ========================================

INSERT INTO `sites` (`id`, `subdomain`, `site_name`, `owner_email`, `plan`, `db_strategy`, `active`)
VALUES
    ('demo-free-001', 'demo-free', 'Free Plan Demo', 'demo-free@angple.com', 'free', 'shared', TRUE),
    ('demo-pro-001', 'demo-pro', 'Pro Plan Demo', 'demo-pro@angple.com', 'pro', 'schema', TRUE),
    ('demo-biz-001', 'demo-biz', 'Business Plan Demo', 'demo-biz@angple.com', 'business', 'schema', TRUE)
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- 데모 사이트 설정
INSERT INTO `site_settings` (`site_id`, `active_theme`, `primary_color`, `site_description`)
VALUES
    ('demo-free-001', 'damoang-classic', '#3b82f6', 'Free 플랜 데모 사이트'),
    ('demo-pro-001', 'modern-dark', '#8b5cf6', 'Pro 플랜 데모 사이트'),
    ('demo-biz-001', 'corporate-landing', '#06b6d4', 'Business 플랜 데모 사이트')
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- ========================================
-- 8. 헬퍼 뷰 (조회 편의성)
-- ========================================

-- 활성 사이트 목록
CREATE OR REPLACE VIEW `v_active_sites` AS
SELECT
    s.id,
    s.subdomain,
    s.site_name,
    s.owner_email,
    s.plan,
    s.db_strategy,
    st.active_theme,
    st.logo_url,
    st.primary_color,
    s.created_at
FROM `sites` s
LEFT JOIN `site_settings` st ON s.id = st.site_id
WHERE s.active = TRUE AND s.suspended = FALSE;

-- 사이트별 사용자 수
CREATE OR REPLACE VIEW `v_site_user_counts` AS
SELECT
    s.id AS site_id,
    s.subdomain,
    s.site_name,
    COUNT(su.user_id) AS total_users,
    SUM(CASE WHEN su.role = 'owner' THEN 1 ELSE 0 END) AS owners,
    SUM(CASE WHEN su.role = 'admin' THEN 1 ELSE 0 END) AS admins,
    SUM(CASE WHEN su.role = 'editor' THEN 1 ELSE 0 END) AS editors,
    SUM(CASE WHEN su.role = 'viewer' THEN 1 ELSE 0 END) AS viewers
FROM `sites` s
LEFT JOIN `site_users` su ON s.id = su.site_id
GROUP BY s.id, s.subdomain, s.site_name;

-- ========================================
-- 완료!
-- ========================================

-- 마이그레이션 성공 로그
SELECT
    '✅ 멀티 테넌트 스키마 마이그레이션 완료!' AS status,
    (SELECT COUNT(*) FROM sites) AS total_sites,
    (SELECT COUNT(*) FROM site_users) AS total_user_permissions,
    (SELECT COUNT(*) FROM site_settings) AS total_site_settings;
