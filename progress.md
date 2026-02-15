# Progress: Backend Separation

## Session Log

### 이전 세션 요약 (Phase 1&2 API 구현)
- Phase 1 (추천/비추천, 회원, 파일 업로드) 16개 API 구현 완료
- Phase 2 (스크랩, 메모, 차단, 쪽지) 15개 API 구현 완료
- 총 31개 v2 API 구현됨

---

### 2026-02-15: Backend Separation 분석 시작

#### 수행한 작업
1. **backend-separation-plan.md 분석 완료**
   - 4 Phase 로드맵 확인 (angple정리 → damoang생성 → 프론트전환 → Docker배포)
   - 파일별 분류표 확인 (오픈소스 유지 vs 다모앙 전용 vs 판단 필요)

2. **angple-backend 프로젝트 구조 전수 조사**
   - Handler 39개, Service 40+개, Repository 39개, Domain 38개
   - Middleware 17개, Plugin 9개 디렉터리
   - v1 routes + v2 routes 분리 상태
   - main.go: Repository 30개, Service 28개, Handler 25개+, Plugin 6개 import

3. **damoang-backend 레포 확인**
   - 완전히 비어있음 (README.md만)
   - Remote: github.com/damoang/damoang-backend
   - 초기 설정부터 필요

4. **관련 프로젝트 3개 분석 완료**
   - **angple (프론트)**: v1 API 126개+ 호출, v2도 혼용 → v1 의존도 매우 높음
   - **damoang-ads**: 독립 서비스(Go/Fiber, port 9090), 핸들러 22개, angple-backend 미의존
   - **damoang-ops/singo**: angple-backend v1 report API 전면 의존, AI 평가 v2 API 사용

5. **작업 메모리 파일 갱신**
   - findings.md: API 의존성 분석, 코드 분류, 기술적 제약사항, 위험 요소
   - task_plan.md: 6 Phase 실행 계획 + 체크리스트
   - progress.md: 현재 파일

#### 핵심 발견
- angple web이 v1 API를 **매우 많이** 사용 → 과도기에 damoang-backend가 v1을 서빙해야 함
- damoang-ads는 angple-backend에 **의존 없음** → 흡수 시 Fiber→Gin 변환만 필요
- damoang-ops는 v1 report API에 **완전 의존** → damoang-backend 가동 후 프록시 전환 필요
- 코드 분리 시 import 경로 전체 변경 필요 (module path 변경)

### 2026-02-15: Phase 1 - damoang-backend 초기 설정 완료

#### 생성한 파일 (damoang-backend 레포)
```
damoang-backend/
├── cmd/api/main.go              # 엔트리포인트 (MySQL+Redis+Health)
├── internal/
│   ├── config/config.go         # Config 구조체 (MySQL+ClickHouse+Redis+JWT+S3)
│   ├── common/response.go       # API 응답 헬퍼
│   ├── handler/                  # (빈 디렉터리, Phase 2에서 채움)
│   ├── service/                  # (빈 디렉터리)
│   ├── repository/               # (빈 디렉터리)
│   ├── domain/                   # (빈 디렉터리)
│   ├── middleware/               # (빈 디렉터리)
│   └── routes/                   # (빈 디렉터리)
├── configs/
│   ├── config.local.yaml        # 로컬 개발 설정 (port 8090)
│   └── config.docker.yaml       # Docker 설정
├── deployments/docker/
│   └── api.Dockerfile           # Multi-stage build
├── pkg/
│   ├── logger/logger.go         # zerolog 기반 로거
│   ├── redis/client.go          # Redis 클라이언트
│   ├── jwt/jwt.go               # JWT 토큰 매니저
│   ├── jwt/damoang.go           # 다모앙 JWT 쿠키 검증
│   └── cache/cache.go           # Redis 캐시 서비스
├── docker-compose.dev.yml       # MySQL + Redis + ClickHouse + API
├── Makefile                      # make dev, build, test 등
├── .gitignore
├── .env.example
├── README.md
├── go.mod                        # github.com/damoang/damoang-backend
└── go.sum
```

#### 설계 결정
- **포트**: 8090 (angple-backend 8082와 충돌 방지)
- **프레임워크**: Gin (angple-backend과 동일, damoang-ads의 Fiber 대신)
- **모듈 경로**: `github.com/damoang/damoang-backend`
- **ClickHouse**: docker-compose에 포함, 코드 연결은 Phase 3에서
- **Config**: angple-backend 패턴 재사용 + ClickHouse/NaverAPI/Cache TTL 추가
- **JWT**: 자체 패키지로 구현 (angple-backend import 없이 독립)

#### 빌드 검증
- `go build ./...` 성공
- `go mod tidy` 의존성 정리 완료

### 2026-02-15: Phase 2 - angple-backend → damoang-backend 코드 이동 완료

#### 수행한 작업
1. **전체 코드 벌크 복사** (angple-backend → damoang-backend)
   - middleware 17개, domain 33+4개(v2), repository 33+8개(v2), service 28개, handler 36개
   - routes/routes.go, ws/hub.go, ws/client.go
   - pkg/auth, pkg/elasticsearch, pkg/storage, pkg/i18n
   - internal/plugin (16 파일 - hook manager, factory, registry 등)
   - internal/plugins (advertising, commerce, embed, imagelink, marketplace)
   - plugins/ (banner, emoticon, giving, promotion)

2. **import 경로 전체 치환**
   - `github.com/damoang/angple-backend` → `github.com/damoang/damoang-backend`

3. **빌드 오류 5건 수정**
   - `internal/domain/v2`, `internal/repository/v2`, `internal/plugin` 누락 → 추가 복사
   - `common.ErrNotFound` 미정의 → errors.go 복사
   - `pkglogger.GetLogger` 미정의 → logger.go에 zerolog 통합
   - JWT Claims 필드 불일치 → angple-backend와 동일한 JWT 패키지로 교체
   - `*cache.Service` 포인터→인터페이스 타입 오류 수정

4. **main.go 완전 DI 배선 작성**
   - Repository 26+개, Service 24+개, Handler 26+개 인스턴스 생성
   - AI Evaluator 설정, WebSocket Hub, v1 Usage Tracker
   - v1 routes + AI Evaluation v2 routes 등록
   - 6개 플러그인 blank import 추가

5. **최종 빌드 검증 성공**
   - `go build ./...` 통과 (301 Go 파일)
   - `go mod tidy` 정상

#### 현재 damoang-backend 구조 (301 Go files)
```
damoang-backend/
├── cmd/api/main.go               # 완전한 DI 배선
├── internal/
│   ├── config/                    # Config (MySQL+Redis+ClickHouse+JWT+S3)
│   ├── common/                    # errors.go, response.go, v2_response.go
│   ├── handler/ (36)              # v1 전체 핸들러
│   ├── service/ (32)              # 전체 서비스
│   ├── repository/ (33+8 v2)      # 전체 레포지토리
│   ├── domain/ (33+4 v2)          # 전체 도메인 모델
│   ├── middleware/ (16)           # 인증, 보안, 캐시, 메트릭 등
│   ├── routes/routes.go           # v1 라우트 등록
│   ├── ws/ (2)                    # WebSocket
│   ├── plugin/ (16)               # 플러그인 프레임워크
│   └── plugins/                   # 내장 플러그인
│       ├── advertising/
│       ├── commerce/ (large)
│       ├── embed/
│       ├── imagelink/
│       └── marketplace/
├── plugins/                       # 외부 플러그인
│   ├── banner/
│   ├── emoticon/
│   ├── giving/
│   └── promotion/
├── pkg/                           # 공유 패키지
│   ├── auth/, cache/, elasticsearch/
│   ├── i18n/, jwt/, logger/
│   ├── redis/, storage/
├── configs/                       # config.local.yaml, config.docker.yaml
├── deployments/docker/            # Dockerfile
├── docker-compose.dev.yml
├── Makefile
└── go.mod                         # github.com/damoang/damoang-backend
```

### 2026-02-15: Phase 3 - damoang-ads 흡수 완료

#### 수행한 작업
1. **Phase 3-1: 코드 복사** (damoang-ads → damoang-backend/internal/ads/)
   - database/ (clickhouse.go, mysql.go, redis.go)
   - models/ (7 파일: types, banner_types, economy_types, promotion_types, adsense_types, celebration_types, ads_management_types)
   - repository/ (9 파일: stats, user, banner, advertiser_mgmt, adsense, celebration, economy, promotion, member)
   - service/ (21 파일: period, advertiser, board, os, position, ads, browser, device, user, admin_user, banner, adsense, advertiser_mgmt, promotion, celebration, economy, economy_scheduler, member, s3_uploader, scraper, utils)
   - handler/ (20 파일: health, period, advertiser, board, os, position, ads, browser, device, user, admin_user, auth, advertiser_mgmt, banner, adsense, promotion, celebration, economy, dev_auth, member)
   - middleware/ (4 파일: jwt, admin_auth, authorization, last_login)
   - routes/router.go
   - config/config.go (신규 생성 — ads 전용 config 타입)

2. **Phase 3-2: Fiber → Gin 핸들러 변환**
   - 전체 핸들러 20개 Fiber→Gin 기계적 변환 (sed/perl)
   - `*fiber.Ctx` → `*gin.Context`, `c.Status(N).JSON()` → `c.JSON(N, ...)`, etc.
   - 모델 struct tag: `query:"field"` → `form:"field"`
   - `c.Query("key", "default")` → `c.DefaultQuery("key", "default")`
   - `c.QueryInt()` → `strconv.Atoi(c.DefaultQuery(...))`
   - `c.Locals()` → `c.Get()`/`c.Set()` (getter/setter 분리)
   - `c.IP()` → `c.ClientIP()`
   - `fiber.Cookie{}` → `c.SetCookie()`
   - Handler return type `error` 제거, `return` 추가

3. **Phase 3-3: 미들웨어 & 라우트 변환**
   - jwt.go: Fiber→Gin 완전 재작성 (쿠키 읽기, 컨텍스트 패턴, Abort 플로우)
   - admin_auth.go: Gin 패턴으로 재작성
   - authorization.go: 광고주 접근 제어 Gin 패턴
   - last_login.go: 비동기 업데이트, Gin c.Get() 패턴
   - cors.go, logger.go, error_handler.go: 삭제 (damoang-backend 기존 미들웨어 사용)
   - routes/router.go: 완전 재작성, `/api/ads/` prefix로 v1 충돌 방지

4. **Phase 3-4: main.go DI 배선**
   - config.go에 AdsDatabase, AdsJWT 설정 추가 + env override
   - Config → ads config 타입 변환 (ClickHouse, MySQL, Redis, Cache, S3, NaverAPI)
   - DB 클라이언트 3개 생성 (ClickHouse, ads MySQL, ads Redis)
   - Repository 9개 → Service 18개+S3Uploader → Handler 20개 인스턴스 생성
   - `adsroutes.SetupAdsRoutes()` 호출
   - Economy scheduler 시작
   - Graceful fallback: ads DB 미설정/연결 실패 시 ads 시스템 비활성화

#### 최종 빌드 검증
- `go build ./...` 통과 (367 Go 파일)
- 신규 파일 66개 (internal/ads/ 전체)
- 광고 라우트 50+개 등록 (/api/ads/*)

#### 설계 결정
- **라우트 prefix**: `/api/ads/` (기존 `/api/v1/`과 충돌 방지)
- **DB 분리**: ads MySQL은 별도 연결 (angple_ads DB), 메인 MySQL은 GORM 유지
- **Redis 공유**: 동일 Redis 인스턴스 사용, ads 전용 RedisClient로 래핑
- **JWT**: ads_jwt secret은 AdsJWT.Secret → DamoangSecret 폴백
- **Graceful degradation**: ClickHouse/MySQL/Redis 중 하나라도 실패하면 ads 시스템 비활성화

## Current Status
- [x] 문서 분석 완료
- [x] 4개 프로젝트 전수 조사 완료
- [x] 작업 메모리 파일 갱신 완료
- [x] Phase 1 (damoang-backend 초기 설정) 완료
- [x] Phase 2 (코드 이동) 완료 — 301 Go files, 빌드 성공
- [x] Phase 3 (damoang-ads 흡수) 완료 — 367 Go files, 빌드 성공
- [ ] Phase 4 (angple-backend 정리) 대기
- [ ] Phase 5 (프론트엔드 전환) 대기
- [ ] Phase 6 (Docker 배포) 대기
