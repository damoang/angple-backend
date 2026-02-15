-- =============================================================================
-- Seed Data for Memo, Reaction, Report Tables
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Member Memo Seed Data
-- -----------------------------------------------------------------------------
INSERT IGNORE INTO g5_member_memo (member_uid, member_id, target_member_uid, target_member_id, memo, memo_detail, color) VALUES
-- test1이 다른 회원에 대해 메모
(2, 'test1', 1, 'admin', '관리자님', '문의사항 있을 때 연락하기', 'blue'),
(2, 'test1', 3, 'test2', '친한 친구', '같이 게임하는 친구', 'green'),
(2, 'test1', 6, 'power1', '도움 많이 받음', '기술적인 질문에 잘 답해줌', 'yellow'),

-- test2가 다른 회원에 대해 메모
(3, 'test2', 2, 'test1', '좋은 사람', NULL, 'yellow'),
(3, 'test2', 1, 'admin', '사이트 관리자', '건의사항 전달 시 연락', 'red'),

-- admin이 회원들에 대해 메모 (관리 목적)
(1, 'admin', 2, 'test1', '활동 회원', '적극적으로 커뮤니티 활동 중', 'green'),
(1, 'admin', 7, 'power2', '우수 기여자', '양질의 글 작성', 'blue');

-- -----------------------------------------------------------------------------
-- Reaction Seed Data
-- 자유게시판 댓글에 대한 반응
-- target_id 형식: comment:{board_id}:{comment_id}
-- parent_id 형식: document:{board_id}:{post_id}
-- -----------------------------------------------------------------------------
INSERT IGNORE INTO g5_da_reaction (target_id, parent_id, reaction, reaction_count) VALUES
-- 게시글 1의 댓글 101에 대한 반응
('comment:free:101', 'document:free:1', 'emoji:thumbsup', 3),
('comment:free:101', 'document:free:1', 'emoji:heart', 2),
('comment:free:101', 'document:free:1', 'emoji:laughing', 1),

-- 게시글 1의 댓글 102에 대한 반응
('comment:free:102', 'document:free:1', 'emoji:thumbsup', 2),

-- 게시글 4의 댓글 104에 대한 반응
('comment:free:104', 'document:free:4', 'emoji:heart', 5),
('comment:free:104', 'document:free:4', 'emoji:thumbsup', 3),

-- 게시글 4의 댓글 105에 대한 반응
('comment:free:105', 'document:free:4', 'emoji:laughing', 2),

-- 게시글 4의 댓글 106에 대한 반응
('comment:free:106', 'document:free:4', 'emoji:thumbsup', 4),
('comment:free:106', 'document:free:4', 'emoji:clap', 2);

-- -----------------------------------------------------------------------------
-- Reaction Choose Seed Data (사용자별 반응 선택 기록)
-- -----------------------------------------------------------------------------
INSERT IGNORE INTO g5_da_reaction_choose (member_id, target_id, parent_id, reaction, chosen_ip) VALUES
-- 댓글 101에 대한 반응들
('test1', 'comment:free:101', 'document:free:1', 'emoji:thumbsup', '127.0.0.1'),
('test2', 'comment:free:101', 'document:free:1', 'emoji:thumbsup', '127.0.0.1'),
('test3', 'comment:free:101', 'document:free:1', 'emoji:thumbsup', '127.0.0.1'),
('power1', 'comment:free:101', 'document:free:1', 'emoji:heart', '127.0.0.1'),
('power2', 'comment:free:101', 'document:free:1', 'emoji:heart', '127.0.0.1'),
('admin', 'comment:free:101', 'document:free:1', 'emoji:laughing', '127.0.0.1'),

-- 댓글 102에 대한 반응들
('test1', 'comment:free:102', 'document:free:1', 'emoji:thumbsup', '127.0.0.1'),
('admin', 'comment:free:102', 'document:free:1', 'emoji:thumbsup', '127.0.0.1'),

-- 댓글 104에 대한 반응들
('test1', 'comment:free:104', 'document:free:4', 'emoji:heart', '127.0.0.1'),
('test2', 'comment:free:104', 'document:free:4', 'emoji:heart', '127.0.0.1'),
('test3', 'comment:free:104', 'document:free:4', 'emoji:heart', '127.0.0.1'),
('power1', 'comment:free:104', 'document:free:4', 'emoji:heart', '127.0.0.1'),
('power2', 'comment:free:104', 'document:free:4', 'emoji:heart', '127.0.0.1'),
('test1', 'comment:free:104', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('test2', 'comment:free:104', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('admin', 'comment:free:104', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),

-- 댓글 105에 대한 반응들
('test1', 'comment:free:105', 'document:free:4', 'emoji:laughing', '127.0.0.1'),
('test2', 'comment:free:105', 'document:free:4', 'emoji:laughing', '127.0.0.1'),

-- 댓글 106에 대한 반응들
('test1', 'comment:free:106', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('test2', 'comment:free:106', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('test3', 'comment:free:106', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('admin', 'comment:free:106', 'document:free:4', 'emoji:thumbsup', '127.0.0.1'),
('power1', 'comment:free:106', 'document:free:4', 'emoji:clap', '127.0.0.1'),
('power2', 'comment:free:106', 'document:free:4', 'emoji:clap', '127.0.0.1');

-- -----------------------------------------------------------------------------
-- Report Seed Data (신고 테스트 데이터)
-- -----------------------------------------------------------------------------
INSERT IGNORE INTO g5_singo (sg_table, sg_parent, mb_id, target_mb_id, sg_reason, sg_status, sg_datetime) VALUES
-- 대기 중인 신고
('write_free', 101, 'test3', 'test1', '부적절한 댓글입니다.', 'pending', NOW()),
('write_free', 102, 'test4', 'test2', '광고성 댓글이 의심됩니다.', 'pending', DATE_SUB(NOW(), INTERVAL 1 HOUR)),

-- 검토 중인 신고
('write_qa', 101, 'test1', 'power1', '잘못된 정보를 전달하고 있습니다.', 'monitoring', DATE_SUB(NOW(), INTERVAL 2 HOUR)),

-- 처리 완료된 신고 (승인)
('write_free', 3, 'admin', 'test5', '스팸 게시글', 'approved', DATE_SUB(NOW(), INTERVAL 1 DAY)),

-- 처리 완료된 신고 (기각)
('write_notice', 1, 'test2', 'admin', '테스트 신고 (기각됨)', 'dismissed', DATE_SUB(NOW(), INTERVAL 2 DAY));

-- 처리된 신고에 처리자 정보 업데이트
UPDATE g5_singo SET sg_processed_by = 'admin', sg_processed_at = NOW() WHERE sg_status IN ('approved', 'dismissed');
