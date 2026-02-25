-- ========================================
-- 동적 게시판 테이블에 site_id 컬럼 추가
-- ========================================
-- g5_write_* 테이블은 07-seed-boards.sql에서 생성되므로
-- 해당 파일 이후에 실행되어야 합니다.

-- 자유게시판
ALTER TABLE `g5_write_free`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);

-- QA 게시판
ALTER TABLE `g5_write_qa`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);

-- 공지사항 게시판
ALTER TABLE `g5_write_notice`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);

-- 갤러리 게시판
ALTER TABLE `g5_write_gallery`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);

-- 개발 게시판
ALTER TABLE `g5_write_development`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);
