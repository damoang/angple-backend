-- 013_keyword_alert rollback (코드 롤백 후 실행)
DROP TABLE IF EXISTS g5_member_keyword_alert;
ALTER TABLE g5_noti_preference DROP COLUMN noti_keyword;
