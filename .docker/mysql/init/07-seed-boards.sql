-- =============================================================================
-- 테스트 게시판 Seed 데이터
-- =============================================================================

-- 기본 게시판 그룹
INSERT IGNORE INTO g5_group (gr_id, gr_subject, gr_admin) VALUES
('community', '커뮤니티', 'admin'),
('hobby', '취미', 'admin'),
('tech', '기술', 'admin');

-- 기본 게시판
INSERT IGNORE INTO g5_board (
    bo_table, gr_id, bo_subject, bo_admin,
    bo_list_level, bo_read_level, bo_write_level, bo_reply_level, bo_comment_level,
    bo_upload_level, bo_download_level,
    bo_upload_size, bo_upload_count, bo_page_rows,
    bo_skin, bo_mobile_skin, bo_include_head, bo_include_tail,
    bo_use_category, bo_category_list, bo_use_sns, bo_use_search,
    bo_order, bo_device
) VALUES
-- 자유게시판
('free', 'community', '자유게시판', 'admin',
 1, 1, 2, 2, 2,
 2, 1,
 20971520, 5, 20,
 'basic', 'basic', '', '',
 0, '', 1, 1,
 10, 'both'),

-- Q&A 게시판
('qa', 'community', 'Q&A', 'admin',
 1, 1, 2, 2, 2,
 2, 1,
 20971520, 5, 20,
 'basic', 'basic', '', '',
 1, '질문|답변완료|공지', 1, 1,
 20, 'both'),

-- 공지사항
('notice', 'community', '공지사항', 'admin',
 1, 1, 10, 10, 2,
 10, 1,
 20971520, 10, 20,
 'basic', 'basic', '', '',
 0, '', 0, 1,
 1, 'both'),

-- 갤러리
('gallery', 'hobby', '갤러리', 'admin',
 1, 1, 2, 2, 2,
 2, 1,
 52428800, 10, 20,
 'gallery', 'gallery', '', '',
 0, '', 1, 1,
 30, 'both'),

-- 개발 게시판
('development', 'tech', '개발', 'admin',
 1, 1, 2, 2, 2,
 2, 1,
 20971520, 5, 20,
 'basic', 'basic', '', '',
 1, 'Frontend|Backend|DevOps|기타', 1, 1,
 40, 'both');

-- 동적 게시판 테이블 생성 (그누보드 호환)
-- 자유게시판 테이블
CREATE TABLE IF NOT EXISTS g5_write_free (
    wr_id INT(11) NOT NULL AUTO_INCREMENT,
    wr_num INT(11) NOT NULL DEFAULT 0,
    wr_reply VARCHAR(10) NOT NULL DEFAULT '',
    wr_parent INT(11) NOT NULL DEFAULT 0,
    wr_is_comment TINYINT(4) NOT NULL DEFAULT 0,
    wr_comment INT(11) NOT NULL DEFAULT 0,
    wr_comment_reply VARCHAR(5) NOT NULL DEFAULT '',
    ca_name VARCHAR(255) NOT NULL DEFAULT '',
    wr_option VARCHAR(255) NOT NULL DEFAULT '',
    wr_subject VARCHAR(255) NOT NULL DEFAULT '',
    wr_content MEDIUMTEXT NOT NULL,
    wr_seo_title VARCHAR(255) NOT NULL DEFAULT '',
    wr_link1 TEXT NOT NULL,
    wr_link2 TEXT NOT NULL,
    wr_link1_hit INT(11) NOT NULL DEFAULT 0,
    wr_link2_hit INT(11) NOT NULL DEFAULT 0,
    wr_hit INT(11) NOT NULL DEFAULT 0,
    wr_good INT(11) NOT NULL DEFAULT 0,
    wr_nogood INT(11) NOT NULL DEFAULT 0,
    mb_id VARCHAR(20) NOT NULL DEFAULT '',
    wr_password VARCHAR(255) NOT NULL DEFAULT '',
    wr_name VARCHAR(255) NOT NULL DEFAULT '',
    wr_email VARCHAR(255) NOT NULL DEFAULT '',
    wr_homepage VARCHAR(255) NOT NULL DEFAULT '',
    wr_datetime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    wr_file INT(11) NOT NULL DEFAULT 0,
    wr_last VARCHAR(19) NOT NULL DEFAULT '',
    wr_ip VARCHAR(255) NOT NULL DEFAULT '',
    wr_facebook VARCHAR(255) NOT NULL DEFAULT '',
    wr_twitter VARCHAR(255) NOT NULL DEFAULT '',
    wr_1 VARCHAR(255) NOT NULL DEFAULT '',
    wr_2 VARCHAR(255) NOT NULL DEFAULT '',
    wr_3 VARCHAR(255) NOT NULL DEFAULT '',
    wr_4 VARCHAR(255) NOT NULL DEFAULT '',
    wr_5 VARCHAR(255) NOT NULL DEFAULT '',
    wr_6 VARCHAR(255) NOT NULL DEFAULT '',
    wr_7 VARCHAR(255) NOT NULL DEFAULT '',
    wr_8 VARCHAR(255) NOT NULL DEFAULT '',
    wr_9 VARCHAR(255) NOT NULL DEFAULT '',
    wr_10 VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (wr_id),
    KEY wr_num_reply (wr_num, wr_reply),
    KEY wr_parent (wr_parent),
    KEY mb_id (mb_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Q&A 게시판 테이블
CREATE TABLE IF NOT EXISTS g5_write_qa LIKE g5_write_free;

-- 공지사항 테이블
CREATE TABLE IF NOT EXISTS g5_write_notice LIKE g5_write_free;

-- 갤러리 테이블
CREATE TABLE IF NOT EXISTS g5_write_gallery LIKE g5_write_free;

-- 개발 게시판 테이블
CREATE TABLE IF NOT EXISTS g5_write_development LIKE g5_write_free;
