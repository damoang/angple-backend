-- Commerce Plugin Initial Migration
-- Version: 1
-- Description: 커머스 플러그인 초기 테이블 생성

-- ============================================
-- 1. 상품 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_products (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    seller_id       BIGINT UNSIGNED NOT NULL COMMENT '판매자 ID (users.id)',

    -- 기본 정보
    name            VARCHAR(255) NOT NULL COMMENT '상품명',
    slug            VARCHAR(255) NOT NULL COMMENT 'URL 슬러그',
    description     TEXT COMMENT '상품 설명',
    short_desc      VARCHAR(500) COMMENT '짧은 설명',

    -- 상품 유형
    product_type    ENUM('digital', 'physical') NOT NULL DEFAULT 'digital' COMMENT '상품 유형',

    -- 가격
    price           DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT '판매가',
    original_price  DECIMAL(12, 2) COMMENT '정가 (할인 전)',
    currency        CHAR(3) NOT NULL DEFAULT 'KRW' COMMENT '통화 코드',

    -- 재고 (실물 상품용)
    stock_quantity  INT DEFAULT NULL COMMENT '재고 수량 (NULL=무제한)',
    stock_status    ENUM('in_stock', 'out_of_stock', 'preorder') DEFAULT 'in_stock',

    -- 디지털 상품 설정
    download_limit  INT DEFAULT NULL COMMENT '다운로드 제한 횟수 (NULL=무제한)',
    download_expiry INT DEFAULT NULL COMMENT '다운로드 만료일 (일수, NULL=무제한)',

    -- 상태
    status          ENUM('draft', 'published', 'archived') NOT NULL DEFAULT 'draft',
    visibility      ENUM('public', 'private', 'password') NOT NULL DEFAULT 'public',
    password        VARCHAR(100) COMMENT '비밀번호 보호 (visibility=password인 경우)',

    -- 메타
    featured_image  VARCHAR(500) COMMENT '대표 이미지 URL',
    gallery_images  JSON COMMENT '갤러리 이미지 URL 배열',
    meta_data       JSON COMMENT '추가 메타데이터',

    -- 통계
    sales_count     INT UNSIGNED DEFAULT 0 COMMENT '판매 수량',
    view_count      INT UNSIGNED DEFAULT 0 COMMENT '조회수',
    rating_avg      DECIMAL(2, 1) DEFAULT 0 COMMENT '평균 평점',
    rating_count    INT UNSIGNED DEFAULT 0 COMMENT '리뷰 수',

    -- 타임스탬프
    published_at    TIMESTAMP NULL COMMENT '발행일',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP NULL COMMENT '소프트 삭제',

    -- 인덱스
    UNIQUE KEY idx_slug (slug),
    INDEX idx_seller (seller_id),
    INDEX idx_status (status),
    INDEX idx_type (product_type),
    INDEX idx_published (published_at),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 2. 상품 파일 테이블 (디지털 상품용)
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_product_files (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    product_id      BIGINT UNSIGNED NOT NULL,

    -- 파일 정보
    file_name       VARCHAR(255) NOT NULL COMMENT '원본 파일명',
    file_path       VARCHAR(500) NOT NULL COMMENT '저장 경로',
    file_size       BIGINT UNSIGNED NOT NULL COMMENT '파일 크기 (bytes)',
    file_type       VARCHAR(100) COMMENT 'MIME 타입',
    file_hash       VARCHAR(64) COMMENT 'SHA-256 해시',

    -- 메타
    display_name    VARCHAR(255) COMMENT '표시 이름',
    description     TEXT COMMENT '파일 설명',
    sort_order      INT DEFAULT 0 COMMENT '정렬 순서',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_product_files_product
        FOREIGN KEY (product_id) REFERENCES commerce_products(id) ON DELETE CASCADE,

    INDEX idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 3. 장바구니 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_carts (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id         BIGINT UNSIGNED NOT NULL COMMENT '사용자 ID',
    product_id      BIGINT UNSIGNED NOT NULL COMMENT '상품 ID',
    quantity        INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '수량',

    -- 메타
    meta_data       JSON COMMENT '추가 옵션 등',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_carts_product
        FOREIGN KEY (product_id) REFERENCES commerce_products(id) ON DELETE CASCADE,

    -- 사용자당 상품 중복 방지
    UNIQUE KEY idx_user_product (user_id, product_id),
    INDEX idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 4. 주문 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_orders (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    order_number    VARCHAR(32) NOT NULL COMMENT '주문번호',
    user_id         BIGINT UNSIGNED NOT NULL COMMENT '구매자 ID',

    -- 금액
    subtotal        DECIMAL(12, 2) NOT NULL COMMENT '상품 합계',
    discount        DECIMAL(12, 2) DEFAULT 0 COMMENT '할인 금액',
    shipping_fee    DECIMAL(12, 2) DEFAULT 0 COMMENT '배송비',
    total           DECIMAL(12, 2) NOT NULL COMMENT '최종 결제 금액',
    currency        CHAR(3) NOT NULL DEFAULT 'KRW',

    -- 상태
    status          ENUM('pending', 'paid', 'processing', 'shipped', 'delivered', 'completed', 'cancelled', 'refunded')
                    NOT NULL DEFAULT 'pending',

    -- 배송 정보 (실물 상품용)
    shipping_name       VARCHAR(100) COMMENT '수령인',
    shipping_phone      VARCHAR(20) COMMENT '연락처',
    shipping_address    VARCHAR(500) COMMENT '배송 주소',
    shipping_postal     VARCHAR(10) COMMENT '우편번호',
    shipping_memo       VARCHAR(255) COMMENT '배송 메모',

    -- 송장 정보
    shipping_carrier    VARCHAR(50) COMMENT '배송사 코드',
    tracking_number     VARCHAR(100) COMMENT '송장번호',
    shipped_at          TIMESTAMP NULL COMMENT '발송일',
    delivered_at        TIMESTAMP NULL COMMENT '배송완료일',

    -- 메타
    ip_address      VARCHAR(45) COMMENT '주문 IP',
    user_agent      VARCHAR(500) COMMENT 'User Agent',
    meta_data       JSON COMMENT '추가 메타데이터',
    notes           TEXT COMMENT '관리자 메모',

    -- 타임스탬프
    paid_at         TIMESTAMP NULL COMMENT '결제 완료일',
    completed_at    TIMESTAMP NULL COMMENT '주문 완료일',
    cancelled_at    TIMESTAMP NULL COMMENT '취소일',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 인덱스
    UNIQUE KEY idx_order_number (order_number),
    INDEX idx_user (user_id),
    INDEX idx_status (status),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 5. 주문 아이템 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_order_items (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    order_id        BIGINT UNSIGNED NOT NULL,
    product_id      BIGINT UNSIGNED NOT NULL,
    seller_id       BIGINT UNSIGNED NOT NULL COMMENT '판매자 ID',

    -- 상품 스냅샷 (주문 시점 정보 보존)
    product_name    VARCHAR(255) NOT NULL,
    product_type    ENUM('digital', 'physical') NOT NULL,

    -- 금액
    price           DECIMAL(12, 2) NOT NULL COMMENT '개당 가격',
    quantity        INT UNSIGNED NOT NULL DEFAULT 1,
    subtotal        DECIMAL(12, 2) NOT NULL COMMENT '소계 (price * quantity)',

    -- 정산 정보
    platform_fee_rate   DECIMAL(5, 2) COMMENT '플랫폼 수수료율 (%)',
    platform_fee        DECIMAL(12, 2) COMMENT '플랫폼 수수료',
    seller_amount       DECIMAL(12, 2) COMMENT '판매자 정산금액',

    -- 상태
    status          ENUM('pending', 'processing', 'completed', 'refunded') DEFAULT 'pending',

    -- 메타
    meta_data       JSON COMMENT '상품 옵션 등',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_order_items_order
        FOREIGN KEY (order_id) REFERENCES commerce_orders(id) ON DELETE CASCADE,
    CONSTRAINT fk_order_items_product
        FOREIGN KEY (product_id) REFERENCES commerce_products(id) ON DELETE RESTRICT,

    INDEX idx_order (order_id),
    INDEX idx_product (product_id),
    INDEX idx_seller (seller_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 6. 결제 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_payments (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    order_id        BIGINT UNSIGNED NOT NULL,

    -- PG 정보
    pg_provider     VARCHAR(50) NOT NULL COMMENT 'PG사 (inicis, tosspay, kakaopay)',
    pg_tid          VARCHAR(100) COMMENT 'PG 거래 ID',
    pg_order_id     VARCHAR(100) COMMENT 'PG 주문 ID',

    -- 결제 정보
    payment_method  VARCHAR(50) COMMENT '결제 수단 (card, bank, virtual, phone)',
    amount          DECIMAL(12, 2) NOT NULL COMMENT '결제 금액',
    currency        CHAR(3) NOT NULL DEFAULT 'KRW',

    -- 상태
    status          ENUM('pending', 'ready', 'paid', 'cancelled', 'partial_cancelled', 'failed')
                    NOT NULL DEFAULT 'pending',

    -- 카드 정보 (마스킹)
    card_company    VARCHAR(50) COMMENT '카드사',
    card_number     VARCHAR(20) COMMENT '마스킹된 카드번호',
    card_type       VARCHAR(20) COMMENT '카드 타입 (credit, check)',
    install_month   INT COMMENT '할부 개월',

    -- 가상계좌 정보
    vbank_name      VARCHAR(50) COMMENT '가상계좌 은행',
    vbank_number    VARCHAR(50) COMMENT '가상계좌 번호',
    vbank_holder    VARCHAR(50) COMMENT '예금주',
    vbank_due       TIMESTAMP NULL COMMENT '입금 기한',

    -- 수수료
    pg_fee          DECIMAL(12, 2) COMMENT 'PG 수수료',

    -- 취소/환불
    cancelled_amount    DECIMAL(12, 2) DEFAULT 0 COMMENT '취소/환불 금액',
    cancel_reason       VARCHAR(255) COMMENT '취소 사유',
    cancelled_at        TIMESTAMP NULL,

    -- 메타
    raw_response    JSON COMMENT 'PG 원본 응답',
    meta_data       JSON COMMENT '추가 메타데이터',

    -- 타임스탬프
    paid_at         TIMESTAMP NULL COMMENT '결제 완료 시각',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_payments_order
        FOREIGN KEY (order_id) REFERENCES commerce_orders(id) ON DELETE CASCADE,

    INDEX idx_order (order_id),
    INDEX idx_pg_tid (pg_tid),
    INDEX idx_status (status),
    INDEX idx_paid (paid_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 7. 다운로드 로그 테이블 (디지털 상품용)
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_downloads (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    order_item_id   BIGINT UNSIGNED NOT NULL,
    file_id         BIGINT UNSIGNED NOT NULL,
    user_id         BIGINT UNSIGNED NOT NULL,

    -- 다운로드 정보
    download_token  VARCHAR(64) NOT NULL COMMENT '다운로드 토큰',
    download_count  INT UNSIGNED DEFAULT 0 COMMENT '다운로드 횟수',
    last_download_at TIMESTAMP NULL COMMENT '마지막 다운로드 시각',
    expires_at      TIMESTAMP NULL COMMENT '만료 시각',

    -- 요청 정보
    ip_address      VARCHAR(45) COMMENT 'IP 주소',
    user_agent      VARCHAR(500) COMMENT 'User Agent',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 외래키
    CONSTRAINT fk_downloads_order_item
        FOREIGN KEY (order_item_id) REFERENCES commerce_order_items(id) ON DELETE CASCADE,
    CONSTRAINT fk_downloads_file
        FOREIGN KEY (file_id) REFERENCES commerce_product_files(id) ON DELETE CASCADE,

    UNIQUE KEY idx_token (download_token),
    INDEX idx_order_item (order_item_id),
    INDEX idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 8. 정산 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_settlements (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    seller_id       BIGINT UNSIGNED NOT NULL COMMENT '판매자 ID',

    -- 정산 기간
    period_start    DATE NOT NULL COMMENT '정산 시작일',
    period_end      DATE NOT NULL COMMENT '정산 종료일',

    -- 금액
    total_sales     DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT '총 매출',
    total_refunds   DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT '총 환불',
    pg_fees         DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT 'PG 수수료 합계',
    platform_fees   DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT '플랫폼 수수료 합계',
    settlement_amount DECIMAL(12, 2) NOT NULL DEFAULT 0 COMMENT '정산 금액',
    currency        CHAR(3) NOT NULL DEFAULT 'KRW',

    -- 상태
    status          ENUM('pending', 'processing', 'completed', 'failed') NOT NULL DEFAULT 'pending',

    -- 입금 정보
    bank_name       VARCHAR(50) COMMENT '은행명',
    bank_account    VARCHAR(50) COMMENT '계좌번호',
    bank_holder     VARCHAR(50) COMMENT '예금주',

    -- 처리 정보
    processed_at    TIMESTAMP NULL COMMENT '정산 처리 시각',
    processed_by    BIGINT UNSIGNED COMMENT '처리자 ID',

    -- 메타
    notes           TEXT COMMENT '메모',
    meta_data       JSON COMMENT '상세 내역',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_seller (seller_id),
    INDEX idx_period (period_start, period_end),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 9. 플러그인 설정 테이블
-- ============================================
CREATE TABLE IF NOT EXISTS commerce_settings (
    id              BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    setting_key     VARCHAR(100) NOT NULL COMMENT '설정 키',
    setting_value   JSON COMMENT '설정 값',

    -- 타임스탬프
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY idx_key (setting_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================
-- 초기 설정 데이터
-- ============================================
INSERT INTO commerce_settings (setting_key, setting_value) VALUES
('platform_fee_rate', '5.0'),
('pg_providers', '["inicis", "tosspay"]'),
('enabled_product_types', '["digital", "physical"]'),
('download_expiry_days', '30'),
('download_limit_default', '5');
