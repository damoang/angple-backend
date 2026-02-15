-- Commerce Plugin Coupon Migration
-- Version: 2
-- Description: 쿠폰 및 할인 코드 시스템

-- ============================================
-- 1. 쿠폰 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_coupons (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,

    -- 쿠폰 기본 정보
    code            VARCHAR(50) NOT NULL COMMENT '쿠폰 코드',
    name            VARCHAR(100) NOT NULL COMMENT '쿠폰명',
    description     TEXT COMMENT '쿠폰 설명',

    -- 할인 타입 및 금액
    discount_type   ENUM('fixed', 'percent', 'free_shipping') NOT NULL COMMENT '할인 유형',
    discount_value  DECIMAL(12, 2) NOT NULL COMMENT '할인 값 (금액 또는 %)',
    max_discount    DECIMAL(12, 2) DEFAULT NULL COMMENT '최대 할인 금액 (percent일 때)',

    -- 사용 조건
    min_order_amount DECIMAL(12, 2) DEFAULT 0 COMMENT '최소 주문 금액',

    -- 적용 범위
    apply_to        ENUM('all', 'product', 'category', 'seller') DEFAULT 'all' COMMENT '적용 대상',
    apply_ids       JSON COMMENT '적용 대상 ID 배열',

    -- 사용 제한
    usage_limit     INT UNSIGNED DEFAULT NULL COMMENT '총 사용 제한 (NULL=무제한)',
    usage_per_user  INT UNSIGNED DEFAULT 1 COMMENT '사용자당 사용 제한',
    usage_count     INT UNSIGNED DEFAULT 0 COMMENT '현재 사용 횟수',

    -- 유효 기간
    starts_at       TIMESTAMP NULL COMMENT '유효 시작일',
    expires_at      TIMESTAMP NULL COMMENT '유효 종료일',

    -- 상태
    status          ENUM('active', 'inactive', 'expired') DEFAULT 'active',
    is_public       BOOLEAN DEFAULT FALSE COMMENT '공개 쿠폰 여부',

    -- 생성자
    created_by      BIGINT UNSIGNED COMMENT '생성자 ID (관리자)',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP NULL COMMENT '소프트 삭제',

    -- 인덱스
    UNIQUE KEY idx_code (code),
    INDEX idx_status (status),
    INDEX idx_expires (expires_at),
    INDEX idx_public (is_public, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 2. 쿠폰 사용 내역 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_coupon_usages (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    coupon_id       BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,
    order_id        BIGINT UNSIGNED NOT NULL,

    -- 할인 정보
    discount_amount DECIMAL(12, 2) NOT NULL COMMENT '적용된 할인 금액',

    -- 타임스탬프
    used_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_coupon_usages_coupon
        FOREIGN KEY (coupon_id) REFERENCES commerce_coupons(id) ON DELETE CASCADE,
    CONSTRAINT fk_coupon_usages_order
        FOREIGN KEY (order_id) REFERENCES commerce_orders(id) ON DELETE CASCADE,

    INDEX idx_coupon (coupon_id),
    INDEX idx_user (user_id),
    INDEX idx_order (order_id),
    UNIQUE KEY idx_order_coupon (order_id, coupon_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 3. 주문 테이블에 쿠폰 컬럼 추가
-- ============================================
ALTER TABLE commerce_orders
ADD COLUMN coupon_id BIGINT UNSIGNED DEFAULT NULL COMMENT '사용된 쿠폰 ID' AFTER discount,
ADD COLUMN coupon_code VARCHAR(50) DEFAULT NULL COMMENT '사용된 쿠폰 코드' AFTER coupon_id,
ADD INDEX idx_coupon (coupon_id);
