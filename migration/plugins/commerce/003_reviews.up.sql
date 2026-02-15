-- Commerce Plugin Review Migration
-- Version: 3
-- Description: 상품 리뷰 및 평점 시스템

-- ============================================
-- 1. 리뷰 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_reviews (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    product_id      BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,
    order_item_id   BIGINT UNSIGNED NOT NULL COMMENT '구매 검증용',

    -- 리뷰 내용
    rating          TINYINT UNSIGNED NOT NULL COMMENT '별점 (1-5)',
    title           VARCHAR(200) COMMENT '리뷰 제목',
    content         TEXT NOT NULL COMMENT '리뷰 내용',

    -- 이미지
    images          JSON COMMENT '리뷰 이미지 URL 배열',

    -- 상태
    status          ENUM('pending', 'approved', 'rejected', 'hidden') DEFAULT 'pending',
    is_verified     BOOLEAN DEFAULT TRUE COMMENT '구매 검증된 리뷰',

    -- 도움됨
    helpful_count   INT UNSIGNED DEFAULT 0 COMMENT '도움됨 카운트',

    -- 판매자 답글
    seller_reply    TEXT COMMENT '판매자 답글',
    replied_at      TIMESTAMP NULL COMMENT '답글 작성 시각',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP NULL COMMENT '소프트 삭제',

    -- 외래키
    CONSTRAINT fk_reviews_product
        FOREIGN KEY (product_id) REFERENCES commerce_products(id) ON DELETE CASCADE,
    CONSTRAINT fk_reviews_order_item
        FOREIGN KEY (order_item_id) REFERENCES commerce_order_items(id) ON DELETE CASCADE,

    -- 인덱스
    INDEX idx_product (product_id),
    INDEX idx_user (user_id),
    INDEX idx_rating (rating),
    INDEX idx_status (status),
    INDEX idx_created (created_at),

    -- 사용자당 주문 아이템별 1개의 리뷰만 허용
    UNIQUE KEY idx_user_order_item (user_id, order_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 2. 리뷰 도움됨 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_review_helpfuls (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    review_id       BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_review_helpfuls_review
        FOREIGN KEY (review_id) REFERENCES commerce_reviews(id) ON DELETE CASCADE,

    -- 사용자당 리뷰별 1회만
    UNIQUE KEY idx_review_user (review_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 3. 상품 테이블에 리뷰 통계 컬럼 확인 (이미 있음)
-- rating_avg, rating_count 컬럼은 001_init.up.sql에서 생성됨
-- ============================================
