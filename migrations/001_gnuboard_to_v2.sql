-- ============================================================
-- Gnuboard (g5_*) → v2 Data Migration Script
-- ============================================================
-- 실행 전 주의사항:
-- 1. v2 스키마가 먼저 생성되어야 합니다 (RunV2Schema)
-- 2. 운영 DB에서는 반드시 트랜잭션 단위로 실행
-- 3. 마이그레이션 중 서비스 중단 없음 (INSERT ... SELECT)
-- ============================================================

-- 1. 회원 마이그레이션: g5_member → v2_users
INSERT INTO v2_users (username, email, password, nickname, level, status, bio, created_at, updated_at)
SELECT
    mb_id,
    CASE WHEN mb_email = '' THEN CONCAT(mb_id, '@legacy.local') ELSE mb_email END,
    mb_password,
    mb_nick,
    LEAST(mb_level, 10),
    CASE
        WHEN mb_leave_date != '' THEN 'inactive'
        WHEN mb_intercept_date != '' THEN 'banned'
        ELSE 'active'
    END,
    NULLIF(mb_profile, ''),
    mb_datetime,
    mb_datetime
FROM g5_member
WHERE mb_id != ''
ON DUPLICATE KEY UPDATE username = VALUES(username);

-- 2. 게시판 마이그레이션: g5_board → v2_boards
INSERT INTO v2_boards (slug, name, description, is_active, order_num, created_at, updated_at)
SELECT
    bo_table,
    bo_subject,
    NULLIF(bo_content_head, ''),
    TRUE,
    bo_order,
    NOW(),
    NOW()
FROM g5_board
ON DUPLICATE KEY UPDATE slug = VALUES(slug);

-- 3. 게시글 마이그레이션 (각 g5_write_{board_id} 테이블에서)
-- 주의: 이 부분은 프로시저 또는 앱 레벨에서 동적으로 실행해야 합니다.
-- 아래는 예시이며, 실제로는 Go 코드에서 게시판 목록을 순회하며 실행합니다.

-- 예시: g5_write_free → v2_posts
-- INSERT INTO v2_posts (board_id, user_id, title, content, status, view_count, comment_count, is_notice, created_at, updated_at)
-- SELECT
--     (SELECT id FROM v2_boards WHERE slug = 'free'),
--     COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id), 1),
--     w.wr_subject,
--     w.wr_content,
--     'published',
--     w.wr_hit,
--     w.wr_comment,
--     CASE WHEN w.wr_option LIKE '%notice%' THEN TRUE ELSE FALSE END,
--     w.wr_datetime,
--     w.wr_last
-- FROM g5_write_free w
-- WHERE w.wr_is_comment = 0 AND w.wr_id = w.wr_parent;

-- 4. 댓글 마이그레이션 (동적 테이블 - Go 코드에서 실행)
-- INSERT INTO v2_comments (post_id, user_id, content, depth, status, created_at, updated_at)
-- SELECT
--     (매핑 필요: wr_parent → v2_posts.id),
--     COALESCE((SELECT id FROM v2_users WHERE username = w.mb_id), 1),
--     w.wr_content,
--     0,
--     'active',
--     w.wr_datetime,
--     w.wr_last
-- FROM g5_write_free w
-- WHERE w.wr_is_comment = 1;

-- 5. 파일 마이그레이션: g5_board_file → v2_files
-- INSERT INTO v2_files (post_id, user_id, original_name, stored_name, mime_type, file_size, storage_path, download_count, created_at)
-- SELECT
--     (매핑 필요: wr_id → v2_posts.id),
--     COALESCE((SELECT id FROM v2_users WHERE username = bf.mb_id), 1),
--     bf.bf_source,
--     bf.bf_file,
--     bf.bf_type,
--     bf.bf_filesize,
--     CONCAT('/data/file/', bo_table, '/', bf.bf_file),
--     bf.bf_download,
--     bf.bf_datetime
-- FROM g5_board_file bf;

-- ============================================================
-- 검증 쿼리
-- ============================================================

-- SELECT 'v2_users' AS tbl, COUNT(*) AS cnt FROM v2_users
-- UNION ALL SELECT 'g5_member', COUNT(*) FROM g5_member WHERE mb_id != ''
-- UNION ALL SELECT 'v2_boards', COUNT(*) FROM v2_boards
-- UNION ALL SELECT 'g5_board', COUNT(*) FROM g5_board;
