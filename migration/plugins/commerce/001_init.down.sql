-- Commerce Plugin Rollback Migration
-- Version: 1
-- Description: 커머스 플러그인 테이블 삭제 (역순으로 삭제)

-- 외래키 제약 조건이 있으므로 역순으로 삭제

DROP TABLE IF EXISTS commerce_settings;
DROP TABLE IF EXISTS commerce_settlements;
DROP TABLE IF EXISTS commerce_downloads;
DROP TABLE IF EXISTS commerce_payments;
DROP TABLE IF EXISTS commerce_order_items;
DROP TABLE IF EXISTS commerce_orders;
DROP TABLE IF EXISTS commerce_carts;
DROP TABLE IF EXISTS commerce_product_files;
DROP TABLE IF EXISTS commerce_products;
