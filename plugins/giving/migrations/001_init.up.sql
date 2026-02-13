-- 나눔 플러그인 초기 스키마
-- Giving Plugin Initial Schema
-- 참고: g5_write_giving, g5_giving_bid, g5_giving_bid_numbers 테이블은 기존에 존재.
-- 이 마이그레이션은 인덱스 보완 및 향후 확장을 위한 것.

-- g5_giving_bid 테이블 인덱스 보완
CREATE INDEX IF NOT EXISTS idx_giving_bid_wr_id_status
    ON g5_giving_bid (wr_id, bid_status);

CREATE INDEX IF NOT EXISTS idx_giving_bid_mb_id_status
    ON g5_giving_bid (mb_id, bid_status);

CREATE INDEX IF NOT EXISTS idx_giving_bid_wr_mb_status
    ON g5_giving_bid (wr_id, mb_id, bid_status);

-- g5_giving_bid_numbers 테이블 인덱스 보완
CREATE INDEX IF NOT EXISTS idx_giving_bid_numbers_wr_status
    ON g5_giving_bid_numbers (wr_id, bid_status);

CREATE INDEX IF NOT EXISTS idx_giving_bid_numbers_wr_mb_status
    ON g5_giving_bid_numbers (wr_id, mb_id, bid_status);

CREATE INDEX IF NOT EXISTS idx_giving_bid_numbers_bid_id
    ON g5_giving_bid_numbers (bid_id);

-- g5_write_giving 상태 관련 인덱스 보완
CREATE INDEX IF NOT EXISTS idx_write_giving_status
    ON g5_write_giving (wr_is_comment, wr_5, wr_7);
