-- ========================================
-- 기존 테이블에 site_id 컬럼 추가 (Free 플랜용)
-- ========================================
-- g5_member, g5_board는 05-gnuboard-tables.sql에서 생성되므로
-- 해당 파일 이후에 실행되어야 합니다.

-- 회원 테이블
ALTER TABLE `g5_member`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID (멀티 테넌트)',
ADD INDEX `idx_site_id` (`site_id`);

-- 게시판 설정 테이블
ALTER TABLE `g5_board`
ADD COLUMN `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX `idx_site_id` (`site_id`);
