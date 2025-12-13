# Angple Backend

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

다모앙(damoang.net) 커뮤니티 백엔드 API 서버입니다. 기존 PHP 기반 시스템을 Go로 점진적으로 마이그레이션하는 프로젝트입니다.

## 🎯 프로젝트 목표

- **성능 향상**: 응답 시간 800ms → 50ms 이하
- **확장성**: 동시 접속 7천~2만명 안정적 처리
- **유지보수성**: Clean Architecture 적용
- **레거시 호환**: 기존 그누보드 데이터베이스와 100% 호환

## ✨ 주요 기능

### ✅ 구현 완료
- **인증 시스템**: JWT 기반 인증, 그누보드 레거시 비밀번호 호환
- **게시글 관리**: CRUD, 검색, 페이지네이션
- **댓글 시스템**: 게시글별 댓글 CRUD
- **권한 관리**: 소유자 기반 수정/삭제 제어

### 🚧 개발 예정
- 추천/비추천 시스템
- 파일 업로드 (이미지, 첨부파일)
- 실시간 알림 (WebSocket)
- 통합 검색 (ElasticSearch)
- 관리자 기능

상세 로드맵은 [docs/api-roadmap.csv](docs/api-roadmap.csv) 참고

## 🛠 기술 스택

### Backend
- **Go 1.23+** - 높은 성능과 동시성
- **Fiber v2** - Express 스타일의 빠른 HTTP 프레임워크
- **GORM** - Go ORM with MySQL
- **golang-jwt/jwt v5** - JWT 인증

### Infrastructure
- **MySQL 8.0+** - 기존 데이터베이스 (100GB+)
- **Redis 7+** - 캐싱 및 세션
- **Docker** - 컨테이너 기반 배포
- **Docker Compose** - 로컬 개발 환경

## 🏗 아키텍처

### Clean Architecture

```
┌─────────────────────────────────────────────┐
│              HTTP Handler                    │  ← HTTP 요청/응답
├─────────────────────────────────────────────┤
│               Service                        │  ← 비즈니스 로직
├─────────────────────────────────────────────┤
│              Repository                      │  ← 데이터 접근
├─────────────────────────────────────────────┤
│              Database                        │  ← MySQL/Redis
└─────────────────────────────────────────────┘
```

### 프로젝트 구조

```
angple-backend/
├── cmd/
│   └── api/              # API 서버 엔트리포인트
├── internal/
│   ├── handler/          # HTTP 핸들러 (Presentation Layer)
│   ├── service/          # 비즈니스 로직 (Application Layer)
│   ├── repository/       # 데이터 접근 (Infrastructure Layer)
│   ├── domain/           # 도메인 모델 (Domain Layer)
│   ├── middleware/       # 미들웨어 (JWT, CORS, etc)
│   ├── common/           # 공통 응답/에러 정의
│   ├── routes/           # 라우트 설정
│   └── config/           # 설정 관리
├── pkg/
│   ├── jwt/              # JWT 유틸리티
│   ├── auth/             # 레거시 인증 호환
│   ├── logger/           # 로거
│   └── redis/            # Redis 클라이언트
├── configs/              # 설정 파일 (YAML)
├── docs/                 # API 문서 (Swagger, CSV)
└── deployments/          # Docker, Docker Compose
```

## 🚀 시작하기

### 필수 요구사항

- Go 1.23 이상
- Docker & Docker Compose
- MySQL 8.0+ (또는 기존 그누보드 DB 연결)
- Redis 7+

### 설치 및 실행

#### 1. 저장소 클론

```bash
git clone https://github.com/damoang/angple-backend.git
cd angple-backend
```

#### 2. 환경 설정

```bash
# 설정 파일 복사 및 수정
cp configs/config.dev.yaml.example configs/config.dev.yaml
```

`configs/config.dev.yaml` 수정:
```yaml
database:
  host: localhost
  port: 3306
  user: root
  password: your_password
  dbname: damoang

redis:
  host: localhost
  port: 6379
  password: ""

jwt:
  secret: your-super-secret-jwt-key
  expires_in: 900      # 15분
  refresh_in: 604800   # 7일
```

#### 3. 로컬 개발 환경 실행

```bash
# Docker Compose로 실행
docker-compose up -d

# 또는 직접 실행
go run cmd/api/main.go
```

#### 4. API 서버 접속 확인

```bash
curl http://localhost:8082/health
# {"status":"ok","time":1732766000}
```

### 빌드

```bash
# 의존성 설치
go mod download

# 빌드
go build -o bin/api cmd/api/main.go

# 실행
./bin/api
```

## 📡 API 문서

### Swagger UI

```bash
# Swagger UI 실행 (Docker)
docker run -p 8082:8080 \
  -e SWAGGER_JSON=/docs/swagger.yaml \
  -v $(pwd)/docs:/docs \
  swaggerapi/swagger-ui

# 접속: http://localhost:8082
```

### API 엔드포인트

#### 인증 (Auth)
```
POST   /api/v2/auth/login           # 로그인
POST   /api/v2/auth/refresh         # 토큰 재발급
GET    /api/v2/auth/profile         # 프로필 조회 (JWT 필요)
```

#### 게시글 (Posts)
```
GET    /api/v2/boards/{board_id}/posts              # 목록 조회
GET    /api/v2/boards/{board_id}/posts/search       # 검색
GET    /api/v2/boards/{board_id}/posts/{id}         # 상세 조회
POST   /api/v2/boards/{board_id}/posts              # 작성 (JWT 필요)
PUT    /api/v2/boards/{board_id}/posts/{id}         # 수정 (JWT 필요)
DELETE /api/v2/boards/{board_id}/posts/{id}         # 삭제 (JWT 필요)
```

#### 댓글 (Comments)
```
GET    /api/v2/boards/{board_id}/posts/{post_id}/comments        # 목록
GET    /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}   # 상세
POST   /api/v2/boards/{board_id}/posts/{post_id}/comments        # 작성 (JWT)
PUT    /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}   # 수정 (JWT)
DELETE /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}   # 삭제 (JWT)
```

### 사용 예제

```bash
# 1. 로그인
curl -X POST http://localhost:8082/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","password":"password123"}'

# Response:
# {
#   "data": {
#     "access_token": "eyJhbGciOiJIUzI1...",
#     "refresh_token": "eyJhbGciOiJIUzI1...",
#     "user": {...}
#   }
# }

# 2. 게시글 작성
curl -X POST http://localhost:8082/api/v2/boards/free/posts \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{"title":"제목","content":"내용","author":"user1"}'

# 3. 댓글 작성
curl -X POST http://localhost:8082/api/v2/boards/free/posts/1/comments \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{"content":"댓글 내용","author":"user1"}'
```

## 🔐 인증

### JWT 토큰

- **Access Token**: 15분 (짧은 수명으로 보안 강화)
- **Refresh Token**: 7일 (토큰 재발급용)

### 레거시 비밀번호 호환

기존 그누보드 사용자의 비밀번호 인증을 지원합니다:
- MySQL PASSWORD() 함수 (SHA1 of SHA1)
- 단순 SHA1 해시
- 평문 비밀번호 (매우 오래된 계정)

## 🧪 테스트

```bash
# 단위 테스트
go test ./...

# 커버리지 포함
go test -cover ./...

# 특정 패키지
go test ./internal/service/...
```

## 🗄 데이터베이스

### 그누보드 호환성

기존 그누보드 5.x 데이터베이스 스키마와 완벽 호환:
- 테이블 접두사: `g5_`
- 동적 게시판 테이블: `g5_write_{board_id}`
- 댓글: 게시글과 같은 테이블 (`wr_is_comment = 1`)

### 주요 테이블

```
g5_member                    # 회원
g5_write_{board_id}          # 게시판별 게시글/댓글
g5_board                     # 게시판 설정
g5_group                     # 게시판 그룹
```

## 📊 성능 벤치마크

### 목표 지표

| 지표 | 기존 PHP | Go 목표 | 현재 상태 |
|------|---------|---------|----------|
| 응답 시간 (P95) | ~800ms | < 50ms | 측정 예정 |
| 처리량 (RPS) | ~1,000 | > 10,000 | 측정 예정 |
| 메모리 사용량 | ~512MB | ~128MB | 측정 예정 |
| 동시 접속 | 7천~2만 | 5만+ | 측정 예정 |

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

- [ ] 추천/비추천 API 구현
- [ ] 파일 업로드 시스템 (이미지 변환)
- [ ] 회원 프로필 API
- [ ] 스크랩 기능
- [ ] 알림 시스템 (WebSocket)
- [ ] 통합 검색 (ElasticSearch)

자세한 내용은 [docs/api-roadmap.csv](docs/api-roadmap.csv) 참고

## 📝 라이선스

이 프로젝트는 MIT 라이선스를 따릅니다. 자세한 내용은 [LICENSE](LICENSE) 파일을 참고하세요.

## 📧 문의

- **웹사이트**: https://damoang.net
- **이슈**: https://github.com/damoang/angple-backend/issues

---

© 2025 SDK Co., Ltd. All rights reserved.
