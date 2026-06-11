-- #12607: 인기글 트리거 cron 첫 실행 시 기존 인기글 소급 통지(백필 폭주) 방지.
-- 실측: free 최근 7일 wr_good>=10 글 2,346건 × level=2 구독자 130명 ≈ 30만 알림이
-- cron 첫 run 에 한 번에 발사될 위험. 배포 시점 이전 free 글을 '통지 완료'로 선마킹해,
-- cron 이 배포 이후 신규로 임계값을 넘긴 글만 통지하도록 한다.
-- cron 기본 window=3일이며 30일 버퍼로 선마킹(window 확대 대비). free 외 보드는
-- 현재 level=2 구독자가 없어(자유게시판만 인기글만 정책) 대상 아님.
INSERT IGNORE INTO g5_board_subscribe_notified (bo_table, wr_id)
SELECT 'free', wr_id
FROM g5_write_free
WHERE wr_is_comment = 0
  AND wr_datetime >= DATE_SUB(NOW(), INTERVAL 30 DAY);
