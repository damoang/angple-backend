-- ============================================================
-- 직접홍보 게시판 & 배너 시스템 스키마
-- Promotion Board & Banner System Schema
-- ============================================================

SET NAMES utf8mb4;
SET CHARACTER SET utf8mb4;

-- ============================================================
-- 광고주 테이블 (Advertisers Table)
-- ============================================================
CREATE TABLE IF NOT EXISTS `advertisers` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '광고주 ID',
    `member_id` VARCHAR(50) NOT NULL COMMENT '회원 ID (g5_member.mb_id 참조)',
    `name` VARCHAR(100) NOT NULL COMMENT '광고주명/업체명',
    `post_count` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '표시할 글 개수',
    `start_date` DATE DEFAULT NULL COMMENT '계약 시작일',
    `end_date` DATE DEFAULT NULL COMMENT '계약 종료일',
    `is_pinned` BOOLEAN NOT NULL DEFAULT FALSE COMMENT '상단 고정 여부',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `memo` TEXT DEFAULT NULL COMMENT '관리자 메모',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_member_id` (`member_id`),
    KEY `idx_is_active` (`is_active`),
    KEY `idx_is_active_dates` (`is_active`, `start_date`, `end_date`),
    KEY `idx_is_pinned` (`is_pinned`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='광고주 테이블';

-- ============================================================
-- 직홍게 글 테이블 (Promotion Posts Table)
-- ============================================================
CREATE TABLE IF NOT EXISTS `promotion_posts` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '글 ID',
    `advertiser_id` BIGINT UNSIGNED NOT NULL COMMENT '광고주 ID',
    `title` VARCHAR(255) NOT NULL COMMENT '글 제목',
    `content` TEXT DEFAULT NULL COMMENT '글 내용',
    `link_url` VARCHAR(500) DEFAULT NULL COMMENT '외부 링크 URL',
    `image_url` VARCHAR(500) DEFAULT NULL COMMENT '대표 이미지 URL',
    `views` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '조회수',
    `likes` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '좋아요 수',
    `comment_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '댓글 수',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    KEY `idx_advertiser_id` (`advertiser_id`),
    KEY `idx_created_at` (`created_at`),
    KEY `idx_is_active` (`is_active`),

    CONSTRAINT `fk_promotion_posts_advertiser`
        FOREIGN KEY (`advertiser_id`)
        REFERENCES `advertisers` (`id`)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='직홍게 글 테이블';

-- ============================================================
-- 배너 테이블 (Banners Table)
-- ============================================================
CREATE TABLE IF NOT EXISTS `banners` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '배너 ID',
    `title` VARCHAR(100) NOT NULL COMMENT '배너 제목 (관리용)',
    `image_url` VARCHAR(500) DEFAULT NULL COMMENT '배너 이미지 URL',
    `link_url` VARCHAR(500) DEFAULT NULL COMMENT '클릭 시 이동 URL',
    `position` ENUM('header', 'sidebar', 'content', 'footer') NOT NULL DEFAULT 'sidebar' COMMENT '배너 위치',
    `start_date` DATE DEFAULT NULL COMMENT '노출 시작일',
    `end_date` DATE DEFAULT NULL COMMENT '노출 종료일',
    `priority` INT NOT NULL DEFAULT 0 COMMENT '우선순위 (높을수록 먼저 표시)',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `click_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '클릭 수',
    `view_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '노출 수',
    `alt_text` VARCHAR(255) DEFAULT NULL COMMENT '이미지 대체 텍스트',
    `target` ENUM('_self', '_blank') NOT NULL DEFAULT '_blank' COMMENT '링크 타겟',
    `memo` TEXT DEFAULT NULL COMMENT '관리자 메모',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    KEY `idx_position` (`position`),
    KEY `idx_is_active_dates` (`is_active`, `start_date`, `end_date`),
    KEY `idx_priority` (`priority`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='배너 테이블';

-- ============================================================
-- 배너 클릭 로그 테이블 (Banner Click Logs Table) - 분석용
-- ============================================================
CREATE TABLE IF NOT EXISTS `banner_click_logs` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '로그 ID',
    `banner_id` BIGINT UNSIGNED NOT NULL COMMENT '배너 ID',
    `member_id` VARCHAR(50) DEFAULT NULL COMMENT '회원 ID (비회원은 NULL)',
    `ip_address` VARCHAR(45) DEFAULT NULL COMMENT 'IP 주소',
    `user_agent` VARCHAR(500) DEFAULT NULL COMMENT 'User Agent',
    `referer` VARCHAR(500) DEFAULT NULL COMMENT 'Referer URL',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '클릭 일시',

    PRIMARY KEY (`id`),
    KEY `idx_banner_id` (`banner_id`),
    KEY `idx_created_at` (`created_at`),

    CONSTRAINT `fk_banner_click_logs_banner`
        FOREIGN KEY (`banner_id`)
        REFERENCES `banners` (`id`)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='배너 클릭 로그 테이블';
