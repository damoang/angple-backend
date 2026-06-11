-- 선마킹 데이터 제거. g5_board_subscribe_notified 는 통지 중복방지용 상태 테이블이라
-- (사용자 가치 없음) free 마커 전체 삭제해도 무해(최악: 재통지 가능).
-- 단, 009.down 이 테이블 자체를 DROP 하므로 보통은 그쪽에서 정리됨.
DELETE FROM g5_board_subscribe_notified WHERE bo_table = 'free';
