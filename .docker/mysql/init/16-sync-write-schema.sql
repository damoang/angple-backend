-- 게시판 테이블(g5_write_*)을 운영 스키마에 맞춘다 (2026-10-12, 운영 information_schema 기준)
-- 개발 초기화가 운영보다 뒤처져 API 가 「Unknown column wr_deleted_at」·「Key idx_list_page doesn't exist」로 실패했다.
-- 모든 g5_write_* 테이블에 대해 없는 컬럼·인덱스만 추가한다(이미 있으면 건너뜀).
DELIMITER $$
DROP PROCEDURE IF EXISTS sync_write_schema $$
CREATE PROCEDURE sync_write_schema()
BEGIN
  DECLARE done INT DEFAULT 0;
  DECLARE t VARCHAR(64);
  DECLARE cur CURSOR FOR
    SELECT table_name FROM information_schema.tables
    WHERE table_schema = DATABASE() AND table_name LIKE 'g5\_write\_%' AND table_type = 'BASE TABLE';
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;
  OPEN cur;
  each_table: LOOP
    FETCH cur INTO t;
    IF done THEN LEAVE each_table; END IF;
    CALL add_col(t, 'wr_facebook_user', "VARCHAR(255) NOT NULL DEFAULT ''");
    CALL add_col(t, 'wr_twitter_user', "VARCHAR(255) NOT NULL DEFAULT ''");
    CALL add_col(t, 'wr_deleted_at', 'DATETIME NULL DEFAULT NULL');
    CALL add_col(t, 'wr_deleted_by', 'VARCHAR(20) NULL DEFAULT NULL');
    CALL add_col(t, 'wr_edit_count', 'INT NOT NULL DEFAULT 0');
    CALL add_col(t, 'wr_last_edited_at', 'DATETIME NULL DEFAULT NULL');
    CALL add_idx(t, 'idx_list_page', 'wr_is_comment, wr_num, wr_reply');
    CALL add_idx(t, 'idx_list_deleted', 'wr_is_comment, wr_deleted_at, wr_num, wr_reply');
    CALL add_idx(t, 'idx_comment_deleted', 'wr_is_comment, wr_deleted_at');
    CALL add_idx(t, 'idx_comment_paging', 'wr_parent, wr_comment, wr_comment_reply');
    CALL add_idx(t, 'idx_good_cover', 'wr_is_comment, wr_good, wr_id');
    CALL add_idx(t, 'idx_mb_comment', 'mb_id, wr_is_comment');
    CALL add_idx(t, 'wr_is_comment', 'wr_is_comment, wr_id');
    CALL add_idx(t, 'ix_wr_datetime', 'wr_datetime');
  END LOOP;
  CLOSE cur;
END $$

DROP PROCEDURE IF EXISTS add_col $$
CREATE PROCEDURE add_col(IN t VARCHAR(64), IN c VARCHAR(64), IN def VARCHAR(255))
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_schema = DATABASE() AND table_name = t AND column_name = c) THEN
    SET @s = CONCAT('ALTER TABLE `', t, '` ADD COLUMN `', c, '` ', def);
    PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
  END IF;
END $$

DROP PROCEDURE IF EXISTS add_idx $$
CREATE PROCEDURE add_idx(IN t VARCHAR(64), IN i VARCHAR(64), IN cols VARCHAR(255))
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics
                 WHERE table_schema = DATABASE() AND table_name = t AND index_name = i) THEN
    SET @s = CONCAT('ALTER TABLE `', t, '` ADD INDEX `', i, '` (', cols, ')');
    PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
  END IF;
END $$
DELIMITER ;

CALL sync_write_schema();
DROP PROCEDURE IF EXISTS sync_write_schema;
DROP PROCEDURE IF EXISTS add_col;
DROP PROCEDURE IF EXISTS add_idx;

-- 글쓰기 후처리 이벤트 큐(운영 SHOW CREATE TABLE 기준) — 글 조회·작성 경로가 참조
CREATE TABLE IF NOT EXISTS `g5_write_after_events` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `event_type` varchar(40) COLLATE utf8mb4_unicode_ci NOT NULL,
  `board_slug` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL,
  `write_id` int NOT NULL,
  `post_id` int DEFAULT NULL,
  `parent_id` int DEFAULT NULL,
  `member_id` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `author` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `subject` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `occurred_at` datetime NOT NULL,
  `available_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `status` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending',
  `retry_count` int NOT NULL DEFAULT '0',
  `last_error` text COLLATE utf8mb4_unicode_ci,
  `claimed_at` datetime DEFAULT NULL,
  `processed_at` datetime DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_status_available` (`status`,`available_at`,`id`),
  KEY `idx_board_write` (`board_slug`,`write_id`),
  KEY `idx_post_status` (`post_id`,`status`),
  KEY `idx_processed_at` (`processed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
