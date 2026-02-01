-- 이모티콘 플러그인 초기 스키마
-- Emoticon Plugin Initial Schema

-- 이모티콘 팩 (카테고리) 테이블
CREATE TABLE IF NOT EXISTS emoticon_packs (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    slug          VARCHAR(50) NOT NULL COMMENT '팩 슬러그 (URL용)',
    name          VARCHAR(100) NOT NULL COMMENT '팩 표시명',
    default_width INT NOT NULL DEFAULT 50 COMMENT '기본 너비 (px)',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    sort_order    INT NOT NULL DEFAULT 0 COMMENT '정렬 순서 (낮을수록 먼저)',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY uk_slug (slug),
    KEY idx_is_active_sort (is_active, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 개별 이모티콘 테이블
CREATE TABLE IF NOT EXISTS emoticon_items (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    pack_id     BIGINT UNSIGNED NOT NULL COMMENT '팩 ID',
    filename    VARCHAR(255) NOT NULL COMMENT '파일명 (고유)',
    thumb_path  VARCHAR(255) DEFAULT NULL COMMENT '썸네일 경로',
    mime_type   VARCHAR(50) DEFAULT NULL COMMENT 'MIME 타입',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY uk_filename (filename),
    KEY idx_pack_id (pack_id),
    KEY idx_pack_active (pack_id, is_active),

    CONSTRAINT fk_emoticon_items_pack
        FOREIGN KEY (pack_id)
        REFERENCES emoticon_packs (id)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
