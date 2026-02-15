-- =============================================================================
-- 테스트 게시글 및 댓글 Seed 데이터
-- =============================================================================

-- 자유게시판 테스트 게시글
INSERT IGNORE INTO g5_write_free (
    wr_id, wr_num, wr_parent, wr_is_comment, wr_comment,
    wr_subject, wr_content, mb_id, wr_name, wr_datetime, wr_hit, wr_good, wr_ip
) VALUES
-- 게시글 1
(1, -1, 1, 0, 2,
 '환영합니다! 테스트 게시판입니다.',
 '<p>안녕하세요! ANGPLE 테스트 환경에 오신 것을 환영합니다.</p><p>이 게시판은 개발 및 테스트 목적으로 사용됩니다.</p>',
 'admin', '관리자', NOW(), 100, 5, '127.0.0.1'),

-- 게시글 2
(2, -2, 2, 0, 1,
 'API 테스트 게시글',
 '<p>이 게시글은 API 테스트를 위해 작성되었습니다.</p><p>게시글 CRUD 기능을 테스트해보세요.</p>',
 'test1', '테스트1', NOW(), 50, 2, '127.0.0.1'),

-- 게시글 3
(3, -3, 3, 0, 0,
 '댓글 없는 게시글',
 '<p>이 게시글에는 댓글이 없습니다.</p>',
 'test2', '테스트2', NOW(), 30, 1, '127.0.0.1'),

-- 게시글 4
(4, -4, 4, 0, 3,
 '많은 댓글이 달린 게시글',
 '<p>이 게시글은 댓글 테스트용입니다.</p><p>여러 댓글이 달려있습니다.</p>',
 'test1', '테스트1', NOW(), 200, 10, '127.0.0.1'),

-- 게시글 5
(5, -5, 5, 0, 0,
 'Markdown 테스트',
 '<h2>제목 테스트</h2><ul><li>항목 1</li><li>항목 2</li></ul><pre><code>console.log("Hello World");</code></pre>',
 'power1', '파워유저1', NOW(), 80, 3, '127.0.0.1');

-- 댓글 (wr_is_comment = 1, wr_parent = 원글 ID)
INSERT IGNORE INTO g5_write_free (
    wr_id, wr_num, wr_parent, wr_is_comment, wr_comment, wr_comment_reply,
    wr_subject, wr_content, mb_id, wr_name, wr_datetime, wr_ip
) VALUES
-- 게시글 1의 댓글
(101, -1, 1, 1, 0, '',
 '', '환영합니다! 잘 부탁드립니다.',
 'test1', '테스트1', NOW(), '127.0.0.1'),
(102, -1, 1, 1, 0, '',
 '', '테스트 환경 잘 구성되었네요!',
 'test2', '테스트2', NOW(), '127.0.0.1'),

-- 게시글 2의 댓글
(103, -2, 2, 1, 0, '',
 '', 'API 잘 동작합니다!',
 'test3', '테스트3', NOW(), '127.0.0.1'),

-- 게시글 4의 댓글 (여러 개)
(104, -4, 4, 1, 0, '',
 '', '첫 번째 댓글입니다.',
 'test1', '테스트1', NOW(), '127.0.0.1'),
(105, -4, 4, 1, 0, '',
 '', '두 번째 댓글입니다.',
 'test2', '테스트2', NOW(), '127.0.0.1'),
(106, -4, 4, 1, 0, '',
 '', '세 번째 댓글입니다. 좋은 글이네요!',
 'test3', '테스트3', NOW(), '127.0.0.1');

-- Q&A 테스트 게시글
INSERT IGNORE INTO g5_write_qa (
    wr_id, wr_num, wr_parent, wr_is_comment, wr_comment,
    ca_name, wr_subject, wr_content, mb_id, wr_name, wr_datetime, wr_hit, wr_ip
) VALUES
(1, -1, 1, 0, 1,
 '질문', 'Go API에서 JWT 토큰 만료 시간 설정 방법',
 '<p>JWT 토큰의 만료 시간을 설정하는 방법이 궁금합니다.</p><p>config에서 설정하나요?</p>',
 'test1', '테스트1', NOW(), 45, '127.0.0.1'),

(2, -2, 2, 0, 0,
 '답변완료', 'GORM에서 동적 테이블 사용하는 방법',
 '<p>게시판마다 다른 테이블을 사용해야 하는데 어떻게 하나요?</p>',
 'test2', '테스트2', NOW(), 60, '127.0.0.1');

-- Q&A 댓글
INSERT IGNORE INTO g5_write_qa (
    wr_id, wr_num, wr_parent, wr_is_comment, wr_comment_reply,
    wr_subject, wr_content, mb_id, wr_name, wr_datetime, wr_ip
) VALUES
(101, -1, 1, 1, '',
 '', 'config.yaml의 jwt.expires_in 값을 수정하시면 됩니다. 기본값은 900초(15분)입니다.',
 'power1', '파워유저1', NOW(), '127.0.0.1');

-- 공지사항
INSERT IGNORE INTO g5_write_notice (
    wr_id, wr_num, wr_parent, wr_is_comment, wr_comment,
    wr_subject, wr_content, mb_id, wr_name, wr_datetime, wr_hit, wr_ip
) VALUES
(1, -1, 1, 0, 0,
 '[공지] ANGPLE 테스트 서버 안내',
 '<p>이 서버는 ANGPLE 개발 및 테스트 목적으로 운영됩니다.</p><h3>테스트 계정</h3><ul><li>ID: test1 ~ test5</li><li>비밀번호: test1234</li></ul>',
 'admin', '관리자', NOW(), 500, '127.0.0.1');

-- 게시판 글/댓글 수 업데이트
UPDATE g5_board SET bo_count_write = 5, bo_count_comment = 6 WHERE bo_table = 'free';
UPDATE g5_board SET bo_count_write = 2, bo_count_comment = 1 WHERE bo_table = 'qa';
UPDATE g5_board SET bo_count_write = 1, bo_count_comment = 0 WHERE bo_table = 'notice';
