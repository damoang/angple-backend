# ANGPLE Backend — 프로젝트 계획서

> 최종 수정: 2026-02-02 | 버전: v2.0

---

## 1. 프로젝트 비전

**다모앙(damoang.net) 커뮤니티 백엔드를 PHP(그누보드)에서 Go로 완전 마이그레이션하여, 응답 시간 800ms → 50ms, 동시 접속 2만명을 안정적으로 처리하는 고성능 API 서버 구축.**

### 목표

| 지표 | 현재 (PHP) | 목표 (Go) |
|------|-----------|----------|
| 응답 시간 | ~800ms | ≤50ms |
| 동시 접속 | ~5,000 | 20,000+ |
| 서버 비용 | 고사양 필요 | 저사양 가능 |
| 확장성 | 모놀리식 | 플러그인 아키텍처 |

---

## 2. 현재 상태 요약

- **구현 완료**: 70/81 v1 API + v2 API 레이어 — 전 Phase(1~16) 완료
- **v2 전환**: `/api/v1` (레거시, deprecated) + `/api/v2` (신규 DB) 공존 중
- **아키텍처**: Clean Architecture (Handler → Service → Repository) 확립
- **플러그인 시스템**: 스펙 완성, 기본 구현 완료, Hook 연동 완료
- **Commerce 플러그인**: 완료
- **인증**: JWT + 레거시 SSO (damoang_jwt) + 회원가입/탈퇴 동작
- **CI/CD**: GitHub Actions + GHCR + AWS EC2 배포 파이프라인 구축

### 완료된 API (28개)

| 영역 | API | 상태 |
|------|-----|------|
| 인증 | 로그인, 토큰 재발급, 프로필 조회, 회원가입, 로그아웃 | ✅ |
| 게시글 | 목록, 검색, 상세, 작성, 수정, 삭제 | ✅ |
| 댓글 | 목록, 상세, 작성, 수정, 삭제 | ✅ |
| 추천/비추천 | 게시글 추천/취소, 비추천/취소, 댓글 추천/취소 | ✅ |
| 회원 프로필 | 프로필 조회, 작성글, 작성댓글, 포인트 내역 | ✅ |
| 회원 탈퇴 | DELETE /members/me | ✅ |
| 파일 | 에디터 이미지 업로드, 첨부파일 업로드, 다운로드 | ✅ |
| 시스템 | Health Check | ✅ |

---

## 3. Phase별 실행 계획

### v1 완성 (Phase 1-7) — 레거시 DB 기반 66개 API

> 상세 API 목록: [`docs/api-roadmap.csv`](docs/api-roadmap.csv) 참조

#### Phase 1: 추천/비추천, 회원, 파일 업로드 (13개 API) ✅ 완료 (2026-02-02)

| API | Method | Endpoint | 상태 | PR |
|-----|--------|----------|------|-----|
| 게시글 추천 | POST | `/boards/{id}/posts/{id}/recommend` | ✅ | 기존 구현 |
| 게시글 추천 취소 | DELETE | `/boards/{id}/posts/{id}/recommend` | ✅ | 기존 구현 |
| 게시글 비추천 | POST | `/boards/{id}/posts/{id}/downvote` | ✅ | 기존 구현 |
| 게시글 비추천 취소 | DELETE | `/boards/{id}/posts/{id}/downvote` | ✅ | 기존 구현 |
| 댓글 추천 | POST | `.../comments/{id}/recommend` | ✅ | 기존 구현 |
| 댓글 추천 취소 | DELETE | `.../comments/{id}/recommend` | ✅ | 기존 구현 |
| 회원 프로필 | GET | `/members/{id}/profile` | ✅ | #74 |
| 회원 작성글 | GET | `/members/{id}/posts` | ✅ | #74 |
| 회원 작성댓글 | GET | `/members/{id}/comments` | ✅ | #74 |
| 포인트 내역 | GET | `/members/{id}/points/history` | ✅ | #74 |
| 회원가입 | POST | `/auth/register` | ✅ | #75 |
| 회원 탈퇴 | DELETE | `/members/me` | ✅ | #75 |
| 에디터 이미지 업로드 | POST | `/upload/editor` | ✅ | #76 |
| 첨부파일 업로드 | POST | `/upload/attachment` | ✅ | #76 |
| 파일 다운로드 | GET | `/files/{board_id}/{wr_id}/{file_no}/download` | ✅ | #76 |

**미구현 (Phase 1에서 제외)**:
- 소셜 로그인 (`/auth/social/{provider}`) → Phase 2 이후로 이동 (OAuth 프로바이더 연동 필요)

#### Phase 2: 스크랩, 메모, 차단, 쪽지 (15개 API) ✅ 완료 (2026-02-02)

| API | Method | Endpoint | 상태 | PR |
|-----|--------|----------|------|-----|
| 스크랩 추가 | POST | `/boards/{id}/posts/{id}/scrap` | ✅ | #77 |
| 스크랩 취소 | DELETE | `/boards/{id}/posts/{id}/scrap` | ✅ | #77 |
| 내 스크랩 목록 | GET | `/members/me/scraps` | ✅ | #77 |
| 메모 조회 | GET | `/members/{id}/memo` | ✅ | 기존 구현 |
| 메모 생성 | POST | `/members/{id}/memo` | ✅ | 기존 구현 |
| 메모 수정 | PUT | `/members/{id}/memo` | ✅ | 기존 구현 |
| 메모 삭제 | DELETE | `/members/{id}/memo` | ✅ | 기존 구현 |
| 회원 차단 | POST | `/members/{id}/block` | ✅ | #77 |
| 차단 해제 | DELETE | `/members/{id}/block` | ✅ | #77 |
| 차단 목록 | GET | `/members/me/blocks` | ✅ | #77 |
| 쪽지 보내기 | POST | `/messages` | ✅ | #77 |
| 받은 쪽지함 | GET | `/messages/inbox` | ✅ | #77 |
| 보낸 쪽지함 | GET | `/messages/sent` | ✅ | #77 |
| 쪽지 상세 | GET | `/messages/{id}` | ✅ | #77 |
| 쪽지 삭제 | DELETE | `/messages/{id}` | ✅ | #77 |

#### Phase 3: 알림, WebSocket (6개 API) ✅ 완료 (2026-02-02)

| API | Method | Endpoint | 상태 | PR |
|-----|--------|----------|------|-----|
| 알림 목록 | GET | `/notifications` | ✅ | 기존 구현 |
| 읽지 않은 알림 | GET | `/notifications/unread-count` | ✅ | 기존 구현 |
| 알림 읽음 처리 | POST | `/notifications/{id}/read` | ✅ | 기존 구현 |
| 모두 읽음 | POST | `/notifications/read-all` | ✅ | 기존 구현 |
| 알림 삭제 | DELETE | `/notifications/{id}` | ✅ | 기존 구현 |
| WebSocket 알림 스트림 | GET | `/ws/notifications` | ✅ | #78 |

Redis Pub/Sub 기반 멀티 인스턴스 알림 전파, gorilla/websocket 사용

#### Phase 4: 신고, 이용제한 (7개 API) ✅ 완료 (2026-02-02)

| API | Method | Endpoint | 상태 | PR |
|-----|--------|----------|------|-----|
| 신고 접수 | POST | `/reports` | ✅ | #79 |
| 내 신고 내역 | GET | `/reports/mine` | ✅ | #79 |
| 신고 통계 | GET | `/reports/stats` | ✅ | #79 |
| 이용제한 내역 | GET | `/members/me/disciplines` | ✅ | #79 |
| 이용제한 게시판 | GET | `/disciplines/board` | ✅ | #79 |
| 이용제한 열람 | GET | `/disciplines/{id}` | ✅ | #79 |
| 소명 글 작성 | POST | `/disciplines/{id}/appeal` | ✅ | #79 |

기존 관리자 API (신고 목록/데이터/최근/처리)는 이미 구현 완료

#### Phase 5: 추천글, 갤러리, 통합검색 (5개 API) ✅ 완료 (2026-02-02)

| API | Method | Endpoint | 상태 | PR |
|-----|--------|----------|------|-----|
| 메인 추천글 | GET | `/recommended/{period}` | ✅ | 기존 구현 |
| AI 분석 추천글 | GET | `/recommended/ai/{period}` | ✅ | 기존 구현 |
| 전체 갤러리 | GET | `/gallery` | ✅ | #80 |
| 게시판 갤러리 | GET | `/gallery/{board_id}` | ✅ | #80 |
| 통합 검색 | GET | `/search?q=` | ✅ | #80 |

Redis 캐시: 갤러리 5분, 검색 3분, 게시판ID 10분 TTL (동시접속 1만명 대비)

#### Phase 6: 관리자 API ✅ (PR #81)

| 카테고리 | API 수 | 상태 |
|---------|--------|------|
| 회원 관리 | 5 | ✅ List/Get/Update/AdjustPoint/Restrict |
| 게시판 관리 | 5 | ✅ 기존 구현 완료 |
| 그룹 관리 | 0 | ⏭️ g5_group 테이블 미존재, 스킵 |
| 신고 관리 | 5+ | ✅ 기존 구현 완료 |

총 API 수: 66/81 (Phase 6까지)

#### Phase 7: 광고 시스템 ✅ (PR #82)

| API | 상태 | 비고 |
|-----|------|------|
| 광고주 통계 | ✅ | GET /promotion/my/stats (본인만) |
| 남은 광고 기간 | ✅ | GET /promotion/my/remaining |
| 광고 가져오기 | ✅ | GET /banner/list?position= (기존) |
| 광고 클릭 기록 | ✅ | GET /banner/:id/click + POST /:id/view (기존) |

총 API 수: 70/81 (Phase 7까지, 신규 2개 + 기존 2개)

---

### v2 전환 (Phase 8-12) — 신규 DB 설계

> v2 DB 스키마: [`docs/specs/core-spec-v1.0.md` §3](docs/specs/core-spec-v1.0.md) 참조

#### Phase 8: v2 Core 테이블 마이그레이션 ✅ (PR #83)

- ✅ v2_ 접두사 Core 테이블 10개: users, boards, posts, comments, categories, tags, post_tags, files, notifications, sessions
- ✅ Meta 테이블 4개: user_meta, post_meta, comment_meta, option_meta
- ✅ GORM AutoMigrate (서버 시작 시 자동, 멱등)
- ✅ 데이터 마이그레이션: Go 코드 (MigrateV2Data) + SQL 스크립트 (migrations/001_gnuboard_to_v2.sql)
- ✅ 플러그인 관리 테이블 (이전 Phase에서 이미 구현됨)

#### Phase 9: v2 API 개발 (v1과 병행) ✅ (PR #84)

- ✅ v2 Repository 4종 (user, post, comment, board) — v2_ 테이블 기반 GORM 구현
- ✅ v2 Handler: Users/Boards/Posts/Comments CRUD 전체 엔드포인트
- ✅ v2 Routes: `/api/v2-next` 경로로 레거시와 충돌 없이 공존
- ✅ main.go DI 배선, 라우터 v1/v2 공존 처리

#### Phase 10: 프론트엔드 v2 전환 ✅ (PR #85)

- ✅ `V2Response` 표준 응답 형식 (`success` 필드, `per_page`, `total_pages`) — core-spec §4.3 준수
- ✅ v2 Handler를 V2Response 형식으로 전환
- ✅ `docs/v1-to-v2-migration-guide.md` — 프론트엔드 전환 가이드 (엔드포인트 매핑, 데이터 모델, 전환 절차)

#### Phase 11: v1 Deprecated ✅ (PR #86)

- ✅ Deprecation 미들웨어: `Deprecation: true`, `Sunset`, `Link` 헤더 자동 추가
- ✅ APIUsageTracker: 엔드포인트별 atomic counter 사용량 추적
- ✅ 모니터링 엔드포인트: `GET /api/v2/admin/v1-usage`, `POST .../reset`
- ✅ Sunset 날짜: 2026-08-01

#### Phase 12: v1/v2 URL 정리 ✅ (PR #87)

- ✅ 레거시 API: `/api/v2` → `/api/v1` 으로 이동 (deprecation 유지)
- ✅ 신규 v2 API: `/api/v2-next` → `/api/v2` 로 승격
- ✅ CLAUDE.md, migration guide 문서 URL 갱신
- ⏳ 레거시 코드 완전 제거는 프론트엔드 v2 전환 완료 후 진행

---

### 장기 비전 (Phase 13+)

#### Phase 13: 플러그인 마켓플레이스 API ✅ (PR #88)

- ✅ Public API 5개: 마켓플레이스 탐색/상세, 리뷰 목록/작성, 다운로드 추적
- ✅ Developer API 4개: 개발자 등록, 프로필, 플러그인 제출, 내 제출 목록
- ✅ Admin API 2개: 대기 중 제출 목록, 승인/거절 심사
- ✅ DB 테이블 4개: plugin_developers, plugin_submissions, plugin_reviews, plugin_downloads
- ⏳ 자동 보안 검사 파이프라인 (CI/CD 영역, 추후)
- ⏳ 수익 분배 시스템 (결제 연동 필요, 추후)

#### Phase 14: 멀티테넌트 지원 ✅

- ✅ TenantDBResolver: shared/schema/dedicated DB 전략별 격리
- ✅ PlanLimits: free/pro/business/enterprise 플랜별 리소스 제한
- ✅ TenantService: 목록/상세/정지/해제/플랜변경/사용량 조회
- ✅ Admin API 7개: `/api/v2/admin/tenants/*`
- ✅ 플랜 업그레이드 시 자동 스키마 생성

#### Phase 15: 호스팅 SaaS 백엔드 ✅

- ✅ 원클릭 커뮤니티 생성 API (ProvisionCommunity)
- ✅ 과금/구독 시스템 (Subscription, Invoice 모델 + CRUD)
- ✅ 플랜 변경/구독 취소/청구서 조회 API
- ✅ 플랜별 가격 정보 API (`/api/v2/saas/pricing`)
- ✅ SaaS API 7개: `/api/v2/saas/*`

#### Phase 16: AI 추천 엔진 ✅

- ✅ 사용자 행동 추적 (view/like/comment/scrap/search)
- ✅ 키워드/토픽 자동 추출 (TF 기반, 한국어+영어)
- ✅ 사용자 관심사 프로파일링 (UserInterest 누적 스코어)
- ✅ 개인화된 피드 생성 (토픽 매칭 + 인기글 보완)
- ✅ 트렌딩 토픽 (24h/7d/30d)
- ✅ API 6개: `/api/v2/recommendations/*` + `/api/v2/admin/recommendations/*`

---

### 프로덕션 준비 (Phase 17-20)

#### Phase 17: 테스트 커버리지 강화 ✅

- ✅ AuthService 테스트 12개: 로그인/회원가입/탈퇴/토큰갱신 (성공+실패 케이스)
- ✅ PostService 테스트 13개: CRUD/검색/페이지네이션/권한검증
- ✅ RecommendationService 테스트 8개: 키워드추출/불용어/HTML제거
- ✅ ProvisioningService 테스트: DB전략/플랜가격 검증
- ✅ TenantService 테스트: DB전략/PlanLimits 검증
- ✅ 기존 ReportService 테스트 유지
- 서비스 레이어 테스트 총 70개 PASS

#### Phase 18: 성능 최적화 & Redis 캐시 ✅

- ✅ 전역 Redis 캐시 미들웨어 (`middleware/cache.go`): GET 응답 캐시, X-Cache HIT/MISS 헤더
- ✅ 전역 Rate Limiter (`middleware/rate_limit.go`): Lua 슬라이딩 윈도우, IP별 120 req/min
- ✅ 사용자별 Rate Limiter (`RateLimitPerUser`): 인증 사용자 ID 기반
- ✅ 캐시 적용: 트렌딩 토픽(5분), 플랜 가격(10분), PlanLimits(10분)
- ✅ Fail-open 패턴: Redis 장애 시 정상 처리
- ✅ 캐시 무효화 함수 (`InvalidateCache`): 패턴 기반 일괄 삭제

#### Phase 19: Observability (모니터링/로깅/추적)

현재: stdout 단순 로거 + 플러그인 전용 메트릭. 프로덕션 수준 필요.

- 구조화 로깅 (zerolog 또는 zap): request_id, user_id, latency 포함
- Prometheus 메트릭 export (`/metrics` 엔드포인트)
  - HTTP 요청 수/지연/에러율, DB 커넥션 풀, Redis 히트율
- Grafana 대시보드 템플릿
- 감사 로그 (Audit Log): 로그인, 권한 변경, 데이터 삭제 등 민감 작업 기록
- Healthcheck 고도화: DB/Redis/외부 서비스 상태 포함

#### Phase 20: 보안 강화

- 소셜 로그인 (OAuth2): 네이버, 카카오, 구글 (api-roadmap.csv 미구현 항목)
- CORS 정책 강화 (현재 허용 범위 점검)
- API Key 인증 (외부 연동용)
- 입력 검증 강화 (XSS, SQL Injection 방어 재점검)
- Rate Limiting 전역 적용 (IP별 + 사용자별)
- CSRF 보호 (쿠키 인증 사용하는 v1 경로)

---

### 확장 기능 (Phase 21-24)

#### Phase 21: 실시간 검색 (Elasticsearch)

현재: DB LIKE 기반 통합 검색 (Redis 3분 캐시). 대규모 데이터에서 성능 한계.

- Elasticsearch 연동 (게시글/댓글 인덱싱)
- 한국어 형태소 분석기 (nori) 적용
- 검색 자동완성 (Suggest API)
- 검색 결과 하이라이팅
- 실시간 인덱스 동기화 (게시글 작성/수정/삭제 시)

#### Phase 22: 미디어 파이프라인

현재: 로컬 파일 시스템 기반 업로드. 스케일링 한계.

- S3/R2 호환 오브젝트 스토리지 연동
- 이미지 리사이즈/WebP 자동 변환 (서버사이드)
- CDN 연동 (CloudFront/Cloudflare)
- 동영상 업로드 + 트랜스코딩 (ffmpeg 또는 외부 서비스)
- 파일 바이러스 스캔

#### Phase 23: 국제화 (i18n)

현재: 한국어 단일 언어. 에러 메시지/응답 메시지 하드코딩.

- 메시지 번들 시스템 (ko, en, ja)
- Accept-Language 헤더 기반 자동 언어 감지
- 에러 코드 체계 정비 (문자열 메시지 → 코드 + 번역)
- 관리자 UI 다국어 지원

#### Phase 24: 결제 연동

현재: SaaS 과금 모델만 존재 (Subscription/Invoice). 실 결제 미연동.

- 토스페이먼츠 연동 (국내 주력)
- Stripe 연동 (해외)
- 웹훅 수신 및 결제 상태 동기화
- 자동 결제 실패 시 재시도/알림
- 환불 처리 API
- 플러그인 마켓플레이스 수익 분배

---

## 4. 핵심 마일스톤

| 마일스톤 | Phase | 체크포인트 |
|---------|-------|-----------|
| **v1 API 완성** | Phase 7 ✅ | 70/81 API 구현, 프론트엔드 연동 가능 |
| **v2 DB 마이그레이션** | Phase 8 ✅ | v2 테이블 생성 + 마이그레이션 스크립트 |
| **v2 전환 완료** | Phase 12 ✅ | v1→deprecated, v2→활성 URL 정리 |
| **SaaS 기반 완성** | Phase 15 ✅ | 원클릭 커뮤니티 생성, 구독 관리 |
| **AI 추천 완성** | Phase 16 ✅ | 개인화 피드, 트렌딩, 토픽 추출 |
| **프로덕션 준비** | Phase 20 완료 | 테스트 70%+, 모니터링, 보안 강화 |
| **풀 SaaS 런칭** | Phase 24 완료 | 결제 연동, 미디어 CDN, 검색 고도화 |

---

## 5. 기술 부채 및 리스크

### 현재 기술 부채

| 항목 | 심각도 | 해결 Phase | 설명 |
|------|--------|-----------|------|
| 테스트 커버리지 부족 | 높 | Phase 17 | 핵심 서비스 테스트 29파일 중 대부분 플러그인 영역 |
| Redis 캐시 전역 미적용 | 중 | Phase 18 | commerce 플러그인에만 적용, 코어 API 캐시 없음 |
| 구조화 로깅 미적용 | 중 | Phase 19 | stdout 단순 로거, request_id/tracing 없음 |
| 감사 로그 미존재 | 중 | Phase 19 | 민감 작업 추적 불가 |
| 소셜 로그인 미구현 | 중 | Phase 20 | 네이버/카카오/구글 OAuth 미연동 |
| sql_mode 비활성화 | 중 | v1 제거 시 | 그누보드 호환용, v2 전용 시 STRICT 활성화 |
| 로컬 파일 시스템 의존 | 중 | Phase 22 | 멀티 인스턴스 배포 시 파일 공유 문제 |
| 결제 미연동 | 높 | Phase 24 | SaaS 과금 모델만 존재, 실 결제 플로우 없음 |

### 리스크

| 리스크 | 영향도 | 완화 전략 |
|--------|--------|----------|
| 프로덕션 장애 감지 지연 | 높음 | Phase 19 Observability로 해결 |
| 동시 접속 폭증 | 중간 | Phase 18 캐시 + Rate Limiter 전역 적용 |
| 마이그레이션 데이터 손실 | 높음 | 이중 쓰기, 롤백 스크립트, 데이터 검증 도구 |
| 결제 사고 | 높음 | Phase 24에서 웹훅 검증 + 멱등성 보장 |

---

## 6. 관련 문서

| 문서 | 경로 | 설명 |
|------|------|------|
| Core 스펙 v1.0 | [`docs/specs/core-spec-v1.0.md`](docs/specs/core-spec-v1.0.md) | v2 DB 스키마, API 규약, 인증 시스템 |
| 플러그인 스펙 v1.0 | [`docs/specs/plugin-spec-v1.0.md`](docs/specs/plugin-spec-v1.0.md) | 플러그인 개발 규약 |
| 내부 연동 스펙 | [`docs/specs/internal-integration-spec.md`](docs/specs/internal-integration-spec.md) | damoang-ops, angple-ads 연동 |
| API 로드맵 | [`docs/api-roadmap.csv`](docs/api-roadmap.csv) | 81개 API 상세 목록 |
| 개발 가이드 | [`CLAUDE.md`](CLAUDE.md) | 코딩 컨벤션, 아키텍처, 명령어 |
| DB 스키마 | [`DATABASE.md`](DATABASE.md) | 현재 DB 구조 |
| 프론트엔드 계획 | [`../angple/plan.md`](../angple/plan.md) | 프론트엔드 Phase 연동 |
