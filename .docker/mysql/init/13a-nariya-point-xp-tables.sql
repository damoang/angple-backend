-- 포인트·경험치 원장 테이블 (운영 스키마 SHOW CREATE TABLE 기준, 2026-10-12)
-- 14-xp-indexes·15-point-expiry-indexes 가 이 테이블을 전제하는데 개발 DB 초기화에 없어서
-- 부하 테스트용 MySQL 이 ERROR 1146 으로 죽었다.
CREATE TABLE IF NOT EXISTS `g5_na_xp` (
  `xp_id` int NOT NULL AUTO_INCREMENT,
  `mb_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xp_datetime` datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  `xp_content` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xp_point` int NOT NULL DEFAULT '0',
  `xp_rel_table` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xp_rel_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `xp_rel_action` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`xp_id`),
  KEY `index1` (`mb_id`,`xp_rel_table`,`xp_rel_id`,`xp_rel_action`),
  KEY `ix_g5_na_xp_2` (`xp_rel_table`,`xp_rel_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `g5_point` (
  `po_id` int NOT NULL AUTO_INCREMENT,
  `mb_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `po_datetime` datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  `po_content` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `po_point` int NOT NULL DEFAULT '0',
  `po_use_point` int NOT NULL DEFAULT '0',
  `po_expired` tinyint NOT NULL DEFAULT '0',
  `po_expire_date` date NOT NULL DEFAULT '0000-00-00',
  `po_mb_point` int NOT NULL DEFAULT '0',
  `po_rel_table` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `po_rel_id` varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `po_rel_action` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  PRIMARY KEY (`po_id`),
  KEY `index1` (`mb_id`,`po_rel_table`,`po_rel_id`,`po_rel_action`),
  KEY `index2` (`po_expire_date`),
  KEY `ix1_g5_point` (`mb_id`,`po_expire_date`,`po_expired`),
  KEY `idx_lucky_display` (`po_rel_table`,`po_rel_id`,`po_rel_action`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
