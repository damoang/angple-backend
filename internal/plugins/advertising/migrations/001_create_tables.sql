-- Advertising Plugin Migration
-- Version: 001
-- Description: Create ad_units, ad_rotation_config, celebration_banners tables

-- 광고 단위 테이블 (GAM/AdSense)
CREATE TABLE IF NOT EXISTS ad_units (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(50) NOT NULL COMMENT '광고 단위 이름 (main, sub, article 등)',
    ad_type ENUM('gam', 'adsense') NOT NULL COMMENT '광고 유형',
    gam_unit_path VARCHAR(255) COMMENT 'GAM 광고 단위 경로 (/네트워크코드/광고단위)',
    adsense_slot VARCHAR(50) COMMENT 'AdSense 슬롯 ID',
    adsense_client VARCHAR(50) COMMENT 'AdSense 클라이언트 ID (ca-pub-...)',
    sizes JSON COMMENT '광고 사이즈 배열 [[728,90], [970,90], [300,250]]',
    responsive_breakpoints JSON COMMENT '반응형 브레이크포인트 설정',
    position VARCHAR(50) NOT NULL COMMENT '광고 위치 (main_top, sidebar_1 등)',
    priority INT DEFAULT 0 COMMENT '표시 우선순위',
    is_active BOOLEAN DEFAULT TRUE COMMENT '활성화 여부',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_position (position),
    INDEX idx_ad_type (ad_type),
    INDEX idx_is_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='광고 단위 설정';

-- AdSense 슬롯 로테이션 설정 테이블
CREATE TABLE IF NOT EXISTS ad_rotation_config (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    position VARCHAR(50) NOT NULL COMMENT '광고 위치',
    slot_pool JSON NOT NULL COMMENT '슬롯 ID 풀 ["slot1", "slot2", "slot3"]',
    rotation_strategy ENUM('sequential', 'random', 'weighted') DEFAULT 'sequential' COMMENT '로테이션 전략',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_position (position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='AdSense 슬롯 로테이션 설정';

-- 축하 배너 테이블
CREATE TABLE IF NOT EXISTS celebration_banners (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    title VARCHAR(255) NOT NULL COMMENT '배너 제목',
    content TEXT COMMENT '배너 내용',
    image_url VARCHAR(500) COMMENT '배너 이미지 URL',
    link_url VARCHAR(500) COMMENT '클릭 시 이동 URL',
    display_date DATE NOT NULL COMMENT '표시할 날짜',
    is_active BOOLEAN DEFAULT TRUE COMMENT '활성화 여부',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_display_date (display_date),
    INDEX idx_is_active (is_active),
    INDEX idx_display_active (display_date, is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='축하 배너';

-- 기본 GAM 광고 단위 데이터 삽입
INSERT INTO ad_units (name, ad_type, gam_unit_path, sizes, responsive_breakpoints, position, priority, is_active) VALUES
-- 메인 콘텐츠 영역 (main)
('banner-horizontal', 'gam', '/22996793498/damoang/banner-responsive_main', '[[970,250],[970,90],[728,90],[320,100],[300,250]]', '[[970,[[970,250],[970,90],[728,90]]],[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-horizontal', 100, TRUE),
('banner-large', 'gam', '/22996793498/damoang/banner-responsive_main', '[[970,250],[970,90],[728,90],[320,100],[300,250]]', '[[970,[[970,250],[970,90]]],[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-large', 90, TRUE),
('banner-large-728', 'gam', '/22996793498/damoang/banner-responsive_main', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-large-728', 80, TRUE),

-- 게시글 본문 영역 (article)
('banner-view-content', 'gam', '/22996793498/damoang/banner-responsive_article', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-view-content', 70, TRUE),
('banner-horizontal-728', 'gam', '/22996793498/damoang/banner-responsive_article', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-horizontal-728', 60, TRUE),

-- 반응형 배너 (sub)
('banner-responsive', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-responsive', 50, TRUE),
('banner-medium', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'banner-medium', 40, TRUE),
('banner-small', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[728,90],[320,100]]', '[[728,[[728,90]]],[0,[[320,100]]]]', 'banner-small', 30, TRUE),
('banner-compact', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[728,90],[320,100]]', '[[728,[[728,90]]],[0,[[320,100]]]]', 'banner-compact', 20, TRUE),

-- 사이드바 (sub)
('banner-square', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[300,250]]', NULL, 'banner-square', 10, TRUE),
('banner-vertical', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[160,600]]', NULL, 'banner-vertical', 5, TRUE),
('banner-halfpage', 'gam', '/22996793498/damoang/banner-responsive_sub', '[[300,600],[300,250]]', NULL, 'banner-halfpage', 5, TRUE),

-- 인피드 (curation)
('infeed', 'gam', '/22996793498/damoang/banner-responsive_curation', '[[728,90],[320,100],[300,250]]', '[[728,[[728,90]]],[0,[[320,100],[300,250]]]]', 'infeed', 50, TRUE)
ON DUPLICATE KEY UPDATE updated_at = CURRENT_TIMESTAMP;

-- 기본 AdSense 로테이션 설정 데이터 삽입
INSERT INTO ad_rotation_config (position, slot_pool, rotation_strategy) VALUES
('banner_horizontal', '["1282465226","2649190580","3781227288","2468145615","8268294873","1273950610","9281514713","1980527625"]', 'sequential'),
('banner_responsive', '["8336276313","5710112977","4188421399","8915162137","7602080468","7968433046","7041282612","9368595884"]', 'sequential'),
('banner_square', '["7466402991","5618613634","4744870889","3431789215","5728200944","3102037601","2349753787","1788955938","1090893531"]', 'sequential'),
('banner_vertical', '["7464730194","1774011047","8147847708","7273749737"]', 'sequential'),
('banner_small', '["8336276313","5710112977","4188421399","8915162137","7602080468","7968433046","7041282612","9368595884","1980732555","4258455619"]', 'sequential'),
('infeed', '["9024980950","8452181607","4153843942","7901517260","5861978607","4548896939","1922733594","7410508775"]', 'sequential'),
('infeed_dark', '["5858055273","2194142431","5346440459","5666483580","8199834500","6001046571","8567979094","4556102961"]', 'sequential')
ON DUPLICATE KEY UPDATE slot_pool = VALUES(slot_pool);
