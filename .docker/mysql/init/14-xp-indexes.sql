-- g5_na_xp 인덱스 추가
-- HasTodayAction 쿼리: WHERE mb_id = ? AND xp_rel_action = ? AND xp_datetime >= ? AND xp_datetime < ?
-- DATE() 함수 대신 범위 쿼리 사용으로 인덱스 활용 가능
CREATE INDEX IF NOT EXISTS idx_mb_action_date ON g5_na_xp (mb_id, xp_rel_action, xp_datetime);

-- GetHistory 쿼리: WHERE mb_id = ? ORDER BY xp_datetime DESC
-- 기존 mb_id 단일 인덱스 대신 복합 인덱스로 filesort 방지
CREATE INDEX IF NOT EXISTS idx_mb_datetime ON g5_na_xp (mb_id, xp_datetime DESC);
