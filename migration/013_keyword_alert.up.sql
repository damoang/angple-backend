-- 키워드 알림 — 새 원글 제목에 회원이 등록한 키워드가 들어가면 알림(ph_to_case='keyword')
-- ⛔ 수동 DDL 선행 원칙: 백엔드 배포 **전에** 반영해야 한다.
--    noti_keyword 컬럼이 없으면 알림 설정 저장(Upsert)이 Unknown column 으로 실패한다.

CREATE TABLE IF NOT EXISTS g5_member_keyword_alert (
  id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
  mb_id      VARCHAR(20)  NOT NULL,
  keyword    VARCHAR(30)  NOT NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_mb_keyword (mb_id, keyword)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 키워드 알림 토글(기본 ON)
ALTER TABLE g5_noti_preference
  ADD COLUMN noti_keyword TINYINT(1) NOT NULL DEFAULT 1 AFTER noti_board_subscribe;
