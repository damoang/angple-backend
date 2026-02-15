-- 게시글/댓글 추천/비추천 테이블
-- UNIQUE KEY: (bo_table, wr_id, mb_id) — 한 사용자가 같은 글에 하나의 액션만 가능
CREATE TABLE IF NOT EXISTS `g5_board_good` (
  `bg_id` int(11) NOT NULL AUTO_INCREMENT,
  `bo_table` varchar(20) NOT NULL DEFAULT '',
  `wr_id` int(11) NOT NULL DEFAULT '0',
  `mb_id` varchar(20) NOT NULL DEFAULT '',
  `bg_flag` varchar(255) NOT NULL DEFAULT '',
  `bg_datetime` datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  `bg_ip` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`bg_id`),
  UNIQUE KEY `fkey1` (`bo_table`, `wr_id`, `mb_id`),
  KEY `idx_member` (`mb_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
