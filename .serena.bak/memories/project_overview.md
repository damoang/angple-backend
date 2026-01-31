# Angple Backend - 프로젝트 개요

## 프로젝트 목적
다모앙(damoang.net) 커뮤니티를 위한 차세대 Go API 서버입니다. 
기존 PHP 그누보드 시스템을 Go로 마이그레이션하여 응답 시간을 800ms → 50ms로 개선하는 것이 목표입니다.

## 기술 스택
- **언어**: Go 1.23+
- **웹 프레임워크**: Fiber v2 (Express 스타일)
- **ORM**: GORM (MySQL)
- **데이터베이스**: MySQL 8.0 (그누보드 `g5_*` 테이블 호환)
- **캐시**: Redis 7+
- **인증**: JWT (golang-jwt/jwt v5) + 레거시 다모앙 SSO 쿠키

## 아키텍처 (Clean Architecture 4계층)
```
Handler (internal/handler/)     → HTTP 요청/응답 처리
    ↓
Service (internal/service/)     → 비즈니스 로직
    ↓
Repository (internal/repository/) → 데이터 접근
    ↓
Domain (internal/domain/)       → 엔티티 & DTO
```

## 주요 디렉토리
- `cmd/api/` - API 서버 진입점 (DI 와이어링)
- `internal/handler/` - HTTP 핸들러
- `internal/service/` - 비즈니스 로직
- `internal/repository/` - 데이터 접근 레이어
- `internal/domain/` - 도메인 모델/DTO
- `internal/middleware/` - JWT, CORS, Cookie Auth
- `internal/common/` - 공통 응답/에러
- `internal/routes/` - 라우트 설정
- `internal/config/` - 설정 관리
- `pkg/` - 재사용 가능한 패키지 (jwt, auth, logger, redis)
- `configs/` - YAML 설정 파일
- `docs/` - API 문서 (Swagger, Roadmap)

## API Base URL
`/api/v2`

## 구현 완료 기능
- 인증 (JWT + 레거시 SSO)
- 게시글 CRUD, 검색, 페이지네이션
- 계층형 댓글 시스템
- 동적 메뉴 관리
- 캐시 기반 추천 게시물

## 데이터베이스 규칙
- 레거시 Gnuboard 호환 필수 (`g5_*` 테이블)
- 동적 게시판 테이블: `g5_write_{board_id}`
- 댓글: 게시글 테이블에 `wr_is_comment = 1`
- 회원: `g5_member` 테이블
