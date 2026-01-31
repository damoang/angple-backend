-- =============================================================================
-- Feature Tables for Memo, Reaction, Report (v1 API)
-- =============================================================================

-- -----------------------------------------------------------------------------
-- Member Memo Table (회원 간 메모)
-- 회원이 다른 회원에 대해 개인적으로 메모를 남기는 기능
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS g5_member_memo (
    id INT(11) NOT NULL AUTO_INCREMENT,
    member_uid INT(11) NOT NULL COMMENT '작성자 UID (g5_member.mb_no)',
    member_id VARCHAR(50) NOT NULL COMMENT '작성자 ID',
    target_member_uid INT(11) NOT NULL COMMENT '대상 회원 UID',
    target_member_id VARCHAR(50) NOT NULL COMMENT '대상 회원 ID',
    memo VARCHAR(255) NOT NULL DEFAULT '' COMMENT '짧은 메모 (요약)',
    memo_detail TEXT COMMENT '상세 메모',
    color VARCHAR(50) NOT NULL DEFAULT 'yellow' COMMENT '메모 색상',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_member_target (member_id, target_member_id),
    KEY idx_member_id (member_id),
    KEY idx_target_member_id (target_member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- Reaction Count Table (반응 집계)
-- 게시글/댓글에 대한 반응(좋아요, 이모지 등)의 개수를 집계
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS g5_da_reaction (
    id INT(11) NOT NULL AUTO_INCREMENT,
    target_id VARCHAR(100) NOT NULL COMMENT '대상 ID (comment:{board_id}:{wr_id})',
    parent_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '부모 ID (document:{board_id}:{wr_id})',
    reaction VARCHAR(50) NOT NULL COMMENT '반응 타입 (emoji:thumbsup, image:heart 등)',
    reaction_count INT(11) NOT NULL DEFAULT 0 COMMENT '반응 수',
    PRIMARY KEY (id),
    UNIQUE KEY uk_target_reaction (target_id, reaction),
    KEY idx_target_id (target_id),
    KEY idx_parent_id (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- Reaction Choose Table (사용자별 반응 선택)
-- 어떤 사용자가 어떤 반응을 선택했는지 기록
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS g5_da_reaction_choose (
    id INT(11) NOT NULL AUTO_INCREMENT,
    member_id VARCHAR(50) NOT NULL COMMENT '회원 ID',
    target_id VARCHAR(100) NOT NULL COMMENT '대상 ID',
    parent_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '부모 ID',
    reaction VARCHAR(50) NOT NULL COMMENT '선택한 반응',
    chosen_ip VARCHAR(50) NOT NULL DEFAULT '' COMMENT 'IP 주소',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_member_target_reaction (member_id, target_id, reaction),
    KEY idx_member_id (member_id),
    KEY idx_target_id (target_id),
    KEY idx_parent_id (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- -----------------------------------------------------------------------------
-- Report Table (신고)
-- 부적절한 게시글/댓글에 대한 신고 기능
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS g5_singo (
    sg_id INT(11) NOT NULL AUTO_INCREMENT,
    sg_table VARCHAR(50) NOT NULL COMMENT '게시판 테이블명 (write_free 등)',
    sg_parent INT(11) NOT NULL COMMENT '게시글/댓글 ID',
    mb_id VARCHAR(50) NOT NULL COMMENT '신고자 ID',
    target_mb_id VARCHAR(50) NOT NULL DEFAULT '' COMMENT '피신고자 ID',
    sg_reason TEXT COMMENT '신고 사유',
    sg_status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT '상태: pending, monitoring, approved, dismissed',
    sg_datetime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '신고 일시',
    sg_processed_at DATETIME DEFAULT NULL COMMENT '처리 일시',
    sg_processed_by VARCHAR(50) DEFAULT NULL COMMENT '처리자 ID',
    PRIMARY KEY (sg_id),
    KEY idx_sg_table_parent (sg_table, sg_parent),
    KEY idx_mb_id (mb_id),
    KEY idx_target_mb_id (target_mb_id),
    KEY idx_sg_status (sg_status),
    KEY idx_sg_datetime (sg_datetime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
