-- auth_domain_groups: cookie domain 정책 멀티 테넌트 매핑
-- backend cookieDomain(host) 가 5분 cache 로 lookup
-- 미등록 host 는 host-only fallback (가장 안전)

CREATE TABLE IF NOT EXISTS auth_domain_groups (
    suffix VARCHAR(255) NOT NULL PRIMARY KEY,            -- 예: 'damoang.net', 'church.re.kr'
    cookie_domain VARCHAR(255) NOT NULL DEFAULT '',      -- 예: '.damoang.net' (host-only 일 때 '')
    description VARCHAR(255) DEFAULT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- seed: SSO 활성 도메인 그룹만 등록
-- damoang.net 멀티 서브도메인 (ww1, www, m 등) → .damoang.net 공유
-- church.re.kr 교회별 서브도메인 (각 교회 사이트) → .church.re.kr 공유
-- muzia.net / muzia.io / hdbc.kr 미등록 → host-only (자동 fix, 자동 재로그인 방지)
INSERT INTO auth_domain_groups (suffix, cookie_domain, description) VALUES
  ('damoang.net', '.damoang.net', '다모앙 멀티 서브도메인 SSO (ww1, www, m 등)'),
  ('church.re.kr', '.church.re.kr', '처치레 교회별 서브도메인 SSO')
ON DUPLICATE KEY UPDATE updated_at = NOW();
