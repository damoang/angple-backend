# ANGPLE Backend — 개발 프로세스

> 최종 수정: 2026-01-31 | 버전: v1.0

---

## 1. 개발 환경 설정

### 필수 도구

| 도구 | 버전 | 용도 |
|------|------|------|
| Go | 1.24+ | 런타임 |
| Docker + Compose | 최신 | 컨테이너 환경 |
| MySQL | 8.0+ | 데이터베이스 |
| Redis | 7+ | 캐시/세션 |
| golangci-lint | 최신 | 정적 분석 |
| make | - | 빌드 자동화 |

### 초기 설정

```bash
# 1. 저장소 클론
git clone https://github.com/damoang/angple-backend.git
cd angple-backend

# 2. 설정 파일 생성 (최초 1회)
cp configs/config.dev.yaml.example configs/config.dev.yaml
# config.dev.yaml에서 DB 접속 정보 수정

# 3. 의존성 설치
make deps

# 4. Docker 환경 시작
make docker-up

# 5. API 서버 실행
make dev
```

### 주요 명령어

> 전체 명령어: [`CLAUDE.md`](CLAUDE.md) "필수 명령어" 섹션 참조

```bash
make dev              # 로컬 API 서버 실행
make dev-gateway      # Gateway 실행
make test             # 전체 테스트
make test-coverage    # 커버리지 포함 테스트
make fmt              # 코드 포맷팅
make lint             # 린트 실행
make docker-up        # Docker 환경 시작
make docker-rebuild   # 재빌드 후 실행
```

### 포트 구성

| 서비스 | 개발 포트 | 프로덕션 포트 |
|--------|----------|-------------|
| API Server | 8081 | 8082 |
| Gateway | 8080 | 8083 |
| MySQL | 3307 | 3306 |
| Redis | 6381 | 6379 |

---

## 2. Git 워크플로우

### 브랜치 전략

```
main ──────────────────────────────── 프로덕션 (보호)
  └── develop ─────────────────────── 개발 통합
        ├── feature/추천시스템 ──────── 기능 개발
        ├── fix/로그인-에러 ──────────── 버그 수정
        ├── hotfix/보안-패치 ──────────── 긴급 수정
        └── refactor/서비스-분리 ────── 리팩토링
```

### 브랜치 네이밍

| 타입 | 형식 | 예시 |
|------|------|------|
| 기능 | `feature/{기능명}` | `feature/추천시스템` |
| 버그 | `fix/{이슈명}` | `fix/로그인-에러` |
| 긴급 | `hotfix/{설명}` | `hotfix/보안-패치` |
| 리팩 | `refactor/{대상}` | `refactor/서비스-분리` |

### 커밋 컨벤션

```
<type>(<scope>): <subject>

feat(auth): 소셜 로그인 구현
fix(post): 게시글 조회수 중복 증가 수정
refactor(repo): PostRepository 인터페이스 분리
docs(api): API 로드맵 Phase 2 추가
test(service): AuthService 단위 테스트 추가
chore(docker): compose 파일 최적화
```

| type | 용도 |
|------|------|
| `feat` | 새 기능 |
| `fix` | 버그 수정 |
| `refactor` | 리팩토링 (동작 변경 없음) |
| `docs` | 문서 수정 |
| `test` | 테스트 추가/수정 |
| `chore` | 빌드, 설정 등 |
| `perf` | 성능 개선 |

### PR 규칙

- PR 제목: 커밋 컨벤션과 동일한 형식
- PR 본문: 변경 내용, 테스트 방법, 스크린샷(UI 변경 시)
- 리뷰어: 최소 1명 승인 필요
- CI 통과 필수 (lint + test)

---

## 3. 코딩 표준

### Clean Architecture 패턴

> 상세 설명: [`CLAUDE.md`](CLAUDE.md) "아키텍처 핵심" 섹션 참조

```
Handler (Presentation Layer)
    → 요청 파싱, 응답 포맷, 입력 검증
Service (Business Logic)
    → 비즈니스 규칙, 트랜잭션, 권한, Hook 호출
Repository (Data Access)
    → DB 쿼리, 데이터 매핑, 캐시
```

**핵심 원칙:**
- 역방향 의존성 금지 (Repository → Service 호출 불가)
- Handler에서 직접 DB 접근 금지
- Service 간 순환 의존성 금지

### 네이밍 규칙

| 대상 | 규칙 | 예시 |
|------|------|------|
| 파일명 | snake_case | `post_handler.go` |
| 타입/구조체 | PascalCase | `PostHandler` |
| 함수/메서드 | PascalCase (exported) | `CreatePost()` |
| 비공개 함수 | camelCase | `validateInput()` |
| 상수 | UPPER_SNAKE_CASE | `MAX_PAGE_SIZE` |
| 패키지명 | 소문자 단일 단어 | `handler`, `service` |

### 에러 처리 패턴

```go
// Repository: 에러 전파
func (r *PostRepo) FindByID(boardID string, id int) (*domain.Post, error) {
    var post domain.Post
    if err := r.db.Table(tableName).Where("wr_id = ?", id).First(&post).Error; err != nil {
        return nil, err
    }
    return &post, nil
}

// Service: 비즈니스 에러 변환
func (s *PostService) GetPost(boardID string, id int) (*domain.Post, error) {
    post, err := s.repo.FindByID(boardID, id)
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, common.ErrNotFound
    }
    return post, err
}

// Handler: HTTP 응답 변환
func (h *PostHandler) GetPost(c *fiber.Ctx) error {
    post, err := h.service.GetPost(boardID, postID)
    if errors.Is(err, common.ErrNotFound) {
        return common.ErrorResponse(c, 404, "POST_NOT_FOUND", "게시글을 찾을 수 없습니다")
    }
    return common.SuccessResponse(c, 200, post, nil)
}
```

### 파일 크기 제한

- **최대 1000줄/파일** — 초과 시 분리
- Handler: 기능별 분리 (CRUD 그룹)
- Service: 복잡한 비즈니스 로직은 별도 파일
- Repository: 테이블 단위

---

## 4. 새 기능 추가 프로세스

### 단계별 절차

```
1. Domain 모델 정의
   └── internal/domain/{feature}.go
       - Request/Response DTO 구조체
       - 도메인 모델 (GORM 태그 포함)

2. Repository 구현
   └── internal/repository/{feature}_repo.go
       - 인터페이스 정의
       - GORM 쿼리 구현
       - 동적 테이블 처리 (그누보드)

3. Service 구현
   └── internal/service/{feature}_service.go
       - 비즈니스 로직
       - 권한 검증
       - 트랜잭션 관리

4. Handler 구현
   └── internal/handler/{feature}_handler.go
       - 입력 파싱/검증
       - Service 호출
       - 응답 포맷

5. Route 등록
   └── internal/routes/routes.go
       - 엔드포인트 매핑
       - 미들웨어 적용

6. DI 설정
   └── cmd/api/main.go
       - Repository → Service → Handler → Route 순서
```

### 예시: 스크랩 기능 추가

```go
// 1. domain/scrap.go
type ScrapRequest struct { PostID int `json:"post_id"` }
type ScrapResponse struct { ScrapID int `json:"scrap_id"` }

// 2. repository/scrap_repo.go
type ScrapRepository struct { db *gorm.DB }
func (r *ScrapRepository) Create(userID string, postID int) (*Scrap, error) { ... }

// 3. service/scrap_service.go
type ScrapService struct { repo *ScrapRepository }
func (s *ScrapService) ToggleScrap(userID string, postID int) error { ... }

// 4. handler/scrap_handler.go
type ScrapHandler struct { service *ScrapService }
func (h *ScrapHandler) ToggleScrap(c *fiber.Ctx) error { ... }

// 5. routes에 등록, 6. main.go에 DI 추가
```

---

## 5. 플러그인 개발 프로세스

> 상세 스펙: [`docs/specs/plugin-spec-v1.0.md`](docs/specs/plugin-spec-v1.0.md) 참조

### 플러그인 생성 절차

```
1. plugin.yaml 작성
   - name, version, title, requires 정의
   - hooks, routes, settings 선언

2. main.go 구현
   - Plugin 인터페이스 구현
   - Init(), Enable(), Disable() 메서드

3. Hook 핸들러 구현 (선택)
   - hooks/hooks.go
   - Core Hook에 연결

4. HTTP 핸들러 구현 (선택)
   - handlers/handler.go
   - /api/plugins/{name}/ 경로

5. 마이그레이션 작성 (선택)
   - migrations/001_init.up.sql
   - migrations/001_init.down.sql

6. 테스트 및 보안 검증
```

### 네이밍 규칙

| 항목 | 규칙 | 예시 |
|------|------|------|
| 플러그인명 | 소문자+하이픈 | `bookmark` |
| 테이블 | `{plugin}_{table}` | `bookmark_items` |
| API 경로 | `/api/plugins/{plugin}/` | `/api/plugins/bookmark/` |
| Hook 이름 | `{plugin}.{action}` | `bookmark.added` |

---

## 6. 테스트 전략

### 테스트 종류

| 종류 | 범위 | 도구 | 위치 |
|------|------|------|------|
| 단위 테스트 | Service, 유틸리티 | testing + testify | `*_test.go` (같은 디렉토리) |
| 통합 테스트 | Repository + DB | testing + testcontainers | `tests/integration/` |
| E2E 테스트 | API 엔드포인트 | httptest + Fiber | `tests/e2e/` |
| 부하 테스트 | 성능 검증 | k6 / wrk | `tests/load/` |

### 테스트 명령어

```bash
# 전체 테스트
make test

# 특정 패키지
go test ./internal/service/...

# 특정 함수
go test -v -run TestCreatePost ./internal/service

# 커버리지
make test-coverage
```

### 테스트 작성 가이드

```go
func TestPostService_CreatePost(t *testing.T) {
    // Given: 테스트 데이터 준비
    repo := &MockPostRepository{}
    service := NewPostService(repo)

    // When: 동작 실행
    post, err := service.CreatePost("free", "user1", &CreatePostRequest{
        Title:   "테스트 제목",
        Content: "테스트 내용",
    })

    // Then: 결과 검증
    assert.NoError(t, err)
    assert.Equal(t, "테스트 제목", post.Title)
}
```

---

## 7. 빌드 & 배포

### Docker 이미지 빌드

```bash
# 로컬 빌드
make build

# Docker 이미지 빌드
docker build -f deployments/docker/api.Dockerfile -t angple-api .

# Docker Compose 프로덕션
docker compose -f docker-compose.yml up -d
```

### CI/CD 파이프라인 (GitHub Actions)

```
Push/PR → lint.yml ──────── golangci-lint 검사
        → test.yml ──────── 테스트 실행
        → security.yml ───── 보안 스캔

Merge to main → docker-publish.yml ── Docker 이미지 빌드 & GHCR Push
              → deploy-prod.yml ───── AWS EC2 배포 (SSM)
```

### 배포 환경

| 환경 | 트리거 | 대상 |
|------|--------|------|
| Dev | develop 브랜치 push | 개발 서버 |
| Prod | main 브랜치 merge | AWS EC2 (i-038a1d1f) |

### 프로덕션 배포 흐름

1. PR 머지 → main
2. GitHub Actions 자동 트리거
3. Docker 이미지 빌드 + GHCR Push
4. AWS SSM으로 EC2에 배포 명령
5. EC2에서 이미지 pull + 컨테이너 재시작
6. Health check 확인

---

## 8. 코드 리뷰 체크리스트

### 필수 확인 항목

- [ ] **아키텍처**: Clean Architecture 레이어 분리 준수
- [ ] **DI 패턴**: main.go에서 의존성 주입 순서 올바른지
- [ ] **에러 처리**: 적절한 에러 코드 및 메시지 반환
- [ ] **인증/권한**: 필요한 엔드포인트에 미들웨어 적용
- [ ] **입력 검증**: 사용자 입력 서버측 검증 존재
- [ ] **SQL 안전성**: Prepared Statement 사용 (raw SQL 금지)
- [ ] **동적 테이블**: boardID 기반 테이블명 올바르게 생성
- [ ] **테스트**: 핵심 비즈니스 로직에 테스트 존재
- [ ] **파일 크기**: 1000줄 이하
- [ ] **네이밍**: 프로젝트 컨벤션 준수

### 보안 체크

- [ ] SQL Injection 취약점 없음
- [ ] 민감 정보 (비밀번호, 토큰) 로깅 안함
- [ ] CORS 설정 적절
- [ ] Rate Limiting 적용 (인증 관련 API)

---

## 9. 디버깅 가이드

### GORM SQL 로깅 활성화

```go
// cmd/api/main.go에서 로그 레벨 변경
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: gormlogger.Default.LogMode(gormlogger.Info),
})
```

### 자주 발생하는 문제

| 문제 | 원인 | 해결 |
|------|------|------|
| NOT NULL 제약 위반 | 그누보드 호환 모드 미적용 | `sql_mode=''` 설정 확인 |
| 테이블 없음 에러 | 동적 테이블명 미지정 | `db.Table(tableName)` 사용 |
| 401 Unauthorized | JWT 만료 | `/api/v2/auth/refresh`로 재발급 |
| 빈 응답 | GORM 컬럼 매핑 오류 | `gorm:"column:wr_*"` 태그 확인 |
| Connection refused | Docker 미실행 | `docker compose up -d` |

### 유용한 디버깅 명령어

```bash
# Health check
curl http://localhost:8081/health

# 로그인 테스트
curl -X POST http://localhost:8081/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","password":"test1234"}'

# Docker 로그 확인
make docker-logs

# 특정 서비스 로그
docker compose logs -f api
```
