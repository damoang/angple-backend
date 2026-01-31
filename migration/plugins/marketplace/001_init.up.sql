-- 마켓플레이스 (중고거래) 플러그인 초기 스키마
-- Version: 1

-- 카테고리 테이블
CREATE TABLE IF NOT EXISTS `marketplace_categories` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `parent_id` BIGINT UNSIGNED DEFAULT NULL,
    `name` VARCHAR(50) NOT NULL COMMENT '카테고리명',
    `slug` VARCHAR(50) NOT NULL COMMENT '슬러그',
    `icon` VARCHAR(50) DEFAULT NULL COMMENT '아이콘',
    `order_num` INT NOT NULL DEFAULT 0 COMMENT '정렬 순서',
    `is_active` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '활성 여부',
    `item_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '상품 수',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_marketplace_categories_slug` (`slug`),
    KEY `idx_marketplace_categories_parent` (`parent_id`),
    KEY `idx_marketplace_categories_active` (`is_active`),
    CONSTRAINT `fk_marketplace_categories_parent`
        FOREIGN KEY (`parent_id`) REFERENCES `marketplace_categories` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='중고거래 카테고리';

-- 상품 테이블
CREATE TABLE IF NOT EXISTS `marketplace_items` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `seller_id` BIGINT UNSIGNED NOT NULL COMMENT '판매자 ID',
    `category_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '카테고리 ID',
    `title` VARCHAR(200) NOT NULL COMMENT '제목',
    `description` TEXT COMMENT '설명',
    `price` BIGINT NOT NULL COMMENT '가격',
    `original_price` BIGINT DEFAULT NULL COMMENT '원가 (정가)',
    `currency` VARCHAR(3) NOT NULL DEFAULT 'KRW' COMMENT '통화',
    `condition` VARCHAR(20) NOT NULL DEFAULT 'good' COMMENT '상태 (new, like_new, good, fair, poor)',
    `status` VARCHAR(20) NOT NULL DEFAULT 'selling' COMMENT '판매 상태 (selling, reserved, sold, hidden)',
    `trade_method` VARCHAR(20) NOT NULL DEFAULT 'both' COMMENT '거래 방법 (direct, delivery, both)',
    `location` VARCHAR(100) DEFAULT NULL COMMENT '거래 지역',
    `is_negotiable` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '가격 협상 가능',
    `view_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '조회수',
    `wish_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '찜 수',
    `chat_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '채팅 수',
    `images` JSON DEFAULT NULL COMMENT '이미지 목록 (JSON)',
    `buyer_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '구매자 ID',
    `sold_at` TIMESTAMP NULL DEFAULT NULL COMMENT '판매 완료 시간',
    `bumped_at` TIMESTAMP NULL DEFAULT NULL COMMENT '끌올 시간',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_marketplace_items_seller` (`seller_id`),
    KEY `idx_marketplace_items_category` (`category_id`),
    KEY `idx_marketplace_items_status` (`status`),
    KEY `idx_marketplace_items_created` (`created_at`),
    KEY `idx_marketplace_items_price` (`price`),
    KEY `idx_marketplace_items_bumped` (`bumped_at`),
    CONSTRAINT `fk_marketplace_items_category`
        FOREIGN KEY (`category_id`) REFERENCES `marketplace_categories` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='중고거래 상품';

-- 찜하기 테이블
CREATE TABLE IF NOT EXISTS `marketplace_wishes` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '사용자 ID',
    `item_id` BIGINT UNSIGNED NOT NULL COMMENT '상품 ID',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_marketplace_wishes_unique` (`user_id`, `item_id`),
    KEY `idx_marketplace_wishes_user` (`user_id`),
    KEY `idx_marketplace_wishes_item` (`item_id`),
    CONSTRAINT `fk_marketplace_wishes_item`
        FOREIGN KEY (`item_id`) REFERENCES `marketplace_items` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='중고거래 찜 목록';

-- 기본 카테고리 데이터 삽입
INSERT INTO `marketplace_categories` (`name`, `slug`, `icon`, `order_num`) VALUES
('디지털/가전', 'digital', 'smartphone', 1),
('가구/인테리어', 'furniture', 'sofa', 2),
('의류/잡화', 'fashion', 'shirt', 3),
('뷰티/미용', 'beauty', 'sparkles', 4),
('스포츠/레저', 'sports', 'dumbbell', 5),
('취미/게임', 'hobby', 'gamepad-2', 6),
('도서/문구', 'books', 'book', 7),
('유아/아동', 'kids', 'baby', 8),
('반려동물', 'pets', 'dog', 9),
('생활/주방', 'living', 'utensils', 10),
('기타', 'etc', 'more-horizontal', 99);

-- 하위 카테고리 (디지털/가전)
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '휴대폰', 'mobile', 'smartphone', 1 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '노트북/PC', 'computer', 'laptop', 2 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '태블릿', 'tablet', 'tablet', 3 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '카메라', 'camera', 'camera', 4 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '오디오/음향', 'audio', 'headphones', 5 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, 'TV/영상', 'tv', 'tv', 6 FROM `marketplace_categories` WHERE slug = 'digital';
INSERT INTO `marketplace_categories` (`parent_id`, `name`, `slug`, `icon`, `order_num`)
SELECT id, '생활가전', 'appliance', 'refrigerator', 7 FROM `marketplace_categories` WHERE slug = 'digital';
