-- 게시판 구독 알림 다이제스트(요약, level=3) — #12607 P1
-- level: 1=전체(글마다), 2=인기글만(추천 임계 1회), 3=요약(주기적으로 새 글 묶음 1건)
-- level 컬럼은 009 에서 TINYINT 로 이미 존재 → 값(3)만 확장, DDL 불필요.

-- 게시판별 마지막 다이제스트 커서. last_wr_id 초과 글만 다음 run 에 요약.
-- 커서 행이 없는 보드는 digest cron 이 첫 run 에 "현재 MAX(wr_id) 로 초기화 + 통지 0"
-- 으로 자가 시드한다(배포 시점 이전 글 소급 폭발 방지 — 010 preseed 와 동일 사상).
CREATE TABLE IF NOT EXISTS g5_board_subscribe_digest_cursor (
  bo_table    VARCHAR(20) NOT NULL,
  last_wr_id  INT         NOT NULL DEFAULT 0,
  updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bo_table)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
