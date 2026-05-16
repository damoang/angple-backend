-- sites: 통합 multi-tenant SaaS 식별 테이블 (WordPress wp_blogs / Discourse Multisite / Ghost sites 패턴)
-- Phase 8 Day 1: church_sites + auth_domain_groups + hostname 하드코딩 을 단일 source of truth 로 통합
--
-- 점진 마이그레이션 전략:
--   Day 2 — backend cookieDomain(host) 가 sites 우선, auth_domain_groups fallback (dual-read)
--   Day 3 — super-admin/sites UI 가 sites 사용
--   Day 4 — frontend getSiteByHost() helper + locals.site 주입
--   Day 5 — church_sites ↔ sites 매핑
--   Day 6 — auth_domain_groups deprecated (sites.cookie_domain 단일 source)
--   Day 7 — regression

CREATE TABLE IF NOT EXISTS sites (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    slug VARCHAR(64) NOT NULL UNIQUE COMMENT 'URL-safe identifier: muzia, damoang, churchre, hdbc',
    primary_host VARCHAR(255) NOT NULL UNIQUE COMMENT 'Canonical hostname (muzia.net)',
    aliases JSON DEFAULT NULL COMMENT 'Alternative hostnames: ["muzia.io","www.muzia.net"]',
    site_type ENUM('saas','tenant') NOT NULL DEFAULT 'saas'
        COMMENT 'saas=root site, tenant=sub-site (churchre 안 각 교회)',
    parent_site_id BIGINT DEFAULT NULL
        COMMENT 'tenant 사이트의 부모 (hopechurch → churchre.id)',
    cookie_domain VARCHAR(255) NOT NULL DEFAULT ''
        COMMENT '".damoang.net" 멀티 서브도메인 SSO 또는 빈 문자열 (host-only)',
    theme_slug VARCHAR(64) NOT NULL DEFAULT 'default'
        COMMENT '활성 테마: muzia, churchre, damoang, default',
    plan ENUM('free','pro','enterprise','internal') NOT NULL DEFAULT 'internal'
        COMMENT '구독 플랜 (Phase 8.4 에서 구독 결제 연동)',
    status ENUM('active','suspended','archived') NOT NULL DEFAULT 'active',
    settings JSON DEFAULT NULL
        COMMENT '사이트별 설정: {logo_url, brand_color, csp_extra, features: {market:true, arrange:false}, ...}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_primary_host (primary_host),
    INDEX idx_parent (parent_site_id),
    INDEX idx_status_type (status, site_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed: 현재 운영 중인 4 root SaaS 사이트
-- (churchre tenants 는 Day 5 에 church_sites 에서 migrate)
INSERT INTO sites (slug, primary_host, aliases, site_type, cookie_domain, theme_slug, plan, settings) VALUES
  ('damoang', 'damoang.net',
   JSON_ARRAY('www.damoang.net','ww1.damoang.net','m.damoang.net'),
   'saas', '.damoang.net', 'damoang', 'internal',
   JSON_OBJECT('description','다모앙 커뮤니티 — 멀티 서브도메인 SSO')),

  ('muzia', 'muzia.net',
   JSON_ARRAY('www.muzia.net','muzia.io'),
   'saas', '', 'muzia', 'internal',
   JSON_OBJECT('description','뮤지아 — 한국 음악 커뮤니티 (2002~), host-only cookie',
               'features', JSON_OBJECT('market', true, 'arrange', true, 'tools', true))),

  ('churchre', 'church.re.kr',
   JSON_ARRAY('www.church.re.kr'),
   'saas', '.church.re.kr', 'churchre', 'internal',
   JSON_OBJECT('description','처치레 — 교회 SaaS root (각 교회는 *.church.re.kr tenant)')),

  ('hdbc', 'hdbc.kr',
   JSON_ARRAY('www.hdbc.kr'),
   'saas', '', 'default', 'internal',
   JSON_OBJECT('description','hdbc.kr — host-only cookie'))
ON DUPLICATE KEY UPDATE updated_at = NOW();
