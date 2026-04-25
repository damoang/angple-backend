-- ============================================================
-- Angple Sites builder content (M1 A1 PoC, issue #1288)
-- ============================================================
-- 작성: 2026-04-25
-- 적용: 새벽 4시 + canary 절차 준수 (메모리 production-guardrails)
-- DDL 통보 채널: damoang/angple#1224 댓글 (E15 합의)
-- prefix 합의: angple_*  (#1224 4316951688)
--
-- PoC 시점 결정 (Sprint Contract §6 R4):
--   - site_id FK 미설정 — angple `sites` 테이블이 prod 미존재
--   - Phase 2 에서 `sites` 마이그 후 FK 추가
-- ============================================================

CREATE TABLE IF NOT EXISTS angple_site_content (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    site_id         BIGINT UNSIGNED NOT NULL                   COMMENT 'angple sites.id (FK 미설정 — Phase 2)',
    content_key     VARCHAR(64)     NOT NULL                   COMMENT 'home, about, _header, _footer 등',
    schema_version  SMALLINT UNSIGNED NOT NULL DEFAULT 1       COMMENT 'blocks JSON schema 버전 (현재 1)',
    blocks          JSON            NOT NULL                   COMMENT '{schema_version, blocks: [{id, type, data, meta}]}',
    meta            JSON            NULL                       COMMENT 'updated_by, lang, tags 등 부가 메타',
    created_at      TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uniq_site_key (site_id, content_key),
    INDEX idx_site (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Angple Sites builder block-based content (RFC: 2026-04-25-builder-rfc.md §3-1)';

-- 검증 쿼리 (수동 확인용)
-- SHOW CREATE TABLE angple_site_content\G
-- SELECT COUNT(*) FROM angple_site_content;  -- 신규 테이블, 0 row 예상
