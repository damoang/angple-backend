-- 게시판 구독 알림 단계화 (#12607)
-- level: 1=전체(모든 글, 1:1), 2=인기글만(추천 임계값 도달 시 1회)
-- 자유게시판(free) 폭주(최근7일 subscribe 알림 38만건, 99%+) → 인기글만으로 전환.

ALTER TABLE g5_board_subscribe
  ADD COLUMN level TINYINT NOT NULL DEFAULT 1 AFTER bo_table;

-- 게시판 구독 알림 토글을 회원 팔로우(noti_follow)와 분리
ALTER TABLE g5_noti_preference
  ADD COLUMN noti_board_subscribe TINYINT(1) NOT NULL DEFAULT 1 AFTER noti_follow;

-- 인기글 알림 중복 방지 (인기 트리거 cron 이 글당 1회만 알림)
CREATE TABLE IF NOT EXISTS g5_board_subscribe_notified (
  bo_table    VARCHAR(20)  NOT NULL,
  wr_id       INT          NOT NULL,
  notified_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (bo_table, wr_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 기존 자유게시판 구독자 → 인기글만 (폭주 해소). 나머지 보드는 전체(1:1) 유지.
UPDATE g5_board_subscribe SET level = 2 WHERE bo_table = 'free';
