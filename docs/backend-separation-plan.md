# Angple 아키텍처 분석: 백엔드 분리 전략

## 1. 현재 아키텍처 (As-Is)

```
┌─────────────────────────────────────────────────────────────────┐
│                       MySQL RDS (damoang DB)                    │
│                  g5_* 테이블 + v2_* 테이블 혼재                  │
└───────┬───────────────────┬───────────────────┬─────────────────┘
        │                   │                   │
 ┌──────┴──────┐     ┌──────┴──────┐     ┌──────┴──────┐
 │  angple-    │     │  damoang-   │     │  damoang-   │
 │  backend    │     │  ops        │     │  ads        │
 │  (Go:8082)  │     │ (SKit:5175) │     │  (Go:9090)  │
 └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
        │                   │                   │
  ┌─────┴─────┐      ┌─────┴─────┐      ┌─────┴─────┐
  │  angple   │      │ damoang-  │      │ damoang-  │
  │  web/admin│      │   ops     │      │   ads     │
  │ SvelteKit │      │ SvelteKit │      │ SvelteKit │
  └───────────┘      └───────────┘      └───────────┘
```

### nginx 프록시 현황

| 도메인 | upstream | 서비스 |
|--------|----------|--------|
| `api.damoang.net` | `127.0.0.1:8082` | angple-backend |
| `ops.damoang.net /api/*` | `127.0.0.1:8082` | angple-backend (ops가 직접 호출) |
| `ops.damoang.net /ai/*` | `127.0.0.1:8317` | CLIProxyAPI (AI 평가) |
| `ops.damoang.net /*` | `127.0.0.1:5175` | damoang-ops SvelteKit |
| `ads.damoang.net /api/*` | `127.0.0.1:9090` | damoang-ads Go 서버 |

---

## 2. 핵심 문제점

### 2.1 angple-backend 코드 혼재

현재 `angple-backend`에 세 종류의 코드가 섞여 있다:

| 구분 | 파일 수 | 설명 |
|------|---------|------|
| **오픈소스 코어** (v2 API) | ~60 | auth, boards, posts, comments, users, notifications, plugin framework |
| **레거시 호환** (v1 API) | ~40 | g5_* 테이블, DamoangCookieAuth, 그누보드 호환 |
| **다모앙 전용** | ~30 | 광고, 결제, 직홍게, 제재, 다중기, AI 평가 |

### 2.2 3중 DB 접근

```
angple-backend (8082) ──┐
                        ├──→ MySQL RDS (damoang)
damoang-ads    (9090) ──┤
                        │
damoang-ops    (5175) ──┘ (via angple-backend API)
```

- 3개 서비스가 같은 DB에 연결 풀을 각각 유지
- ops는 angple-backend의 API를 nginx로 프록시하여 호출

### 2.3 오픈소스 배포 불가

angple-backend에 다음이 하드코딩/혼재:

- `DamoangSecret` (JWT config)
- `damoang_jwt` 쿠키 인증 (cookie_auth.go)
- 광고 플러그인 (advertising, banner, promotion)
- 결제 시스템 (Toss, Stripe - 다모앙 계정 바인딩)
- 다중기 탐지 (dajoongi)
- 제재 시스템 (discipline)
- AI 콘텐츠 평가 (ai_evaluator.go - 32KB)
- 업로드 경로 기본값: `/home/damoang/www/data/file`

---

## 3. angple-backend 파일별 분류

### 3.1 오픈소스 유지 (Generic Core)

#### Handler (14)
| 파일 | 기능 |
|------|------|
| `auth_handler.go` | JWT 인증 |
| `post_handler.go` | 게시글 CRUD |
| `comment_handler.go` | 댓글 CRUD |
| `board_handler.go` | 게시판 관리 |
| `member_handler.go` | 회원 검증 |
| `menu_handler.go` | 메뉴 관리 |
| `site_handler.go` | 사이트/테넌트 |
| `notification_handler.go` | 알림 |
| `scrap_handler.go` | 스크랩 |
| `block_handler.go` | 회원 차단 |
| `message_handler.go` | 쪽지 |
| `memo_handler.go` | 메모 |
| `reaction_handler.go` | 반응 |
| `file_handler.go` | 파일 업로드 |

#### Handler v2 (8)
| 파일 | 기능 |
|------|------|
| `v2_handler.go` | v2 CRUD |
| `v2_auth_handler.go` | v2 JWT 인증 |
| `admin_handler.go` | v2 관리자 |
| `install_handler.go` | 설치 마법사 |
| `scrap_handler.go` | v2 스크랩 |
| `memo_handler.go` | v2 메모 |
| `block_handler.go` | v2 차단 |
| `message_handler.go` | v2 쪽지 |

#### Middleware (7 - 순수 Generic)
| 파일 | 기능 |
|------|------|
| `auth.go` | JWT 인증 |
| `admin.go` | 관리자 권한 체크 |
| `security.go` | 보안 헤더 |
| `cache.go` | 캐시 미들웨어 |
| `metrics.go` | Prometheus |
| `request_logger.go` | 요청 로깅 |
| `rate_limit.go` | 요청 제한 |

#### Plugin System (순수 Framework)
| 파일 | 기능 |
|------|------|
| `internal/plugin/hook_manager.go` | WordPress 스타일 훅 |
| `internal/plugin/plugin_manager.go` | 플러그인 생명주기 |
| `internal/plugin/registry.go` | 플러그인 레지스트리 |
| `internal/plugin/scheduler.go` | 작업 스케줄링 |
| `internal/pluginstore/` | 플러그인 스토어 전체 |

#### Package (재사용 가능)
| 경로 | 기능 |
|------|------|
| `pkg/jwt/` | JWT 토큰 관리 |
| `pkg/auth/` | 레거시 인증 호환 |
| `pkg/logger/` | 구조화 로깅 |
| `pkg/redis/` | Redis 클라이언트 |
| `pkg/elasticsearch/` | ES 클라이언트 |
| `pkg/storage/` | S3 스토리지 |
| `pkg/cache/` | 캐시 추상화 |
| `pkg/i18n/` | 국제화 |

### 3.2 다모앙 전용 (분리 대상)

#### Handler
| 파일 | 기능 | 이동처 |
|------|------|--------|
| `promotion_handler.go` | 직홍게 | damoang-backend |
| `banner_handler.go` | 배너 광고 | damoang-backend |
| `dajoongi_handler.go` | 다중계정 탐지 | damoang-backend |
| `discipline_handler.go` | 제재 시스템 | damoang-backend |
| `ai_evaluation_handler.go` | AI 콘텐츠 평가 | damoang-backend |
| `report_handler.go` | 신고 시스템 (대부분) | damoang-backend |
| `good_handler.go` | 추천/비추천 (g5 전용) | damoang-backend |
| `gallery_handler.go` | 갤러리 (Redis 캐시) | damoang-backend |
| `recommended_handler.go` | 추천 게시물 (파일 기반) | damoang-backend |
| `admin_handler.go` | 관리자 회원 관리 | damoang-backend |
| `payment_handler.go` | 결제 (Toss/Stripe) | damoang-backend |
| `search_handler.go` | ES 검색 | 판단 필요 |
| `recommendation_handler.go` | AI 추천 | damoang-backend |
| `oauth_handler.go` | OAuth 소셜 로그인 | 판단 필요 |

#### Service
| 파일 | 크기 | 이동처 |
|------|------|--------|
| `ai_evaluator.go` | 32KB | damoang-backend |
| `report_service.go` | 대형 | damoang-backend |
| `promotion_service.go` | | damoang-backend |
| `banner_service.go` | | damoang-backend |
| `discipline_service.go` | | damoang-backend |
| `gallery_service.go` | | damoang-backend |
| `good_service.go` | | damoang-backend |
| `payment_service.go` | | damoang-backend |
| `admin_member_service.go` | | damoang-backend |
| `recommendation_service.go` | | damoang-backend |

#### Domain Model
| 파일 | 테이블 | 이동처 |
|------|--------|--------|
| `promotion.go` | g5_write_* | damoang-backend |
| `banner.go` | 커스텀 | damoang-backend |
| `dajoongi.go` | g5_login | damoang-backend |
| `discipline.go` | g5_write_disciplinelog | damoang-backend |
| `payment.go` | v2_payments | damoang-backend |
| `ai_evaluation.go` | v2_ai_evaluations | damoang-backend |
| `report.go` | g5_write_singo* | damoang-backend |
| `singo_user.go` | v2_singo_users | damoang-backend |
| `singo_setting.go` | v2_singo_settings | damoang-backend |
| `recommendation.go` | v2_recommendations | damoang-backend |

#### Middleware
| 파일 | 이유 | 처리 |
|------|------|------|
| `cookie_auth.go` | `damoang_jwt` 쿠키 전용 | damoang-backend로 이동 |
| `deprecation.go` | v1 전용 | damoang-backend로 이동 |
| `v1_redirect.go` | v1→v2 힌트 | damoang-backend로 이동 |
| `permission.go` | g5 테이블 의존 | 판단 필요 |

#### 내장 플러그인 (전부 다모앙 전용)
| 경로 | 기능 |
|------|------|
| `internal/plugins/advertising/` | 광고 시스템 |
| `internal/plugins/commerce/` | 전자상거래 |
| `internal/plugins/embed/` | 임베딩 |
| `internal/plugins/imagelink/` | 이미지 링크 |
| `internal/plugins/marketplace/` | 마켓플레이스 |
| `plugins/giving/` | 기부/후원 |
| `plugins/banner/` | 배너 |
| `plugins/emoticon/` | 이모티콘 |
| `plugins/promotion/` | 직홍게 |

### 3.3 판단 필요 (Generic 확장 가능)

| 파일 | 현재 | 권장 |
|------|------|------|
| `oauth_handler.go` | 네이버/카카오/구글 하드코딩 | Generic: provider 설정으로 분리 → 오픈소스 유지 |
| `search_handler.go` | ES 통합 검색 | Generic: 오픈소스 유지 (ES 선택적) |
| `media_handler.go` | S3 업로드 | Generic: 오픈소스 유지 (S3 선택적) |
| `tenant_handler.go` | 멀티테넌트 | Generic: 오픈소스 유지 |
| `provisioning_handler.go` | SaaS 프로비저닝 | Generic: 오픈소스 유지 |
| `autosave_handler.go` | 자동 저장 | Generic: 오픈소스 유지 |
| `ws_handler.go` | WebSocket | Generic: 오픈소스 유지 |

---

## 4. 라우트 분류

### 4.1 오픈소스 유지 라우트

```
/api/v2/auth/*                  # JWT 인증
/api/v2/users/*                 # 사용자
/api/v2/boards/*                # 게시판
/api/v2/boards/:slug/posts/*    # 게시글
/api/v2/posts/:id/scrap         # 스크랩
/api/v2/members/:id/memo        # 메모
/api/v2/members/:id/block       # 차단
/api/v2/messages/*              # 쪽지
/api/v2/admin/boards/*          # 관리자 게시판
/api/v2/admin/members/*         # 관리자 회원
/api/v2/admin/plugins/*         # 플러그인 스토어
/api/v2/marketplace/*           # 마켓플레이스
/api/v2/install/*               # 설치 마법사
/api/v2/search/*                # 검색
/api/v2/media/*                 # 미디어
/api/v2/auth/oauth/*            # OAuth
/api/v2/saas/*                  # SaaS
/api/v2/admin/tenants/*         # 테넌트
/api/v2/recommendations/*      # 추천 (범용화 후)
/ws/notifications               # WebSocket
/health                         # 헬스체크
```

### 4.2 다모앙 전용 라우트 (분리 대상)

```
# v1 전체 (그누보드 DB 기반, 2026-08-01 Sunset)
/api/v1/*                       # → damoang-backend

# v2 중 다모앙 전용
/api/v2/reports/*               # 신고 시스템 → damoang-backend
/api/v2/reports/ai-evaluation/* # AI 평가 → damoang-backend
/api/v1/ai-evaluations/*        # AI 평가 v1 → damoang-backend
/api/v2/payments/*              # 결제 → damoang-backend
/api/v2/admin/audit-logs        # 감사 로그 → damoang-backend

# 플러그인 라우트
/api/plugins/promotion/*        # 직홍게 → damoang-backend
/api/plugins/banner/*           # 배너 → damoang-backend
```

---

## 5. 이상적 아키텍처 (To-Be)

```
┌──────────────────────────────────────────────────────────────────────┐
│  Angple Core (MIT 오픈소스)                                          │
│  ┌────────────────┐    ┌────────────────┐                           │
│  │  angple-web     │───▶│  angple-backend │──▶ DB (설치자가 설정)    │
│  │  angple-admin   │    │  v2 API only    │                          │
│  │  SvelteKit      │    │  Go/Gin         │                          │
│  └────────────────┘    └────────────────┘                           │
│  누구나 설치 가능 (WordPress처럼)                                     │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│  다모앙 전용 서비스 (비공개)                                          │
│  ┌────────────────┐    ┌────────────────┐                           │
│  │  damoang-ops   │───▶│                │                           │
│  │  (신고 관리)    │    │  damoang-      │──▶ MySQL RDS (damoang)   │
│  ├────────────────┤    │  backend       │──▶ ClickHouse (analytics) │
│  │  damoang-ads   │───▶│  (통합 백엔드)  │──▶ Redis (cache)         │
│  │  (광고 관리)    │    │                │                           │
│  └────────────────┘    └────────────────┘                           │
│  다모앙닷넷 운영 전용                                                 │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 6. damoang-backend 통합 설계

### 6.1 현재 3개 백엔드 → 1개 통합

| 현재 | 포트 | 기능 |
|------|------|------|
| angple-backend v1 라우트 | 8082 | g5_* CRUD, 신고, 제재, 다중기 |
| damoang-ads 서버 | 9090 | ClickHouse 분석, 배너/광고 관리, 축하배너, 직홍게 |
| ops-api (angple-backend 경유) | 8082 | ops가 angple-backend API를 nginx 프록시로 사용 |

통합 후:

| 통합 | 포트 | 기능 |
|------|------|------|
| **damoang-backend** | 8090 | 위 3개 기능 전부 |

### 6.2 damoang-backend 디렉터리 구조

```
damoang-backend/
├── cmd/
│   └── api/main.go
├── internal/
│   ├── config/
│   │   └── config.go                  # MySQL + ClickHouse + Redis + S3
│   ├── handler/
│   │   ├── auth_handler.go            # damoang_jwt 쿠키 인증
│   │   ├── report_handler.go          ← angple-backend에서 이동
│   │   ├── discipline_handler.go      ← angple-backend에서 이동
│   │   ├── dajoongi_handler.go        ← angple-backend에서 이동
│   │   ├── ai_evaluation_handler.go   ← angple-backend에서 이동
│   │   ├── promotion_handler.go       ← angple-backend에서 이동
│   │   ├── banner_handler.go          ← angple-backend에서 이동
│   │   ├── good_handler.go            ← angple-backend에서 이동
│   │   ├── gallery_handler.go         ← angple-backend에서 이동
│   │   ├── admin_handler.go           ← angple-backend에서 이동
│   │   ├── payment_handler.go         ← angple-backend에서 이동
│   │   ├── recommended_handler.go     ← angple-backend에서 이동
│   │   ├── ads_stats_handler.go       ← damoang-ads에서 이동
│   │   ├── ads_banner_handler.go      ← damoang-ads에서 이동
│   │   ├── ads_adsense_handler.go     ← damoang-ads에서 이동
│   │   ├── ads_celebration_handler.go ← damoang-ads에서 이동
│   │   ├── ads_economy_handler.go     ← damoang-ads에서 이동
│   │   └── ads_serve_handler.go       ← damoang-ads에서 이동
│   ├── service/
│   │   ├── report_service.go
│   │   ├── discipline_service.go
│   │   ├── ai_evaluator.go
│   │   ├── promotion_service.go
│   │   ├── banner_service.go
│   │   ├── gallery_service.go
│   │   ├── good_service.go
│   │   ├── payment_service.go
│   │   └── ...
│   ├── repository/
│   ├── domain/
│   ├── middleware/
│   │   └── cookie_auth.go             # damoang_jwt 쿠키 인증
│   └── plugins/                       # 다모앙 전용 플러그인
│       ├── advertising/
│       ├── commerce/
│       ├── giving/
│       ├── emoticon/
│       └── promotion/
├── configs/
│   └── config.yaml
└── go.mod
```

### 6.3 통합의 장점

| 항목 | 현재 (3개 백엔드) | 통합 후 (1개) |
|------|-------------------|---------------|
| **DB 연결 풀** | 3개 × 각각 max_open_conns | 1개 통합 풀 |
| **배포** | 3개 서비스 각각 | 1개만 배포 |
| **인증** | ops→angple-backend 프록시 | 자체 처리 |
| **코드 중복** | DB 모델, 유틸 중복 | 공유 |
| **nginx 설정** | 3개 upstream | 1개 |
| **모니터링** | 3개 프로세스 | 1개 |

---

## 7. 전환 로드맵

### Phase 1: angple-backend 정리 (오픈소스 준비)

**목표**: angple-backend에서 다모앙 전용 코드 제거

**작업 순서**:

1. **config 정리**
   - `JWTConfig.DamoangSecret` 제거
   - 업로드 경로 기본값에서 `/home/damoang/` 제거
   - CORS 기본값에서 `damoang.net` 제거

2. **middleware 정리**
   - `cookie_auth.go` 제거 (v2 JWT만 유지)
   - `deprecation.go`, `v1_redirect.go` 제거

3. **v1 라우트 전체 제거**
   - `internal/routes/routes.go` → 삭제
   - v1 전용 handler, service, repository, domain 삭제

4. **다모앙 전용 handler 제거**
   - `promotion_handler.go`, `banner_handler.go`, `dajoongi_handler.go`
   - `discipline_handler.go`, `ai_evaluation_handler.go`
   - `report_handler.go`, `good_handler.go`, `gallery_handler.go`
   - `recommended_handler.go`, `admin_handler.go`, `payment_handler.go`

5. **다모앙 전용 플러그인 제거**
   - `internal/plugins/advertising/`, `commerce/`, `embed/`, `imagelink/`, `marketplace/`
   - `plugins/giving/`, `banner/`, `emoticon/`, `promotion/`

6. **main.go 정리**
   - `damoangJWT` 관련 코드 제거
   - `AI_EVAL_*` 환경 변수 제거
   - 플러그인 import 정리

7. **v2 routes 정리**
   - `SetupReports` 제거 (다모앙 전용 DamoangCookieAuth 사용)
   - AI evaluation 라우트 제거
   - Payment 라우트 제거

**결과**: `angple + angple-backend`만으로 커뮤니티 운영 가능

### Phase 2: damoang-backend 생성

**목표**: 다모앙 전용 통합 백엔드

1. **새 Go 모듈 생성**
   - `github.com/damoang/damoang-backend`
   - 비공개 저장소 (Private)

2. **angple-backend에서 코드 이동**
   - Phase 1에서 제거한 모든 파일
   - v1 라우트 전체
   - damoang_jwt 쿠키 인증
   - 그누보드 호환 레이어 (`pkg/auth/legacy.go`)

3. **damoang-ads 코드 흡수**
   - ClickHouse 연결
   - 배너/광고 관리 핸들러
   - AdSense 슬롯 관리
   - 축하배너
   - 경제 게시물 (네이버 API)
   - 프로모션
   - 통계 집계

4. **ops-api 기능 흡수**
   - ops의 angple-backend API 의존 제거
   - 직접 DB 접근으로 전환

5. **통합 인증**
   - `damoang_jwt` 쿠키 + Bearer JWT 모두 지원
   - ops/ads 프론트엔드가 damoang-backend만 호출

### Phase 3: 프론트엔드 API 전환

1. **damoang-ops**
   - nginx 프록시 변경: `/api/*` → `damoang-backend:8090`
   - angple-backend(8082) 의존 제거

2. **damoang-ads**
   - 자체 Go 서버 제거 (damoang-backend로 통합)
   - 프론트엔드만 유지, API는 damoang-backend 호출

3. **angple web/admin**
   - v2 API만 사용 (이미 대부분 v2)
   - v1 호환 라우트 제거

4. **nginx 정리**
   ```nginx
   # BEFORE (3 upstreams)
   upstream angple_api    { server 127.0.0.1:8082; }
   upstream ads_api       { server 127.0.0.1:9090; }
   # ops → angple_api 프록시

   # AFTER (2 upstreams, 역할 명확)
   upstream angple_api    { server 127.0.0.1:8082; }  # 오픈소스 코어
   upstream damoang_api   { server 127.0.0.1:8090; }  # 다모앙 전용
   ```

### Phase 4: Docker 이미지 기반 배포

1. **angple-backend** → GHCR 이미지 (오픈소스, Public)
   - `ghcr.io/angple/angple-backend:latest`
   - 누구나 pull 가능

2. **damoang-backend** → ECR 이미지 (비공개)
   - `xxxx.dkr.ecr.ap-northeast-2.amazonaws.com/damoang-backend:latest`
   - 다모앙 운영팀만 접근

3. **배포 스크립트 통합**
   - 현재: 로컬 빌드 → 바이너리 실행
   - 목표: 이미지 pull → Docker run

---

## 8. 의존성 관계도 (Before/After)

### Before (현재)

```
damoang-ops ──nginx──▶ angple-backend:8082 ──▶ MySQL RDS
                       (v1 + v2 + 다모앙 전용)

damoang-ads ──────────▶ ads-api:9090 ──────▶ MySQL RDS
                                    ──────▶ ClickHouse

angple web ───────────▶ angple-backend:8082 ──▶ MySQL RDS
```

### After (목표)

```
angple web ───────────▶ angple-backend:8082 ──▶ 설치자 DB
                       (v2 only, 오픈소스)

damoang-ops ──────────▶ damoang-backend:8090 ──▶ MySQL RDS
damoang-ads ──────────▶ damoang-backend:8090 ──▶ ClickHouse
                       (v1 + 다모앙 전용)    ──▶ Redis
```

---

## 9. 리스크 및 주의사항

### 9.1 v1 API Sunset (2026-08-01)

현재 v1 API에 Deprecation 헤더가 설정되어 있다:
```
Sunset: Sat, 01 Aug 2026 00:00:00 GMT
```

- v1은 angple-backend에서 제거하되, damoang-backend에서 계속 운영
- 다모앙닷넷에서 v1→v2 전환이 완료될 때까지 damoang-backend가 v1 지원

### 9.2 damoang_jwt 호환성

- angple web은 현재 `damoang_jwt` 쿠키와 Bearer JWT 모두 지원
- 분리 후 angple web은 Bearer JWT만 사용
- damoang-ops, damoang-ads는 damoang-backend의 `damoang_jwt` 사용

### 9.3 DB 마이그레이션 없음

- 이번 분리는 **코드 분리만** 수행
- DB 스키마 변경 없음 (같은 MySQL RDS 계속 사용)
- damoang-backend가 g5_* + v2_* 테이블 모두 접근

### 9.4 Plugin System 분리

- Plugin framework (`internal/plugin/`)는 오픈소스에 유지
- 구체적 플러그인 구현 (advertising, commerce 등)은 damoang-backend로 이동
- damoang-backend는 angple-backend의 Plugin API를 import하지 않음 (독립)

---

## 10. 최종 아키텍처 요약

```
오픈소스 (MIT, GitHub 공개)            비공개 (다모앙 운영용)
┌──────────────────────┐            ┌──────────────────────┐
│  angple (monorepo)   │            │  damoang-ops         │
│  ├── apps/web        │            │  (신고 관리)          │
│  ├── apps/admin      │            ├──────────────────────┤
│  ├── packages/*      │            │  damoang-ads         │
│  ├── themes/         │            │  (광고 관리)          │
│  └── plugins/        │            └──────────┬───────────┘
└──────────┬───────────┘                       │
           │                                   │
┌──────────┴───────────┐            ┌──────────┴───────────┐
│  angple-backend      │            │  damoang-backend      │
│  (오픈소스 코어 API)  │            │  (다모앙 전용 API)    │
│  v2 API only         │            │  v1 호환 + 광고/결제  │
│  Go/Gin              │            │  Go/Gin              │
│  MIT License         │            │  Private             │
└──────────┬───────────┘            └──────────┬───────────┘
           │                                   │
     설치자의 DB                          MySQL RDS
                                        ClickHouse
                                        Redis
```

---

## 부록 A: main.go 의존성 주입 현황

현재 `cmd/api/main.go`에서 생성되는 객체 (DB 연결 시):

- **Repository**: 30개
- **Service**: 28개
- **Handler**: 25개+
- **Plugin import**: 6개 (`advertising`, `commerce`, `embed`, `imagelink`, `marketplace`, `giving`)

분리 후 angple-backend main.go:
- **Repository**: ~15개 (v2 전용)
- **Service**: ~12개
- **Handler**: ~10개
- **Plugin import**: 0개 (플러그인은 외부 설치)

## 부록 B: damoang-ads 라우트 (damoang-backend로 흡수)

```
# Public (인증 불필요)
GET  /api/v1/serve/banners
GET  /api/v1/serve/adsense-slots
GET  /api/v1/serve/celebrations
GET  /api/v1/serve/promotions
GET  /api/v1/serve/promotion-posts

# 인증 필요 (JWT)
GET  /api/v1/stats/period
GET  /api/v1/advertisers
GET  /api/v1/boards
GET  /api/v1/positions
GET  /api/v1/ads
GET  /api/v1/os, /browsers, /devices

# 광고주 셀프서비스
POST/GET/PUT/DELETE  /api/v1/advertisers/:id/banners/*

# Admin (관리자)
CRUD  /api/v1/admin/advertisers/*
CRUD  /api/v1/admin/banners/*
CRUD  /api/v1/admin/promotions/*
CRUD  /api/v1/admin/celebrations/*
CRUD  /api/v1/admin/economy-posts/*
CRUD  /api/v1/admin/adsense-slots/*
CRUD  /api/v1/admin/user-advertisers/*
```

## 부록 C: 공유 인프라

| 서비스 | 호스트 | 포트 | 용도 |
|--------|--------|------|------|
| MySQL RDS | damoang-g5-prd.*.rds.amazonaws.com | 3306 | 메인 DB |
| ClickHouse | localhost | 9000 | 광고 분석 |
| Redis | localhost | 6379 | 캐시/세션 |
| S3 | damoang-data-v1 | - | 파일 스토리지 |
| CLIProxyAPI | localhost | 8317 | AI 평가 |
