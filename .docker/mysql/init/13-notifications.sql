-- 알림 테이블
CREATE TABLE IF NOT EXISTS `g5_da_notification` (
  `nt_id` int(11) NOT NULL AUTO_INCREMENT,
  `mb_id` varchar(20) NOT NULL,
  `nt_type` enum('comment','reply','mention','like','message','system') NOT NULL DEFAULT 'system',
  `nt_title` varchar(255) NOT NULL,
  `nt_content` text,
  `nt_url` varchar(500) DEFAULT NULL,
  `nt_sender_id` varchar(20) DEFAULT NULL,
  `nt_sender_name` varchar(255) DEFAULT NULL,
  `nt_is_read` tinyint(1) NOT NULL DEFAULT 0,
  `nt_created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`nt_id`),
  KEY `idx_member` (`mb_id`, `nt_is_read`),
  KEY `idx_created` (`nt_created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
