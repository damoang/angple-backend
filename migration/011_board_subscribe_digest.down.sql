-- rollback: 다이제스트 커서 테이블 제거. level=3 구독자는 알림이 멈출 뿐 데이터 손상 없음.
DROP TABLE IF EXISTS g5_board_subscribe_digest_cursor;
