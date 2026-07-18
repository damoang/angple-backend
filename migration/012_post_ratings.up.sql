-- 012_post_ratings: 게시글 별점 시스템 (앙티티 — features.rating 보드)
-- 작품 글 1개에 회원들이 ★1~5 투표. 회원당 1표(재투표=UPDATE)를 복합 PK로 보장.
-- ⛔ prod 는 이 DDL 수동 선행 후 코드 배포 (AutoMigrate 는 dev 편의용).
CREATE TABLE IF NOT EXISTS angple_post_ratings (
  bo_table   VARCHAR(20) NOT NULL,
  wr_id      INT         NOT NULL,
  mb_id      VARCHAR(20) NOT NULL,
  rating     TINYINT     NOT NULL,
  created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (bo_table, wr_id, mb_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
