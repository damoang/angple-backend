-- 배너 광고 플러그인 초기 스키마
-- Banner Plugin Initial Schema

-- 배너 테이블
CREATE TABLE IF NOT EXISTS banner_items (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    title       VARCHAR(100) NOT NULL COMMENT '배너 제목 (관리용)',
    image_url   VARCHAR(500) DEFAULT NULL COMMENT '배너 이미지 URL',
    link_url    VARCHAR(500) DEFAULT NULL COMMENT '클릭 시 이동 URL',
    position    ENUM('header', 'sidebar', 'content', 'footer') NOT NULL DEFAULT 'sidebar' COMMENT '배너 위치',
    start_date  DATE DEFAULT NULL COMMENT '노출 시작일',
    end_date    DATE DEFAULT NULL COMMENT '노출 종료일',
    priority    INT NOT NULL DEFAULT 0 COMMENT '우선순위 (높을수록 먼저)',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    click_count INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '클릭 수',
    view_count  INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '노출 수',
    alt_text    VARCHAR(255) DEFAULT NULL COMMENT '이미지 대체 텍스트',
    target      ENUM('_self', '_blank') NOT NULL DEFAULT '_blank' COMMENT '링크 타겟',
    memo        TEXT DEFAULT NULL COMMENT '관리자 메모',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    KEY idx_position (position),
    KEY idx_is_active_dates (is_active, start_date, end_date),
    KEY idx_priority (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 배너 클릭 로그 테이블 (분석용)
CREATE TABLE IF NOT EXISTS banner_click_logs (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    banner_id   BIGINT UNSIGNED NOT NULL COMMENT '배너 ID',
    member_id   VARCHAR(50) DEFAULT NULL COMMENT '회원 ID (비회원은 NULL)',
    ip_address  VARCHAR(45) DEFAULT NULL COMMENT 'IP 주소',
    user_agent  VARCHAR(500) DEFAULT NULL COMMENT 'User Agent',
    referer     VARCHAR(500) DEFAULT NULL COMMENT 'Referer URL',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    KEY idx_banner_id (banner_id),
    KEY idx_created_at (created_at),

    CONSTRAINT fk_banner_click_logs_banner
        FOREIGN KEY (banner_id)
        REFERENCES banner_items (id)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
