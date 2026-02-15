-- 직접홍보 게시판 플러그인 초기 스키마
-- Promotion Plugin Initial Schema

-- 광고주 테이블
CREATE TABLE IF NOT EXISTS promotion_advertisers (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    member_id   VARCHAR(50) NOT NULL COMMENT '회원 ID (g5_member.mb_id)',
    name        VARCHAR(100) NOT NULL COMMENT '광고주명/업체명',
    post_count  INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '표시할 글 개수',
    start_date  DATE DEFAULT NULL COMMENT '계약 시작일',
    end_date    DATE DEFAULT NULL COMMENT '계약 종료일',
    is_pinned   BOOLEAN NOT NULL DEFAULT FALSE COMMENT '상단 고정 여부',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    memo        TEXT DEFAULT NULL COMMENT '관리자 메모',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY uk_member_id (member_id),
    KEY idx_is_active (is_active),
    KEY idx_is_active_dates (is_active, start_date, end_date),
    KEY idx_is_pinned (is_pinned)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 직홍게 글 테이블
CREATE TABLE IF NOT EXISTS promotion_posts (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    advertiser_id   BIGINT UNSIGNED NOT NULL COMMENT '광고주 ID',
    title           VARCHAR(255) NOT NULL COMMENT '글 제목',
    content         TEXT DEFAULT NULL COMMENT '글 내용',
    link_url        VARCHAR(500) DEFAULT NULL COMMENT '외부 링크 URL',
    image_url       VARCHAR(500) DEFAULT NULL COMMENT '대표 이미지 URL',
    views           INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '조회수',
    likes           INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '좋아요 수',
    comment_count   INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '댓글 수',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE COMMENT '활성화 여부',
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    KEY idx_advertiser_id (advertiser_id),
    KEY idx_created_at (created_at),
    KEY idx_is_active (is_active),

    CONSTRAINT fk_promotion_posts_advertiser
        FOREIGN KEY (advertiser_id)
        REFERENCES promotion_advertisers (id)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
