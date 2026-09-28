-- 005_post_status.sql — 글 처리 상태(해결됨·진행중·보류). 카테고리(ca_name)와 독립.
-- 설계: docs/2026-09-28-bug-status-badge-sprint.html (angple web 저장소)
-- ⛔ k3s 는 마이그레이션을 자동 실행하지 않는다 → 라이브 DB 수동 적용(2026-09-28 적용 완료).
-- ⛔ 코드는 테이블이 없어도 배지만 생략하고 동작한다(enrichWithPostStatus fail-open).

CREATE TABLE IF NOT EXISTS g5_da_post_status (
  board_id   VARCHAR(20)  NOT NULL,
  wr_id      INT          NOT NULL,
  status     VARCHAR(16)  NOT NULL,            -- resolved | in_progress | hold
  set_by     VARCHAR(20)  NOT NULL DEFAULT '', -- mb_id 또는 'migration'
  source     VARCHAR(16)  NOT NULL DEFAULT '', -- admin | tool | migration
  note       VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3)  NOT NULL,
  updated_at DATETIME(3)  NOT NULL,
  PRIMARY KEY (board_id, wr_id),
  KEY idx_board_status (board_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
