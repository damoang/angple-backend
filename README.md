# Angple Backend

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Fiber](https://img.shields.io/badge/Fiber-v2-00ACD7?style=flat)](https://gofiber.io)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

다모앙(damoang.net) 커뮤니티 차세대 백엔드 API 서버입니다. 기존 PHP 시스템을 Go로 마이그레이션하여 **800ms → 50ms 이하**의 응답 속도를 목표로 합니다.

---

## 🚀 Quick Start

```bash
# 1. MySQL + Redis 실행
docker-compose up -d

# 2. 환경 설정
cp .env.example .env

# 3. API 서버 실행
go run cmd/api/main.go
```

**서버 확인:**
```bash
curl http://localhost:8081/health
# {"status":"ok","time":1734163200}
```

📖 **상세 가이드:** [개발 환경 설정](#-개발-환경-설정)

---

## ✨ 주요 기능

### ✅ 구현 완료 (Production Ready)

| 기능 | 설명 | 엔드포인트 |
|------|------|------------|
| **인증** | JWT + 레거시 SSO 통합 | `/api/v2/auth/*` |
| **게시글** | CRUD, 검색, 페이지네이션 | `/api/v2/boards/{id}/posts` |
| **댓글** | 계층형 댓글 시스템 | `/api/v2/boards/{id}/posts/{id}/comments` |
| **메뉴** | 동적 메뉴 관리 (헤더/사이드바) | `/api/v2/menus/*` |
| **추천글** | 캐시 기반 추천 게시물 | `/api/v2/recommended/{period}` |

### 🚧 개발 예정 (Roadmap)

- [ ] 파일 업로드 (이미지 리사이징)
- [ ] 실시간 알림 (WebSocket)
- [ ] 통합 검색 (ElasticSearch)
- [ ] 투표/설문 시스템
- [ ] 관리자 대시보드

📋 **전체 로드맵:** [docs/api-roadmap.csv](docs/api-roadmap.csv)

---

## 📡 API 문서

### Swagger UI (추천 ⭐)

```bash
# Swagger UI 실행
docker run -p 8082:8080 \
  -e SWAGGER_JSON=/docs/swagger.yaml \
  -v $(pwd)/docs:/docs \
  swaggerapi/swagger-ui

# 브라우저에서 접속
open http://localhost:8082
```

### 주요 엔드포인트

<details>
<summary><b>📌 인증 (Authentication)</b></summary>

| 메서드 | 경로 | 인증 | 설명 |
|--------|------|------|------|
| POST | `/api/v2/auth/login` | ❌ | 로그인 (JWT 발급) |
| POST | `/api/v2/auth/refresh` | ❌ | 토큰 재발급 |
| GET | `/api/v2/auth/me` | 🍪 Cookie | 현재 사용자 (SSO) |
| GET | `/api/v2/auth/profile` | ✅ JWT | 사용자 프로필 |

**로그인 예제:**
```bash
curl -X POST http://localhost:8081/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"testuser","password":"password123"}'
```

</details>

<details>
<summary><b>📝 게시글 (Posts)</b></summary>

| 메서드 | 경로 | 인증 | 설명 |
|--------|------|------|------|
| GET | `/api/v2/boards/{board_id}/posts` | ❌ | 게시글 목록 |
| GET | `/api/v2/boards/{board_id}/posts/search` | ❌ | 게시글 검색 |
| GET | `/api/v2/boards/{board_id}/posts/{id}` | ❌ | 게시글 상세 |
| POST | `/api/v2/boards/{board_id}/posts` | ✅ | 게시글 작성 |
| PUT | `/api/v2/boards/{board_id}/posts/{id}` | ✅ | 게시글 수정 |
| DELETE | `/api/v2/boards/{board_id}/posts/{id}` | ✅ | 게시글 삭제 |

**게시글 작성 예제:**
```bash
curl -X POST http://localhost:8081/api/v2/boards/free/posts \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{"title":"제목","content":"내용"}'
```

</details>

<details>
<summary><b>💬 댓글 (Comments)</b></summary>

| 메서드 | 경로 | 인증 | 설명 |
|--------|------|------|------|
| GET | `/api/v2/boards/{board_id}/posts/{post_id}/comments` | ❌ | 댓글 목록 |
| POST | `/api/v2/boards/{board_id}/posts/{post_id}/comments` | ✅ | 댓글 작성 |
| PUT | `/api/v2/boards/{board_id}/posts/{post_id}/comments/{id}` | ✅ | 댓글 수정 |
| DELETE | `/api/v2/boards/{board_id}/posts/{post_id}/comments/{id}` | ✅ | 댓글 삭제 |

</details>

<details>
<summary><b>📂 메뉴 (Menus)</b></summary>

| 메서드 | 경로 | 인증 | 설명 |
|--------|------|------|------|
| GET | `/api/v2/menus` | ❌ | 전체 메뉴 |
| GET | `/api/v2/menus/sidebar` | ❌ | 사이드바 메뉴 |
| GET | `/api/v2/menus/header` | ❌ | 헤더 메뉴 |

</details>

<details>
<summary><b>⭐ 추천 게시물 (Recommended)</b></summary>

| 메서드 | 경로 | 인증 | 설명 |
|--------|------|------|------|
| GET | `/api/v2/recommended/{period}` | ❌ | 추천 게시물 (daily/weekly/monthly) |

</details>

---

## 🛠 기술 스택

### Backend
- **Go 1.23+** - 고성능 동시성 처리
- **Fiber v2** - Express 스타일 빠른 HTTP 프레임워크
- **GORM** - Go ORM with MySQL
- **golang-jwt/jwt v5** - JWT 인증

### Infrastructure
- **MySQL 8.0** - 기존 그누보드 DB (100GB+) 호환
- **Redis 7+** - 캐싱 및 세션
- **Docker Compose** - 로컬 개발 환경

### Architecture
```
┌─────────────────────────────────────────────┐
│         Handler (Presentation)              │  ← HTTP 요청/응답
├─────────────────────────────────────────────┤
│         Service (Business Logic)            │  ← 비즈니스 로직
├─────────────────────────────────────────────┤
│         Repository (Data Access)            │  ← 데이터 접근
├─────────────────────────────────────────────┤
│         Domain (Models & DTOs)              │  ← 도메인 모델
└─────────────────────────────────────────────┘
```

---

## 🏗 프로젝트 구조

```
angple-backend/
├── .docker/
│   └── mysql/init/         # MySQL 초기화 스크립트 (메뉴 seed 등)
├── cmd/
│   └── api/                # API 서버 엔트리포인트
├── internal/
│   ├── handler/            # HTTP 핸들러
│   ├── service/            # 비즈니스 로직
│   ├── repository/         # 데이터 접근 레이어
│   ├── domain/             # 도메인 모델
│   ├── middleware/         # JWT, CORS, Cookie Auth
│   ├── common/             # 공통 응답/에러
│   ├── routes/             # 라우트 설정
│   └── config/             # 설정 관리
├── pkg/
│   ├── jwt/                # JWT 유틸리티
│   ├── auth/               # 레거시 인증 호환
│   ├── logger/             # 로거
│   └── redis/              # Redis 클라이언트
├── configs/                # YAML 설정 파일
├── docs/                   # API 문서 (Swagger, Roadmap)
├── docker-compose.yml      # MySQL + Redis 환경
└── .env.example            # 환경 변수 예시
```

---

## 💻 개발 환경 설정

### 필수 요구사항

- **Go 1.23+**
- **Docker & Docker Compose**
- **Git**

### 설치 및 실행

#### 1. 저장소 클론

```bash
git clone https://github.com/damoang/angple-backend.git
cd angple-backend
```

#### 2. Docker 환경 시작

```bash
# MySQL + Redis 컨테이너 실행
docker-compose up -d

# 컨테이너 상태 확인
docker-compose ps
```

**Docker 구성:**
- **MySQL 8.0**: 포트 3307 → 3306 (메뉴 시스템 초기화 포함)
- **Redis 7**: 포트 6379

#### 3. 환경 설정

```bash
# .env 파일 생성
cp .env.example .env
```

**.env 주요 설정:**
```bash
# Environment
APP_ENV=local  # local, dev, staging, prod

# Database (Docker MySQL)
DB_HOST=localhost
DB_PORT=3307
DB_USER=angple_user
DB_PASSWORD=angple_pass_2024
DB_NAME=angple_db

# JWT
JWT_SECRET=your-development-secret-key
DAMOANG_JWT_SECRET=your-legacy-sso-secret  # 필수!

# API
API_PORT=8081
```

<details>
<summary><b>환경별 설정 가이드</b></summary>

| 환경 | APP_ENV | Port | DB Host | CORS | Mock Auth |
|------|---------|------|---------|------|-----------|
| **로컬** | local | 8081 | localhost:3307 | localhost:* | ✅ 활성화 |
| **개발** | dev | 8081 | dev-db.damoang.net | dev.damoang.net | ✅ 활성화 |
| **스테이징** | staging | 8081 | staging-db | staging.damoang.net | ❌ |
| **운영** | prod | 8081 | db.damoang.net | damoang.net | ❌ |

**Mock 인증:**
- `local`, `dev` 환경에서는 자동으로 "개발자" 계정으로 로그인
- 레거시 SSO 쿠키 없이도 API 테스트 가능

</details>

#### 4. 의존성 설치 및 실행

```bash
# Go 모듈 다운로드
go mod download

# API 서버 실행
go run cmd/api/main.go
```

**실행 확인:**
```bash
# Health Check
curl http://localhost:8081/health

# 메뉴 API 테스트
curl http://localhost:8081/api/v2/menus/sidebar
```

#### 5. 빌드 (선택)

```bash
# 바이너리 빌드
go build -o bin/api cmd/api/main.go

# 실행
./bin/api
```

---

## 🔐 인증 시스템

### JWT 토큰

- **Access Token**: 15분 (짧은 수명으로 보안 강화)
- **Refresh Token**: 7일 (토큰 재발급용)

### 레거시 통합

1. **Damoang SSO**: 기존 PHP 시스템의 `damoang_jwt` 쿠키 검증
2. **그누보드 비밀번호**: 3가지 포맷 호환
   - MySQL PASSWORD() 함수 (SHA1 of SHA1)
   - 단순 SHA1 해시
   - 평문 비밀번호 (매우 오래된 계정)

### 개발 환경 Mock 인증

```go
// local, dev 환경에서는 자동 로그인
User ID: "dev_user"
User Name: "개발자"
Level: 10 (관리자)
```

---

## 🗄 데이터베이스

### 그누보드 호환성

기존 그누보드 5.x 데이터베이스와 **100% 호환**:

- **테이블 접두사**: `g5_`
- **동적 게시판**: `g5_write_{board_id}` (예: `g5_write_free`)
- **댓글 구조**: 게시글 테이블에 `wr_is_comment = 1`로 저장

### 주요 테이블

```
g5_member                # 회원 정보
g5_write_{board_id}      # 게시판별 게시글/댓글
g5_board                 # 게시판 설정
g5_group                 # 게시판 그룹
menus                    # 메뉴 시스템 (신규)
```

### Docker MySQL 초기화

`.docker/mysql/init/` 스크립트가 자동 실행:
- `01-schema.sql`: 메뉴 테이블 생성
- `02-seed-menus.sql`: 기본 메뉴 29개 삽입

---

## 🧪 테스트

```bash
# 모든 테스트 실행
go test ./...

# 커버리지 포함
go test -cover ./...

# 특정 패키지
go test ./internal/service/...

# 특정 함수
go test -run TestAuthService ./internal/service/
```

---

## ❗ 자주 묻는 문제

<details>
<summary><b>Port 8081 already in use</b></summary>

**원인:** 이전 프로세스가 종료되지 않음

**해결:**
```bash
# 프로세스 확인 및 종료
lsof -ti :8081 | xargs kill -9

# 또는
pkill -f "go run cmd/api/main.go"
```

</details>

<details>
<summary><b>CORS error (Access-Control-Allow-Origin)</b></summary>

**원인:** 프론트엔드 origin이 허용 목록에 없음

**해결:**
```yaml
# configs/config.dev.yaml
cors:
  allow_origins: "http://localhost:5173, http://localhost:5174"
```

또는 `.env`:
```bash
CORS_ALLOW_ORIGINS="http://localhost:5173, http://localhost:5174"
```

</details>

<details>
<summary><b>Database connection failed</b></summary>

**원인:** MySQL 컨테이너가 실행되지 않음

**해결:**
```bash
# Docker 컨테이너 상태 확인
docker-compose ps

# MySQL 로그 확인
docker-compose logs mysql

# 재시작
docker-compose down
docker-compose up -d
```

</details>

<details>
<summary><b>DAMOANG_JWT_SECRET required</b></summary>

**원인:** 필수 환경 변수 누락

**해결:**
```bash
# .env 파일에 추가
DAMOANG_JWT_SECRET=your-legacy-sso-secret-key
```

</details>

---

## 📊 성능 벤치마크

### 목표 지표

| 지표 | 기존 PHP | Go 목표 | 현재 상태 |
|------|---------|---------|----------|
| **응답 시간 (P95)** | ~800ms | < 50ms | 측정 예정 |
| **처리량 (RPS)** | ~1,000 | > 10,000 | 측정 예정 |
| **메모리 사용** | ~512MB | ~128MB | 측정 예정 |
| **동시 접속** | 7천~2만 | 5만+ | 측정 예정 |

---

## 🤝 기여하기

기여를 환영합니다! 다음 절차를 따라주세요:

1. 이 저장소를 Fork 합니다
2. Feature 브랜치를 생성합니다 (`git checkout -b feature/amazing-feature`)
3. 변경사항을 커밋합니다 (`git commit -m 'Add amazing feature'`)
4. 브랜치에 Push 합니다 (`git push origin feature/amazing-feature`)
5. Pull Request를 생성합니다

### 코딩 컨벤션

- Go 표준 포맷 사용 (`gofmt`, `goimports`)
- 함수/메서드에 주석 작성
- 테스트 코드 포함
- Clean Architecture 패턴 준수

### 우선순위 작업

현재 도움이 필요한 작업들:

- [x] 메뉴 관리 시스템 ✅
- [x] 추천 API (파일 기반) ✅
- [ ] 비추천 API
- [ ] 파일 업로드 시스템 (이미지 변환)
- [ ] 회원 프로필 API
- [ ] 스크랩 기능
- [ ] 알림 시스템 (WebSocket)
- [ ] 통합 검색 (ElasticSearch)

📋 **상세 로드맵:** [docs/api-roadmap.csv](docs/api-roadmap.csv)

---

## 📚 관련 문서

- **[CLAUDE.md](CLAUDE.md)**: AI 개발 도구용 프로젝트 가이드
- **[docs/swagger.yaml](docs/swagger.yaml)**: OpenAPI 3.0 스펙
- **[docs/api-roadmap.csv](docs/api-roadmap.csv)**: API 개발 로드맵
- **[Wiki](https://github.com/damoang/angple-backend/wiki)**: 아키텍처 상세 설명

---

## 📝 라이선스

이 프로젝트는 MIT 라이선스를 따릅니다. 자세한 내용은 [LICENSE](LICENSE) 파일을 참고하세요.

---

## 📧 문의

- **웹사이트**: https://damoang.net
- **이슈**: https://github.com/damoang/angple-backend/issues
- **이메일**: sdk@damoang.net

---

**Made with ❤️ by SDK Co., Ltd.**
