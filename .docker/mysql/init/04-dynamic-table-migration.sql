-- ========================================
-- 동적 게시판 테이블 멀티 테넌트 마이그레이션 헬퍼
-- ========================================
-- 작성일: 2026-01-07
-- 목적: g5_write_* 동적 테이블에 site_id 컬럼 자동 추가
-- 사용법: 이 파일은 참고용이며, 프로비저닝 시 스크립트에서 자동 실행됨
-- ========================================

-- ========================================
-- 1. 기존 동적 테이블 마이그레이션 (개발 환경용)
-- ========================================

-- 자유게시판
ALTER TABLE `g5_write_free`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- QNA 게시판
ALTER TABLE `g5_write_qna`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- 공지사항 게시판
ALTER TABLE `g5_write_notice`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- 갤러리 게시판
ALTER TABLE `g5_write_gallery`
ADD COLUMN IF NOT EXISTS `site_id` VARCHAR(36) DEFAULT 'default' COMMENT '소속 사이트 ID',
ADD INDEX IF NOT EXISTS `idx_site_id` (`site_id`);

-- ========================================
-- 2. 동적 테이블 자동 마이그레이션 프로시저
-- ========================================
-- 모든 g5_write_* 테이블을 자동으로 스캔하여 site_id 추가

DELIMITER $$

DROP PROCEDURE IF EXISTS `sp_migrate_all_write_tables`$$

CREATE PROCEDURE `sp_migrate_all_write_tables`()
BEGIN
    DECLARE done INT DEFAULT FALSE;
    DECLARE tbl_name VARCHAR(255);
    DECLARE alter_sql TEXT;

    -- g5_write_*로 시작하는 모든 테이블 이름 가져오기
    DECLARE cur CURSOR FOR
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = DATABASE()
          AND table_name LIKE 'g5\_write\_%';

    DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = TRUE;

    OPEN cur;

    read_loop: LOOP
        FETCH cur INTO tbl_name;
        IF done THEN
            LEAVE read_loop;
        END IF;

        -- site_id 컬럼이 없는 경우에만 추가
        SET @check_sql = CONCAT(
            'SELECT COUNT(*) INTO @col_exists ',
            'FROM information_schema.columns ',
            'WHERE table_schema = DATABASE() ',
            '  AND table_name = ''', tbl_name, ''' ',
            '  AND column_name = ''site_id'''
        );

        PREPARE stmt FROM @check_sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;

        IF @col_exists = 0 THEN
            -- site_id 컬럼 추가
            SET @alter_sql = CONCAT(
                'ALTER TABLE `', tbl_name, '` ',
                'ADD COLUMN `site_id` VARCHAR(36) DEFAULT ''default'' COMMENT ''소속 사이트 ID'', ',
                'ADD INDEX `idx_site_id` (`site_id`)'
            );

            PREPARE stmt FROM @alter_sql;
            EXECUTE stmt;
            DEALLOCATE PREPARE stmt;

            SELECT CONCAT('✅ 마이그레이션 완료: ', tbl_name) AS status;
        ELSE
            SELECT CONCAT('⏭️  스킵 (이미 존재): ', tbl_name) AS status;
        END IF;
    END LOOP;

    CLOSE cur;

    SELECT '✅ 전체 동적 테이블 마이그레이션 완료!' AS final_status;
END$$

DELIMITER ;

-- ========================================
-- 3. 프로시저 실행 (주석 해제하여 사용)
-- ========================================

-- CALL sp_migrate_all_write_tables();

-- ========================================
-- 4. 새 사이트 생성 시 동적 테이블 초기화 프로시저
-- ========================================
-- 프로비저닝 시 호출: 사이트별로 게시판 테이블 생성하고 site_id 자동 설정

DELIMITER $$

DROP PROCEDURE IF EXISTS `sp_create_site_board_table`$$

CREATE PROCEDURE `sp_create_site_board_table`(
    IN p_site_id VARCHAR(36),
    IN p_board_id VARCHAR(20)
)
BEGIN
    DECLARE table_name VARCHAR(100);

    SET table_name = CONCAT('g5_write_', p_board_id);

    -- 테이블 생성 (그누보드 기본 구조 + site_id)
    SET @create_sql = CONCAT(
        'CREATE TABLE IF NOT EXISTS `', table_name, '` (',
        '  `wr_id` INT AUTO_INCREMENT PRIMARY KEY,',
        '  `wr_num` INT NOT NULL DEFAULT 0,',
        '  `wr_reply` VARCHAR(10) NOT NULL DEFAULT '''',',
        '  `wr_parent` INT NOT NULL DEFAULT 0,',
        '  `wr_is_comment` TINYINT NOT NULL DEFAULT 0,',
        '  `wr_comment` INT NOT NULL DEFAULT 0,',
        '  `wr_comment_reply` VARCHAR(5) NOT NULL DEFAULT '''',',
        '  `ca_name` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_option` SET(''html1'',''html2'',''secret'',''mail'') NOT NULL DEFAULT '''',',
        '  `wr_subject` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_content` TEXT NOT NULL,',
        '  `wr_link1` TEXT NOT NULL,',
        '  `wr_link2` TEXT NOT NULL,',
        '  `wr_link1_hit` INT NOT NULL DEFAULT 0,',
        '  `wr_link2_hit` INT NOT NULL DEFAULT 0,',
        '  `wr_hit` INT NOT NULL DEFAULT 0,',
        '  `wr_good` INT NOT NULL DEFAULT 0,',
        '  `wr_nogood` INT NOT NULL DEFAULT 0,',
        '  `mb_id` VARCHAR(50) NOT NULL DEFAULT '''',',
        '  `wr_password` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_name` VARCHAR(50) NOT NULL DEFAULT '''',',
        '  `wr_email` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_homepage` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_datetime` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,',
        '  `wr_file` TINYINT NOT NULL DEFAULT 0,',
        '  `wr_last` VARCHAR(19) NOT NULL DEFAULT '''',',
        '  `wr_ip` VARCHAR(50) NOT NULL DEFAULT '''',',
        '  `wr_1` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_2` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_3` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_4` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_5` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_6` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_7` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_8` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_9` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `wr_10` VARCHAR(255) NOT NULL DEFAULT '''',',
        '  `site_id` VARCHAR(36) DEFAULT ''', p_site_id, ''' COMMENT ''소속 사이트 ID'',',
        '  INDEX `idx_wr_num_reply_parent` (`wr_num`, `wr_reply`, `wr_parent`),',
        '  INDEX `idx_wr_is_comment` (`wr_is_comment`, `wr_id`),',
        '  INDEX `idx_wr_datetime` (`wr_datetime`),',
        '  INDEX `idx_site_id` (`site_id`)',
        ') ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci'
    );

    PREPARE stmt FROM @create_sql;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    SELECT CONCAT('✅ 게시판 테이블 생성: ', table_name, ' (site_id=', p_site_id, ')') AS status;
END$$

DELIMITER ;

-- ========================================
-- 5. 사용 예시 (프로비저닝 스크립트에서 호출)
-- ========================================

-- 새 사이트의 게시판 생성 예시:
-- CALL sp_create_site_board_table('demo-free-001', 'free');
-- CALL sp_create_site_board_table('demo-free-001', 'qna');
-- CALL sp_create_site_board_table('demo-free-001', 'notice');

-- ========================================
-- 6. 기존 데이터 site_id 업데이트 (개발 환경용)
-- ========================================
-- 주의: 실제 운영 환경에서는 데이터 백업 후 신중히 실행할 것

-- 기본 사이트(다모앙)의 데이터는 'default'로 유지
-- UPDATE g5_write_free SET site_id = 'default' WHERE site_id IS NULL OR site_id = '';
-- UPDATE g5_write_qna SET site_id = 'default' WHERE site_id IS NULL OR site_id = '';
-- UPDATE g5_member SET site_id = 'default' WHERE site_id IS NULL OR site_id = '';

-- ========================================
-- 완료!
-- ========================================

SELECT
    '✅ 동적 테이블 마이그레이션 헬퍼 준비 완료!' AS status,
    'sp_migrate_all_write_tables() - 모든 g5_write_* 테이블에 site_id 추가' AS procedure_1,
    'sp_create_site_board_table(site_id, board_id) - 새 사이트 게시판 생성' AS procedure_2;
