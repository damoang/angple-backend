# Task Plan: Backend Separation (angple-backend → angple-core + damoang-backend)

## Goal
`backend-separation-plan.md` 문서에 따라 angple-backend에서 다모앙 전용 코드를 분리하여:
1. angple-backend를 오픈소스 코어(v2 only)로 정리
2. damoang-backend를 다모앙 전용 통합 백엔드로 생성
3. damoang-ads 코드를 damoang-backend에 흡수
4. 프론트엔드(angple, damoang-ops, damoang-ads) API 전환

## Current Phase
Phase 1 완료, Phase 2 대기

---

## Phase 1: damoang-backend 프로젝트 초기 설정
- [x] Go 모듈 초기화 (`github.com/damoang/damoang-backend`)
- [x] 디렉터리 구조 생성 (cmd/, internal/, configs/, pkg/)
- [x] 기본 설정 파일 생성 (config.go, config.local.yaml, config.docker.yaml)
- [x] Gin 프레임워크 + 기본 의존성 설정
- [x] main.go 기본 골격 (MySQL, Redis 연결 + ClickHouse placeholder)
- [x] Makefile, Dockerfile, docker-compose.yml (ClickHouse 포함)
- [x] .gitignore, .env.example, README.md
- [x] pkg/ 패키지 (logger, redis, jwt, jwt/damoang, cache)
- [x] internal/common/response.go (API 응답 헬퍼)
- [x] 빌드 성공 검증 완료

## Phase 2: angple-backend → damoang-backend 코드 이동

### 2-1: 인증 레이어 이동
- [ ] `cookie_auth.go` (damoang_jwt 쿠키 인증) 복사 & 수정
- [ ] `pkg/auth/legacy.go` (그누보드 호환) 복사
- [ ] `pkg/jwt/damoang.go` 복사
- [ ] damoang-backend 자체 JWT 매니저 설정

### 2-2: v1 라우트 전체 이동
- [ ] `internal/routes/routes.go` → damoang-backend로 복사 & import 수정
- [ ] v1 전용 핸들러 이동 (auth, post, comment, board, member, member_profile, good, scrap, message, block, notification, memo, reaction, file, menu, site, autosave, filter, token, dajoongi, recommended)
- [ ] v1 전용 서비스 이동
- [ ] v1 전용 레포지토리 이동
- [ ] v1 전용 도메인 모델 이동

### 2-3: 다모앙 전용 핸들러/서비스 이동
- [ ] report_handler.go + report_service.go
- [ ] ai_evaluation_handler.go + ai_evaluator.go (32KB) + ai_evaluation_service.go
- [ ] discipline_handler.go + discipline_service.go
- [ ] promotion_handler.go + promotion_service.go
- [ ] banner_handler.go + banner_service.go
- [ ] payment_handler.go + payment_service.go
- [ ] gallery_handler.go + gallery_service.go
- [ ] good_handler.go + good_service.go
- [ ] recommended_handler.go
- [ ] admin_handler.go + admin_member_service.go
- [ ] recommendation_handler.go + recommendation_service.go

### 2-4: 다모앙 전용 미들웨어 이동
- [ ] `deprecation.go` 이동
- [ ] `v1_redirect.go` 이동
- [ ] `permission.go` 판단 & 처리

### 2-5: 내장 플러그인 이동
- [ ] `internal/plugins/advertising/` 이동
- [ ] `internal/plugins/commerce/` 이동
- [ ] `internal/plugins/embed/` 이동
- [ ] `internal/plugins/imagelink/` 이동
- [ ] `internal/plugins/marketplace/` 이동 (판단 필요: 오픈소스 유지 가능)
- [ ] `plugins/giving/` 이동
- [ ] `plugins/banner/` 이동
- [ ] `plugins/emoticon/` 이동
- [ ] `plugins/promotion/` 이동

### 2-6: 다모앙 전용 도메인 모델 이동
- [ ] promotion.go, banner.go, dajoongi.go, discipline.go
- [ ] payment.go, ai_evaluation.go, report.go
- [ ] singo_user.go, singo_setting.go, recommendation.go

## Phase 3: damoang-ads 코드 흡수
- [ ] damoang-ads 핸들러 22개 → Fiber→Gin 변환하여 이식
- [ ] ClickHouse 클라이언트 코드 이식
- [ ] 배너/광고 관리 핸들러 이식
- [ ] AdSense 슬롯 관리 이식
- [ ] 축하배너, 경제 게시물, 프로모션 이식
- [ ] 통계 집계 이식
- [ ] damoang-ads 라우트 통합 (/api/v1/serve/*, /api/v1/admin/*)

## Phase 4: angple-backend 정리 (오픈소스 준비)

### 4-1: config 정리
- [ ] `JWTConfig.DamoangSecret` 제거
- [ ] 업로드 경로 기본값에서 `/home/damoang/` 제거
- [ ] CORS 기본값에서 `damoang.net` 제거

### 4-2: 코드 제거
- [ ] v1 라우트 전체 제거 (`internal/routes/routes.go`)
- [ ] v1 전용 handler/service/repository/domain 제거
- [ ] 다모앙 전용 handler/service 제거
- [ ] 내장 플러그인 디렉토리 제거
- [ ] cookie_auth.go, deprecation.go, v1_redirect.go 제거

### 4-3: main.go 정리
- [ ] `damoangJWT` 관련 코드 제거
- [ ] `AI_EVAL_*` 환경 변수 제거
- [ ] 다모앙 전용 플러그인 import 제거
- [ ] Repository/Service/Handler DI 정리 (~30개 → ~15개)

### 4-4: v2 routes 정리
- [ ] `SetupReports` 제거 (DamoangCookieAuth 사용)
- [ ] AI evaluation 라우트 제거
- [ ] Payment 라우트 제거

### 4-5: 판단 필요 파일 처리
- [ ] oauth_handler.go → Generic provider 설정으로 리팩터
- [ ] search_handler.go → 오픈소스 유지 (ES 선택적)
- [ ] media_handler.go → 오픈소스 유지 (S3 선택적)
- [ ] permission.go → v2용으로 리팩터

## Phase 5: 프론트엔드 API 전환
- [ ] damoang-ops: nginx 프록시 → damoang-backend:8090
- [ ] damoang-ads: 자체 Go 서버 제거, 프론트엔드만 유지
- [ ] angple web: v2 API만 사용하도록 전환 (장기)
- [ ] nginx 설정 정리 (3 upstream → 2 upstream)

## Phase 6: Docker 이미지 기반 배포
- [ ] angple-backend → GHCR 이미지 (Public)
- [ ] damoang-backend → ECR 이미지 (Private)
- [ ] 배포 스크립트 통합

---

## Summary

| Phase | 설명 | 상태 |
|-------|------|------|
| Phase 1 | damoang-backend 초기 설정 | ✅ 완료 |
| Phase 2 | 코드 이동 (angple → damoang) | ❌ 대기 |
| Phase 3 | damoang-ads 흡수 | ❌ 대기 |
| Phase 4 | angple-backend 정리 | ❌ 대기 |
| Phase 5 | 프론트엔드 전환 | ❌ 대기 |
| Phase 6 | Docker 배포 | ❌ 대기 |

## Notes
- Phase 2와 Phase 4는 순서가 중요: 먼저 damoang-backend에 코드를 복사한 후, angple-backend에서 제거
- 서비스 중단 방지: damoang-backend가 가동되기 전까지 angple-backend의 코드를 제거하면 안 됨
- DB 변경 없음: 같은 MySQL RDS 계속 사용
