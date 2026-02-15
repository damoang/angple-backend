-- =============================================================================
-- 테스트 회원 Seed 데이터
-- 비밀번호: test1234 (SHA1 해시)
-- =============================================================================

-- 테스트 계정이 없을 경우에만 삽입
INSERT IGNORE INTO g5_member (
    mb_id, mb_password, mb_name, mb_nick, mb_email,
    mb_level, mb_point, mb_datetime, mb_ip, mb_email_certify
) VALUES
-- 관리자 계정
('admin', SHA1('test1234'), '관리자', '관리자', 'admin@test.local',
 10, 10000, NOW(), '127.0.0.1', NOW()),

-- 일반 테스트 계정 (레벨 2)
('test1', SHA1('test1234'), '테스트1', '테스트1', 'test1@test.local',
 2, 1000, NOW(), '127.0.0.1', NOW()),
('test2', SHA1('test1234'), '테스트2', '테스트2', 'test2@test.local',
 2, 1000, NOW(), '127.0.0.1', NOW()),
('test3', SHA1('test1234'), '테스트3', '테스트3', 'test3@test.local',
 2, 1000, NOW(), '127.0.0.1', NOW()),
('test4', SHA1('test1234'), '테스트4', '테스트4', 'test4@test.local',
 2, 1000, NOW(), '127.0.0.1', NOW()),
('test5', SHA1('test1234'), '테스트5', '테스트5', 'test5@test.local',
 2, 1000, NOW(), '127.0.0.1', NOW()),

-- 고레벨 테스트 계정 (레벨 5)
('power1', SHA1('test1234'), '파워유저1', '파워유저1', 'power1@test.local',
 5, 5000, NOW(), '127.0.0.1', NOW()),
('power2', SHA1('test1234'), '파워유저2', '파워유저2', 'power2@test.local',
 5, 5000, NOW(), '127.0.0.1', NOW()),

-- 부관리자 계정 (레벨 8)
('moderator', SHA1('test1234'), '부관리자', '부관리자', 'mod@test.local',
 8, 8000, NOW(), '127.0.0.1', NOW());

-- 기존 테스트 계정 비밀번호 업데이트 (이미 존재하는 경우)
UPDATE g5_member SET mb_password = SHA1('test1234') WHERE mb_id IN
('admin', 'test1', 'test2', 'test3', 'test4', 'test5', 'test6', 'test7', 'test8', 'test9');
