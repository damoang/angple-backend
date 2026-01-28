-- Commerce Plugin Coupon Migration Rollback
-- Version: 2

-- 1. 주문 테이블에서 쿠폰 컬럼 제거
ALTER TABLE commerce_orders
DROP INDEX idx_coupon,
DROP COLUMN coupon_code,
DROP COLUMN coupon_id;

-- 2. 쿠폰 사용 내역 테이블 삭제
DROP TABLE IF EXISTS commerce_coupon_usages;

-- 3. 쿠폰 테이블 삭제
DROP TABLE IF EXISTS commerce_coupons;
