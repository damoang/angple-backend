# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 프로젝트 개요

다모앙(damoang.net) 커뮤니티 백엔드 API 서버. 기존 PHP(그누보드) 기반 시스템을 Go로 마이그레이션하는 프로젝트.

- **목표**: 응답 시간 800ms → 50ms 이하, 동시 접속 7천~2만명 안정적 처리
- **특징**: 그누보드 레거시 데이터베이스와 100% 호환성 유지
- **기술 스택**: Go 1.23+, Fiber v2, GORM, MySQL 8.0+, Redis 7+, JWT

## 필수 명령어

### 개발 환경 실행
```bash
# 설정 파일 준비 (최초 1회)
cp configs/config.dev.yaml.example configs/config.dev.yaml

# 로컬 개발 환경 실행
make dev                    # API 서버만 실행
make dev-gateway           # Gateway 실행

# 또는 직접 실행
go run cmd/api/main.go
```

### 빌드
```bash
make build                 # 전체 빌드 (api + gateway)
make build-api            # API 서버만 빌드
make build-gateway        # Gateway만 빌드

# 빌드된 바이너리 실행
./bin/api
```

### 테스트
```bash
make test                 # 전체 테스트 실행
make test-coverage        # 커버리지 포함 테스트
go test -v ./...         # 전체 테스트 (상세 출력)

# 특정 패키지 테스트
go test ./internal/service/...
go test ./pkg/jwt/...

# 특정 테스트 함수만 실행
go test -v -run TestFunctionName ./internal/service
```

### Docker
```bash
make docker-up            # Docker Compose 실행
make docker-down          # Docker Compose 중지
make docker-logs          # 로그 확인
make docker-rebuild       # 재빌드 후 실행
```

### 코드 품질
```bash
make fmt                  # 코드 포맷팅
make lint                 # 린트 실행 (golangci-lint 필요)
make tidy                 # go.mod 정리
make deps                 # 의존성 다운로드
```

### 빠른 테스트
```bash
# Health check
curl http://localhost:8081/health

# 로그인 테스트
curl -X POST http://localhost:8081/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","password":"test1234"}'
```

## 아키텍처 핵심

### Clean Architecture 레이어 구조

```
Handler (Presentation)
    ↓
Service (Application/Business Logic)
    ↓
Repository (Data Access)
    ↓
Database (MySQL/Redis)
```

**중요 원칙:**
- Handler는 Service만 의존
- Service는 Repository만 의존
- Repository는 DB/외부 시스템과 통신
- 역방향 의존성 금지 (Repository가 Service 호출 불가)

### 의존성 주입 흐름

`cmd/api/main.go`에서 다음 순서로 DI:

```go
// 1. Repository 생성
memberRepo := repository.NewMemberRepository(db)
postRepo := repository.NewPostRepository(db)

// 2. Service 생성 (Repository 주입)
authService := service.NewAuthService(memberRepo, jwtManager)
postService := service.NewPostService(postRepo)

// 3. Handler 생성 (Service 주입)
authHandler := handler.NewAuthHandler(authService)
postHandler := handler.NewPostHandler(postService)

// 4. Routes 설정 (Handler 주입)
routes.Setup(app, postHandler, commentHandler, authHandler, jwtManager)
```

새로운 기능 추가 시 이 패턴을 따라야 함.

### 동적 테이블 처리 (그누보드 호환)

그누보드는 게시판마다 동적 테이블 사용 (`g5_write_{board_id}`):

```go
// ❌ 잘못된 방법 - 고정 테이블
type Post struct {
    ...
}
func (Post) TableName() string {
    return "g5_write_free"  // 고정!
}

// ✅ 올바른 방법 - 동적 테이블
// Repository에서 동적으로 테이블 지정
func (r *PostRepository) FindByID(boardID string, postID int) (*domain.Post, error) {
    tableName := fmt.Sprintf("g5_write_%s", boardID)
    var post domain.Post
    err := r.db.Table(tableName).Where("wr_id = ?", postID).First(&post).Error
    return &post, err
}
```

**주의:** 모든 게시글/댓글 관련 쿼리는 `boardID`를 매개변수로 받아 동적으로 테이블 결정.

### 댓글 구조 (중요!)

그누보드는 댓글을 별도 테이블이 아닌 **같은 테이블에 저장**:

```sql
-- 게시글
SELECT * FROM g5_write_free WHERE wr_is_comment = 0

-- 댓글
SELECT * FROM g5_write_free WHERE wr_is_comment = 1 AND wr_parent = {post_id}
```

코드에서 처리:
```go
// 댓글 조회
db.Table(tableName).
    Where("wr_parent = ? AND wr_is_comment = 1", postID).
    Find(&comments)

// 댓글 작성
comment.IsComment = 1
comment.ParentID = postID
db.Table(tableName).Create(&comment)
```

## 레거시 호환성

### 비밀번호 인증

그누보드는 시대별로 3가지 해싱 방식 사용 (`pkg/auth/legacy.go`):

1. **MySQL PASSWORD()** - `*` 접두사 + 40자 (SHA1 of SHA1)
2. **단순 SHA1** - 40자 해시
3. **평문** - 매우 오래된 계정

인증 시 `auth.VerifyGnuboardPassword()` 사용 필수.

### 테이블 접두사

모든 테이블은 `g5_` 접두사 사용:
- `g5_member` - 회원
- `g5_write_{board_id}` - 게시판별 게시글/댓글
- `g5_board` - 게시판 설정

### 컬럼 네이밍

그누보드 컬럼 → Go 필드 매핑 예시:
```go
type Post struct {
    ID        int    `gorm:"column:wr_id" json:"id"`
    Title     string `gorm:"column:wr_subject" json:"title"`
    Content   string `gorm:"column:wr_content" json:"content"`
    Author    string `gorm:"column:wr_name" json:"author"`
    AuthorID  string `gorm:"column:mb_id" json:"author_id"`
    Views     int    `gorm:"column:wr_hit" json:"views"`
}
```

**패턴:**
- 게시글 필드: `wr_*` (write)
- 회원 필드: `mb_*` (member)
- 카테고리: `ca_*` (category)

## 인증 및 권한

### JWT 토큰 구조

- **Access Token**: 15분 (짧은 수명, 보안 강화)
- **Refresh Token**: 7일 (토큰 재발급용)

```go
// JWT 클레임 구조
type Claims struct {
    UserID   string `json:"user_id"`
    MemberID string `json:"member_id"`
    jwt.RegisteredClaims
}
```

### 미들웨어 사용

```go
// 인증 필요한 엔드포인트
boards.Post("/:board_id/posts",
    middleware.JWTAuth(jwtManager),
    postHandler.CreatePost)

// 인증 불필요한 엔드포인트
boards.Get("/:board_id/posts", postHandler.ListPosts)
```

### 권한 검증 패턴

```go
// Service 레이어에서 소유자 검증
func (s *PostService) UpdatePost(boardID string, postID int, userID string, req *domain.UpdatePostRequest) error {
    post, err := s.repo.FindByID(boardID, postID)
    if err != nil {
        return err
    }

    // 소유자 확인
    if post.AuthorID != userID {
        return common.ErrForbidden
    }

    // 업데이트 진행...
}
```

## 에러 처리

### 표준 에러 정의 (`internal/common/errors.go`)

```go
var (
    ErrNotFound      = errors.New("resource not found")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrForbidden     = errors.New("forbidden")
    ErrBadRequest    = errors.New("bad request")
)
```

### 에러 응답 형식

```go
// Handler에서 에러 반환
if err != nil {
    return common.ErrorResponse(c, fiber.StatusNotFound, "POST_NOT_FOUND", "Post not found")
}

// 성공 응답
return common.SuccessResponse(c, fiber.StatusOK, data, meta)
```

## 설정 관리

### 설정 파일 경로

- 개발: `configs/config.dev.yaml`
- 운영: `configs/config.prod.yaml` (예정)

### 환경 변수 우선순위

YAML 파일 기본값 → 환경 변수로 오버라이드:

```bash
# 환경 변수로 오버라이드
export DB_HOST=production-db.example.com
export DB_PASSWORD=secret123
export JWT_SECRET=super-secret-key

go run cmd/api/main.go
```

지원 환경 변수 목록은 `internal/config/config.go:overrideFromEnv()` 참고.

### GORM SQL 모드 비활성화

그누보드 호환성을 위해 MySQL STRICT 모드 비활성화:

```go
// DSN에 sql_mode='' 포함
dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&sql_mode=''", ...)

// 세션별로도 설정
db.Exec("SET SESSION sql_mode = ''")
```

이유: NOT NULL 필드에 기본값 없이 INSERT 가능하도록.

## API 버전 관리

현재 버전: `/api/v2`

```go
api := app.Group("/api/v2")

// 인증
api.Group("/auth")

// 게시글
api.Group("/boards/:board_id/posts")

// 댓글
api.Group("/boards/:board_id/posts/:post_id/comments")
```

새 버전 추가 시 `/api/v3` 형태로 추가.

## 코딩 컨벤션

### 파일 구조

```
internal/
├── domain/           # 도메인 모델, Request/Response DTO
├── handler/          # HTTP 핸들러 (route → handler)
├── service/          # 비즈니스 로직
├── repository/       # 데이터베이스 접근
├── middleware/       # 미들웨어 (인증, 로깅 등)
├── common/           # 공통 응답/에러 정의
├── routes/           # 라우트 설정
└── config/           # 설정 로더

pkg/                  # 재사용 가능한 유틸리티
├── jwt/             # JWT 토큰 관리
├── auth/            # 레거시 인증 호환
├── logger/          # 로거
└── redis/           # Redis 클라이언트
```

### 네이밍 규칙

- **파일명**: snake_case (예: `post_handler.go`, `auth_service.go`)
- **타입/구조체**: PascalCase (예: `PostHandler`, `AuthService`)
- **함수/메서드**: camelCase (예: `createPost()`, `findByID()`)
- **상수**: UPPER_SNAKE_CASE (예: `MAX_PAGE_SIZE`)

### 주석 규칙

모든 export된 함수/타입에 주석 필수:

```go
// NewPostHandler creates a new post handler
func NewPostHandler(service *service.PostService) *PostHandler {
    return &PostHandler{service: service}
}

// Post represents a board post in Gnuboard structure
type Post struct {
    // ...
}
```

## 성능 최적화

### Connection Pool 설정

```go
// cmd/api/main.go
sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)     // 기본: 10
sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)     // 기본: 100
sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)  // 기본: 3600s
```

### 인덱스 활용

그누보드 테이블에 이미 인덱스 존재. 쿼리 시 인덱스 활용:

```go
// ✅ 인덱스 사용 (wr_id는 PRIMARY KEY)
db.Where("wr_id = ?", postID).First(&post)

// ✅ 인덱스 사용 (mb_id는 INDEX)
db.Where("mb_id = ?", memberID).First(&member)

// ❌ 인덱스 미사용 (풀스캔)
db.Where("wr_content LIKE ?", "%keyword%").Find(&posts)
```

### Redis 캐싱 (Phase 3 예정)

현재는 연결만 수립. 향후 다음 데이터 캐싱 예정:
- 게시판 설정 (`g5_board`)
- 인기 게시글
- 사용자 세션

## 다음 구현 예정 기능

### Phase 1 (우선순위 높음)
- 추천/비추천 시스템 (`wr_good`, `wr_nogood` 컬럼 활용)
- 파일 업로드 (이미지, 첨부파일 - `g5_board_file` 테이블)
- 회원 프로필 API

### Phase 2
- 스크랩 (`g5_scrap` 테이블)
- 메모 (`g5_memo` 테이블)
- 쪽지 (`g5_write_*` 테이블)

### Phase 3
- 실시간 알림 (WebSocket)
- Redis 캐싱
- ElasticSearch 통합 검색

자세한 로드맵은 `docs/api-roadmap.csv` 참고.

## 디버깅 팁

### GORM SQL 로깅 활성화

```go
// cmd/api/main.go
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: gormlogger.Default.LogMode(gormlogger.Info), // SQL 쿼리 출력
})
```

개발 중에는 `gormlogger.Info`로 설정하여 모든 SQL 쿼리 확인 가능.

### 자주 발생하는 문제

1. **NOT NULL 제약 위반**
   - 해결: `sql_mode=''` 설정 확인

2. **동적 테이블 미지정**
   - 증상: `Table 'damoang.g5_write_free' doesn't exist` (다른 board_id 사용 시)
   - 해결: Repository에서 `db.Table(tableName)` 사용

3. **JWT 토큰 만료**
   - 증상: 401 Unauthorized
   - 해결: `/api/v2/auth/refresh`로 토큰 재발급

## 중요 참고 문서

- API 명세: `docs/swagger.yaml`
- 로드맵: `docs/api-roadmap.csv`
- README: `README.md`