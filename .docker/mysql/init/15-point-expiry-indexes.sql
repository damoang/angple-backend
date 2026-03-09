-- Point expiry indexes for FIFO consumption and batch expiry processing

-- FIFO consumption: find active credit entries ordered by expire date
-- Used in GnuboardPointWriteRepository.AddPoint (negative points)
CREATE INDEX idx_g5_point_fifo ON g5_point (po_mb_id, po_expired, po_point, po_expire_date, po_id);

-- Batch expiry: find entries past their expiry date
-- Used in GnuboardPointWriteRepository.ExpireBatch (cron)
CREATE INDEX idx_g5_point_expiry ON g5_point (po_expired, po_point, po_expire_date);

-- Normalize existing data: set empty/null expire dates to permanent (9999-12-31)
UPDATE g5_point SET po_expire_date = '9999-12-31'
WHERE po_expire_date IS NULL OR po_expire_date = '' OR po_expire_date = '0000-00-00';

-- Normalize existing data: set NULL po_expired to 0
UPDATE g5_point SET po_expired = 0 WHERE po_expired IS NULL;
