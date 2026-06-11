-- rollback: 게시판 구독 단계화 (#12607)
DROP TABLE IF EXISTS g5_board_subscribe_notified;
ALTER TABLE g5_noti_preference DROP COLUMN noti_board_subscribe;
ALTER TABLE g5_board_subscribe DROP COLUMN level;
