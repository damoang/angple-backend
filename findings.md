# Findings: Backend Separation (angple-backend → damoang-backend)

## 1. 프로젝트 현황 요약

### angple-backend (현재 프로젝트)
- **위치**: `/Users/sdk/IdeaProjects/angple-backend`
- **프레임워크**: Go/Gin, port 8082 (prod), 8081 (dev)
- **규모**: Handler 39개, Service 40+개, Repository 39개, Domain 38개, Middleware 17개
- **API 버전**: v1 (g5_* legacy) + v2 (신규)
- **인증**: JWT Bearer + damoang_jwt Cookie + OAuth2 + API Key
- **외부 서비스**: MySQL, Redis, Elasticsearch, S3, ClickHouse(간접)
- **플러그인**: advertising, commerce, embed, imagelink, marketplace, giving, banner, emoticon, promotion

### damoang-backend (새 레포)
- **위치**: `/Users/sdk/IdeaProjects/damoang-backend`
- **상태**: 완전히 비어있음 (README.md만 존재)
- **Remote**: https://github.com/damoang/damoang-backend.git
- **커밋**: 1개 (Initial commit, 2026-02-15)
- go.mod, 소스 코드, 설정 파일 없음

### angple (프론트엔드 모노레포)
- **위치**: `/Users/sdk/IdeaProjects/angple`
- **구조**: apps/web + apps/admin + packages/* + plugins/*
- **API 호출**: **126개+ 엔드포인트** (v1 + v2 혼합)
- **프록시**: vite → localhost:8081 (/api/v1, /api/v2)
- **핵심 의존**: v1 API를 매우 많이 사용 (게시글, 댓글, 추천, 검색, 회원, 신고 등)

### damoang-ads (광고 서비스)
- **위치**: `/Users/sdk/IdeaProjects/damoang-ads/server`
- **프레임워크**: Go/Fiber, port 9090
- **DB**: ClickHouse (19000) + MySQL (3306) + Redis (6379)
- **독립성**: angple-backend에 직접 의존 없음 (자체 DB 접근)
- **핸들러**: 22개 (banner, advertiser, adsense, promotion, celebration, economy 등)
- **흡수 대상**: damoang-backend로 통합 예정

### damoang-ops (신고 관리)
- **위치**: `/Users/sdk/IdeaProjects/damoang-ops/apps/singo`
- **프레임워크**: SvelteKit
- **프록시**: vite → localhost:8082 (/api), localhost:8317 (/ai-cli)
- **핵심 의존**: angple-backend v1 report API 전면 의존
- **AI 통합**: Claude, OpenAI, Gemini API 프록시

---

## 2. API 의존성 분석 (핵심)

### angple web → angple-backend 호출 분류

#### 오픈소스 유지 (v2 API)
```
POST /api/v2/auth/login, /exchange, /refresh
GET  /api/v2/auth/profile
POST /api/v2/media/attachments, /images
GET  /api/v2/admin/plugins, /menus
GET  /api/v2/search/*
```

#### v1 API → damoang-backend로 이전 필요
```
# 인증
POST /api/v1/auth/login, /logout, /refresh
GET  /api/v1/auth/me, /profile, /oauth/*

# 게시판/게시글 (g5_* 테이블 의존)
GET  /api/v1/boards/{boardId}
GET  /api/v1/boards/{boardId}/posts (CRUD 전체)
GET  /api/v1/boards/{boardId}/posts/{id}/comments (CRUD)

# 추천/비추천
POST /api/v1/boards/{boardId}/posts/{id}/like, /dislike
GET  /api/v1/boards/{boardId}/posts/{id}/like-status, /likers

# 회원
GET  /api/v1/members/{id}, /my/activity, /my/posts, /my/comments
POST /api/v1/members/{id}/block

# 스크랩
POST /api/v1/posts/{id}/scrap

# 검색
GET  /api/v1/search?q=...

# 알림/쪽지
GET  /api/v1/notifications/*, /messages/*

# 기타
GET  /api/v1/recommended/*, /menus/sidebar, /board-groups
```

### damoang-ops → angple-backend 호출 (전부 이전 대상)
```
# 신고
GET  /api/v1/reports?status=...
GET  /api/v1/reports/data, /recent, /stats, /adjacent
POST /api/v1/reports/process, /batch-process

# 관리
GET  /api/v1/admin/members
PUT  /api/v1/admin/members/{id}
GET  /api/v1/reports/discipline-history

# AI 평가
POST /api/v2/reports/ai-evaluation
GET  /api/v2/reports/ai-evaluation, /list

# 신고 사용자
GET  /api/v1/singo-users/me, (list)
POST /api/v1/singo-users
PUT  /api/v1/singo-users/{id}
DELETE /api/v1/singo-users/{id}
```

### damoang-ads → 독립 (angple-backend 미의존)
- 자체 MySQL/ClickHouse/Redis 직접 접속
- 외부 호출: Naver URL 스크래핑만

---

## 3. 코드 분류 상세 (angple-backend 내부)

### 오픈소스 유지 파일 (angple-backend에 남음)
| 카테고리 | 파일 수 | 핵심 파일 |
|----------|---------|-----------|
| v2 Handler | 8 | handler/v2/{handler,auth,admin,install,scrap,memo,block,message} |
| v2 Service | 2 | service/v2/{auth,admin}_service |
| v2 Repository | 8 | repository/v2/{user,post,comment,board,scrap,memo,block,message}_repo |
| v2 Domain | 4+ | domain/v2/{models,memo,message,scrap} |
| Core Middleware | 7 | auth, admin, security, cache, metrics, request_logger, rate_limit |
| Plugin Framework | 30 | internal/plugin/* 전체 |
| Plugin Store | 15+ | internal/pluginstore/* 전체 |
| Packages | 8 | pkg/{jwt,auth,logger,redis,elasticsearch,storage,cache,i18n} |
| Config | 1 | internal/config/config.go |
| WebSocket | 2 | internal/ws/{hub,client} |
| Common | 3 | internal/common/{response,v2_response,errors} |
| Migration | 3 | internal/migration/{migrate,v2_schema,v2_data} |

### damoang-backend로 이동할 파일
| 카테고리 | 파일 수 | 핵심 파일 |
|----------|---------|-----------|
| v1 Handler | 17+ | auth, post, comment, board, member, member_profile, good, scrap, message, block, notification, memo, reaction, file, menu, site, autosave... |
| 다모앙 전용 Handler | 14 | report, ai_evaluation, discipline, promotion, banner, dajoongi, gallery, recommended, admin, payment, recommendation, good, gallery, recommended |
| v1 Service | 14+ | 위 핸들러에 대응하는 서비스 전체 |
| 다모앙 전용 Service | 10+ | ai_evaluator(32KB), report, discipline, promotion, banner, gallery, good, payment, admin_member, recommendation |
| v1 Repository | 20+ | 위에 대응하는 레포 전체 |
| v1 Routes | 1 | internal/routes/routes.go |
| 다모앙 Middleware | 3 | cookie_auth, deprecation, v1_redirect |
| 내장 Plugin | 9개 디렉토리 | advertising, commerce, embed, imagelink, marketplace, giving, banner, emoticon, promotion |
| 다모앙 Domain | 10+ | promotion, banner, dajoongi, discipline, payment, ai_evaluation, report, singo_*, recommendation |

### 판단 필요 파일
| 파일 | 현재 | 권장 방향 |
|------|------|-----------|
| oauth_handler.go | 네이버/카카오/구글 하드코딩 | Generic provider 설정으로 리팩터 → 오픈소스 유지 |
| search_handler.go | ES 통합 검색 | 오픈소스 유지 (ES 선택적) |
| media_handler.go | S3 업로드 | 오픈소스 유지 (S3 선택적) |
| tenant_handler.go | 멀티테넌트 | 오픈소스 유지 |
| permission.go | g5 테이블 의존 | v2용으로 리팩터 or 이동 |

---

## 4. 기술적 제약사항

### DB 공유
- damoang-backend는 **같은 MySQL RDS** 사용 (코드 분리만, DB 분리 없음)
- g5_* + v2_* 테이블 모두 접근 필요
- 연결 풀 통합 (현재 3개 → 1개)

### 인증 체계
- angple-backend(오픈소스): Bearer JWT만 유지
- damoang-backend: damoang_jwt 쿠키 + Bearer JWT 모두 지원
- JWT Secret 분리: `DamoangSecret` → damoang-backend 전용

### v1 API Sunset
- 현재 Deprecation 헤더: `Sunset: Sat, 01 Aug 2026 00:00:00 GMT`
- angple-backend에서 v1 제거 후, damoang-backend에서 계속 운영
- angple web의 v1→v2 전환 완료까지 damoang-backend가 v1 유지

### Go 모듈
- angple-backend: `github.com/angple/angple-backend`
- damoang-backend: `github.com/damoang/damoang-backend` (신규)
- damoang-backend는 angple-backend를 import하지 않음 (완전 독립)

### damoang-ads 흡수
- Fiber → Gin 프레임워크 전환 필요
- ClickHouse 클라이언트 코드 이식
- 핸들러 22개 재작성/이식

---

## 5. nginx 전환 계획

### Before (현재)
```
api.damoang.net       → 127.0.0.1:8082 (angple-backend)
ops.damoang.net /api  → 127.0.0.1:8082 (angple-backend 경유)
ops.damoang.net /ai   → 127.0.0.1:8317 (CLIProxyAPI)
ops.damoang.net /*    → 127.0.0.1:5175 (damoang-ops SvelteKit)
ads.damoang.net /api  → 127.0.0.1:9090 (damoang-ads Go)
```

### After (목표)
```
api.damoang.net       → 127.0.0.1:8082 (angple-backend, v2 only, 오픈소스)
ops.damoang.net /api  → 127.0.0.1:8090 (damoang-backend)
ops.damoang.net /ai   → 127.0.0.1:8317 (CLIProxyAPI)
ops.damoang.net /*    → 127.0.0.1:5175 (damoang-ops SvelteKit)
ads.damoang.net /api  → 127.0.0.1:8090 (damoang-backend, ads 흡수)
```

---

## 6. 위험 요소

1. **angple web v1 의존도 높음**: 126개+ 엔드포인트 중 대다수가 v1 → 전환 시 angple web도 동시에 수정 필요하거나, 과도기에 damoang-backend가 v1을 서빙해야 함
2. **코드 복사 시 import 경로 전체 변경**: `github.com/angple/angple-backend/internal/*` → `github.com/damoang/damoang-backend/internal/*`
3. **damoang-ads Fiber→Gin 전환**: 핸들러 시그니처, 미들웨어 패턴 차이
4. **테스트 커버리지**: 기존 테스트가 분리 후에도 동작하는지 검증 필요
5. **배포 순서**: angple-backend 정리와 damoang-backend 가동을 동시에 해야 서비스 중단 없음
