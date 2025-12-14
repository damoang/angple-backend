-- Angple Menu Seed Data
-- 차세대 다모앙 메뉴 시드 데이터
-- Bootstrap Icons → Lucide Icons 매핑 적용

SET NAMES utf8mb4;

-- ============================================================
-- 메뉴 데이터 삽입
-- ============================================================

-- 1. 커뮤니티 (Community) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(1, NULL, '커뮤니티', '/community', 'MessageSquare', '', '커뮤니티', 1, 1, TRUE, '_self', 1, TRUE, TRUE);

-- 1-1. 커뮤니티 하위 메뉴
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(2, 1, '자유게시판', '/free', 'CircleStar', 'F', '자유게시판', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(3, 1, '질문과 답변', '/qa', 'CircleHelp', 'Q', '질문과 답변', 2, 2, TRUE, '_self', 1, FALSE, TRUE),
(4, 1, '갤러리', '/gallery', 'Images', 'G', '갤러리', 2, 3, TRUE, '_self', 1, FALSE, TRUE);

-- 2. 소모임 (Groups) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(10, NULL, '소모임', '/groups', 'Users', '', '소모임', 1, 2, TRUE, '_self', 1, TRUE, TRUE);

-- 2-1. 소모임 상위 카테고리
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(11, 10, '소모임 모아보기', '/groups/all', '', '', '소모임 모아보기', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(12, 10, '소모임 목록', '/groups/list', 'Users', '', '소모임 목록', 2, 2, TRUE, '_self', 1, FALSE, TRUE);

-- 2-2. 소모임 하위 (주요 소모임만 샘플로)
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(13, 12, 'AI당', '/groups/ai', 'Cpu', '', 'AI당', 3, 1, TRUE, '_self', 1, FALSE, TRUE),
(14, 12, '개발한당', '/groups/development', 'Code', '', '개발한당', 3, 2, TRUE, '_self', 1, FALSE, TRUE),
(15, 12, '게임한당', '/groups/game', 'Gamepad2', '', '게임한당', 3, 3, TRUE, '_self', 1, FALSE, TRUE),
(16, 12, '공부한당', '/groups/study', 'BookOpen', '', '공부한당', 3, 4, TRUE, '_self', 1, FALSE, TRUE),
(17, 12, '애플모앙', '/groups/apple', 'Apple', '', '애플모앙', 3, 5, TRUE, '_self', 1, FALSE, TRUE);

-- 3. 새로운 소식 (News) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(20, NULL, '새로운 소식', '/news', 'Newspaper', 'N', '새로운 소식', 1, 3, TRUE, '_self', 1, TRUE, TRUE);

-- 3-1. 새로운 소식 하위
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(21, 20, '사용기', '/tutorial', 'PenTool', 'T', '사용기', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(22, 20, '강좌/팁', '/lecture', 'Lightbulb', 'L', '강좌/팁', 2, 2, TRUE, '_self', 1, FALSE, TRUE),
(23, 20, '자료실', '/pds', 'FolderOpen', 'P', '자료실', 2, 3, TRUE, '_self', 1, FALSE, TRUE);

-- 4. 리뷰 (Reviews) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(30, NULL, '리뷰', '/reviews', 'Gift', '', '리뷰', 1, 4, TRUE, '_self', 1, FALSE, TRUE);

-- 4-1. 리뷰 하위
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(31, 30, '앙지도', '/reviews/map', 'MapPin', 'M', '다모앙 지도', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(32, 30, '앙티티', '/reviews/rating', 'Star', 'O', '다모앙 평점', 2, 2, TRUE, '_self', 1, FALSE, TRUE),
(33, 30, '수익링크', '/reviews/referral', 'TrendingUp', '', '수익링크', 2, 3, TRUE, '_self', 1, FALSE, TRUE);

-- 5. 쇼핑/경제 (Shopping/Economy) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(40, NULL, '알뜰구매', '/economy', 'ShoppingCart', 'E', '알뜰구매', 1, 5, TRUE, '_self', 1, FALSE, TRUE);

-- 5-1. 쇼핑/경제 하위
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(41, 40, '직접홍보', '/economy/promotion', 'Megaphone', 'W', '직접홍보', 2, 1, TRUE, '_self', 1, FALSE, TRUE);

-- 6. 베타 기능 (Beta Features) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(50, NULL, 'BETA', '/beta', 'Sparkles', '', 'BETA 기능', 1, 6, TRUE, '_self', 1, FALSE, TRUE);

-- 6-1. 베타 기능 하위
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(51, 50, '다모앙 레시피', '/beta/recipe', 'Coffee', '', '다모앙 레시피', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(52, 50, '다모앙 전문가', '/beta/expert', 'UserCheck', '', '다모앙 전문가', 2, 2, TRUE, '_self', 1, FALSE, TRUE),
(53, 50, '다모앙 음악', '/beta/music', 'Music', 'D', '다모앙 음악', 2, 3, TRUE, '_self', 1, FALSE, TRUE);

-- 7. 도움말/안내 (Help/Info) - 루트
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(60, NULL, '안내', '/info', 'Info', '', '안내', 1, 7, TRUE, '_self', 1, FALSE, TRUE);

-- 7-1. 안내 하위
INSERT INTO `menus` (`id`, `parent_id`, `title`, `url`, `icon`, `shortcut`, `description`, `depth`, `order_num`, `is_active`, `target`, `view_level`, `show_in_header`, `show_in_sidebar`)
VALUES
(61, 60, '게시판 안내', '/info/board', 'HelpCircle', '', '게시판 안내', 2, 1, TRUE, '_self', 1, FALSE, TRUE),
(62, 60, '위키앙', 'https://wikiang.wiki', 'BookText', '', '위키앙', 2, 2, TRUE, '_blank', 1, FALSE, TRUE);

-- AUTO_INCREMENT 재설정
ALTER TABLE `menus` AUTO_INCREMENT = 100;
