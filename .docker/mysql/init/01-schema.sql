-- Angple Database Schema
-- 차세대 다모앙 데이터베이스 스키마

SET NAMES utf8mb4;
SET CHARACTER SET utf8mb4;

-- ============================================================
-- 메뉴 테이블 (Menu Table)
-- ============================================================
CREATE TABLE IF NOT EXISTS `menus` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '메뉴 ID',
    `parent_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '부모 메뉴 ID (NULL이면 루트)',
    `title` VARCHAR(100) NOT NULL COMMENT '메뉴 제목',
    `url` VARCHAR(255) NOT NULL COMMENT '메뉴 URL',
    `icon` VARCHAR(50) DEFAULT NULL COMMENT 'Lucide 아이콘 이름',
    `shortcut` VARCHAR(10) DEFAULT NULL COMMENT '단축키 (F, Q, G 등)',
    `description` TEXT DEFAULT NULL COMMENT '메뉴 설명',
    `depth` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '메뉴 깊이 (1=루트, 2=하위, 3=하하위)',
    `order_num` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '정렬 순서',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `target` ENUM('_self', '_blank') NOT NULL DEFAULT '_self' COMMENT '링크 타겟',
    `view_level` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '보기 권한 레벨 (1-10)',
    `show_in_header` BOOLEAN NOT NULL DEFAULT FALSE COMMENT '헤더 메뉴 노출 여부',
    `show_in_sidebar` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '사이드바 메뉴 노출 여부',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    KEY `idx_parent_id` (`parent_id`),
    KEY `idx_is_active` (`is_active`),
    KEY `idx_order` (`depth`, `order_num`),
    KEY `idx_sidebar` (`show_in_sidebar`, `is_active`),
    KEY `idx_header` (`show_in_header`, `is_active`),

    CONSTRAINT `fk_menus_parent`
        FOREIGN KEY (`parent_id`)
        REFERENCES `menus` (`id`)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='메뉴 테이블';

-- ============================================================
-- 사용자 테이블 (Users Table) - 향후 확장용
-- ============================================================
CREATE TABLE IF NOT EXISTS `users` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '사용자 ID',
    `username` VARCHAR(50) NOT NULL COMMENT '사용자명',
    `email` VARCHAR(255) NOT NULL COMMENT '이메일',
    `level` TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '사용자 레벨 (1-10)',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_username` (`username`),
    UNIQUE KEY `uk_email` (`email`),
    KEY `idx_level` (`level`),
    KEY `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='사용자 테이블';

-- ============================================================
-- 게시판 테이블 (Boards Table) - 향후 확장용
-- ============================================================
CREATE TABLE IF NOT EXISTS `boards` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '게시판 ID',
    `slug` VARCHAR(50) NOT NULL COMMENT '게시판 슬러그 (URL 식별자)',
    `name` VARCHAR(100) NOT NULL COMMENT '게시판 이름',
    `description` TEXT DEFAULT NULL COMMENT '게시판 설명',
    `is_active` BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '생성 일시',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '수정 일시',

    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_slug` (`slug`),
    KEY `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='게시판 테이블';
