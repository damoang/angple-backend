-- Commerce Plugin Review Migration Rollback
-- Version: 3

-- 1. 리뷰 도움됨 테이블 삭제
DROP TABLE IF EXISTS commerce_review_helpfuls;

-- 2. 리뷰 테이블 삭제
DROP TABLE IF EXISTS commerce_reviews;
