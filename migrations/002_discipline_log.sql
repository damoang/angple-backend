-- ============================================================
-- Discipline Log Table Migration
-- ============================================================
-- 이용제한 기록을 저장하는 테이블
-- PHP disciplinelog 게시판 데이터 구조를 정규화된 형태로 저장
-- ============================================================

CREATE TABLE IF NOT EXISTS v2_discipline_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    member_id VARCHAR(50) NOT NULL COMMENT '제재 대상 회원 ID',
    member_nickname VARCHAR(100) NOT NULL COMMENT '제재 대상 닉네임',
    penalty_period INT NOT NULL DEFAULT 0 COMMENT '제재 기간 (-1: 영구, 0: 주의, >0: 일수)',
    penalty_date_from DATETIME NOT NULL COMMENT '제재 시작일',
    penalty_date_to DATETIME NULL COMMENT '제재 종료일 (영구/주의는 NULL)',
    violation_types JSON NOT NULL COMMENT '위반 유형 코드 배열 [1, 3, 15]',
    reported_items JSON NULL COMMENT '신고된 글/댓글 목록',
    created_by VARCHAR(50) NOT NULL COMMENT '제재 생성 관리자 ID',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status ENUM('pending', 'approved', 'rejected') NOT NULL DEFAULT 'approved',
    INDEX idx_member_id (member_id),
    INDEX idx_penalty_date_from (penalty_date_from),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='이용제한 기록';

-- ============================================================
-- 기존 PHP disciplinelog 데이터 마이그레이션 (선택적)
-- g5_write_disciplinelog 테이블에서 wr_content JSON 파싱
-- ============================================================
-- 주의: 이 마이그레이션은 PHP JSON 구조에 의존합니다.
-- 실제 환경에서는 Go 코드로 마이그레이션하는 것을 권장합니다.
--
-- PHP wr_content JSON 구조:
-- {
--   "penalty_mb_id": "user123",
--   "penalty_period": 7,
--   "penalty_date_from": "2024-01-01 00:00:00",
--   "sg_types": [1, 3, 15],
--   "reported_items": [{"table": "free", "id": 123, "parent": 0}]
-- }
-- ============================================================
