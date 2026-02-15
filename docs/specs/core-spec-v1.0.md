# ANGPLE Core Specification v1.0

> Core 개발자 및 플러그인 개발자를 위한 Core 시스템 공식 규약 문서
> SDK Corporation, 2026년 1월

---

## 목차

1. [Core 철학과 범위](#1-core-철학과-범위)
2. [아키텍처](#2-아키텍처)
3. [데이터베이스 스키마](#3-데이터베이스-스키마)
4. [API 규약](#4-api-규약)
5. [인증/인가 시스템](#5-인증인가-시스템)
6. [Hook 시스템](#6-hook-시스템)
7. [설정 시스템](#7-설정-시스템)
8. [보안 가이드라인](#8-보안-가이드라인)
9. [프론트엔드 Core](#9-프론트엔드-core)
10. [확장 및 연동](#10-확장-및-연동)

---

## 1. Core 철학과 범위

### 1.1 Core 최소주의 원칙

ANGPLE Core는 커뮤니티 운영에 **필수적인 기능만** 포함합니다. 이 원칙은 [플러그인 스펙 §1.1](plugin-spec-v1.0.md#1-철학과-원칙)과 일관됩니다.

| 원칙 | 설명 |
|------|------|
| **최소 기능** | Core는 게시판, 회원, 댓글, 파일, 알림 등 커뮤니티의 뼈대만 제공 |
| **확장점 제공** | 모든 추가 기능은 Hook, Slot, Meta 테이블을 통해 플러그인이 확장 |
| **안정성 우선** | Core API의 Breaking Change는 메이저 버전에서만 허용 |

### 1.2 Core에 포함되는 것 vs 플러그인으로 빠져야 하는 것

**Core에 포함:**

| 기능 | 이유 |
|------|------|
| 회원 가입/로그인/프로필 | 모든 커뮤니티의 기본 기능 |
| 게시판 CRUD | 커뮤니티의 핵심 콘텐츠 단위 |
| 댓글 시스템 | 게시글에 대한 기본 상호작용 |
| 파일 업로드 | 게시글/댓글에 필수적인 미디어 첨부 |
| 알림 시스템 | 사용자 참여를 위한 기본 기능 |
| 카테고리/태그 | 콘텐츠 분류를 위한 기본 구조 |
| 권한/레벨 시스템 | 커뮤니티 운영의 기본 제어 수단 |
| 관리자 대시보드 | 사이트 운영을 위한 필수 도구 |

**플러그인으로 분리:**

| 기능 | 이유 |
|------|------|
| 북마크/스크랩 | 선택적 부가 기능 |
| 추천/비추천 | 커뮤니티마다 정책이 다름 |
| 배너 광고 | 수익화는 선택 사항 |
| 검색 엔진 (Elasticsearch) | 고급 검색은 모든 사이트에 필요하지 않음 |
| 실시간 채팅 | 커뮤니티 성격에 따라 불필요 |
| SEO 도구 | 운영 정책에 따라 상이 |
| 통계/분석 | 외부 서비스 연동도 가능 |
| 소셜 로그인 | OAuth 프로바이더는 사이트마다 다름 |

---

## 2. 아키텍처

### 2.1 전체 시스템 구성도

```
┌──────────────────────────────────────────────────────────────────┐
│                        사용자 브라우저                             │
└──────────┬─────────────────────────────────────┬─────────────────┘
           │                                     │
           ▼                                     ▼
┌─────────────────────┐             ┌─────────────────────┐
│   angple (Web)      │             │   angple (Admin)    │
│   SvelteKit 5       │             │   SvelteKit 5       │
│   Port: 5173        │             │   Port: 5174        │
└──────────┬──────────┘             └──────────┬──────────┘
           │                                     │
           └──────────────┬──────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │  angple-backend       │
              │  Go / Fiber           │
              │  Port: 8080           │
              │                       │
              │  ┌─────────────────┐  │
              │  │ Plugin Manager  │  │
              │  │ Hook System     │  │
              │  │ Auth (JWT+SSO)  │  │
              │  └─────────────────┘  │
              └───────┬───────┬───────┘
                      │       │
              ┌───────┘       └───────┐
              ▼                       ▼
    ┌──────────────────┐    ┌──────────────────┐
    │   MySQL 8.0      │    │   Redis 7+       │
    │   (Read/Write)   │    │   (Cache/Session) │
    └──────────────────┘    └──────────────────┘
```

### 2.2 Clean Architecture

```
┌─────────────────────────────────────────────┐
│  Handler (Presentation Layer)               │
│  - HTTP 요청 파싱, 응답 포맷팅              │
│  - 입력 검증 (기본)                         │
│  - 인증 미들웨어 적용                       │
├─────────────────────────────────────────────┤
│  Service (Application/Business Logic)       │
│  - 비즈니스 규칙 적용                       │
│  - 트랜잭션 관리                            │
│  - 권한 검증                                │
│  - Hook 호출 (before/after)                 │
├─────────────────────────────────────────────┤
│  Repository (Data Access Layer)             │
│  - DB 쿼리 실행                             │
│  - 데이터 매핑 (DB ↔ Domain)               │
│  - 캐시 조회/저장                           │
├─────────────────────────────────────────────┤
│  Database / External Services               │
│  - MySQL, Redis, S3 등                      │
└─────────────────────────────────────────────┘
```

**의존성 규칙:**
- Handler → Service (허용)
- Service → Repository (허용)
- Repository → Database (허용)
- 역방향 의존성 금지 (Repository가 Service 호출 불가)

### 2.3 의존성 주입 패턴

```go
// cmd/api/main.go 에서 수동 DI
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

새로운 기능 추가 시 이 패턴을 반드시 따릅니다. DI 프레임워크는 사용하지 않으며, 생성자 주입 방식을 유지합니다.

---

## 3. 데이터베이스 스키마

### 3.1 버전 전략

| 버전 | 데이터베이스 | 상태 |
|------|-------------|------|
| **v1** | 그누보드 DB (`g5_*`) | 현재 개발 중 |
| **v2** | 신규 설계 DB | 이 문서의 기준 |

> v1 레거시 스키마에 대한 자세한 내용은 [API Versioning 문서](api-versioning.md)를 참조하세요.
> 아래 스키마는 모두 **v2 신규 설계** 기준입니다.

### 3.2 Core 테이블 목록

#### users (사용자)

```sql
CREATE TABLE users (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    username    VARCHAR(50) UNIQUE NOT NULL COMMENT '사용자 ID',
    email       VARCHAR(255) UNIQUE NOT NULL COMMENT '이메일',
    password    VARCHAR(255) NOT NULL COMMENT '해시된 비밀번호',
    nickname    VARCHAR(100) NOT NULL COMMENT '닉네임',
    level       TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '사용자 레벨 (1-10)',
    status      ENUM('active', 'inactive', 'banned') NOT NULL DEFAULT 'active',
    avatar_url  VARCHAR(500) DEFAULT NULL COMMENT '프로필 이미지 URL',
    bio         TEXT DEFAULT NULL COMMENT '자기소개',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_level (level),
    KEY idx_status (status),
    KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### boards (게시판)

```sql
CREATE TABLE boards (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    slug        VARCHAR(50) UNIQUE NOT NULL COMMENT 'URL 식별자',
    name        VARCHAR(100) NOT NULL COMMENT '게시판 이름',
    description TEXT DEFAULT NULL COMMENT '게시판 설명',
    category_id BIGINT UNSIGNED DEFAULT NULL COMMENT '카테고리 ID',
    settings    JSON DEFAULT NULL COMMENT '게시판별 설정 (JSON)',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    order_num   INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '정렬 순서',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_is_active (is_active),
    KEY idx_category_id (category_id),
    KEY idx_order (order_num)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### posts (게시글)

```sql
CREATE TABLE posts (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    board_id    BIGINT UNSIGNED NOT NULL COMMENT '게시판 ID',
    user_id     BIGINT UNSIGNED NOT NULL COMMENT '작성자 ID',
    title       VARCHAR(255) NOT NULL COMMENT '제목',
    content     MEDIUMTEXT NOT NULL COMMENT '본문',
    status      ENUM('draft', 'published', 'deleted') NOT NULL DEFAULT 'published',
    view_count  INT UNSIGNED NOT NULL DEFAULT 0,
    comment_count INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '댓글 수 (캐시)',
    is_notice   BOOLEAN NOT NULL DEFAULT FALSE COMMENT '공지 여부',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_board_status (board_id, status, created_at),
    KEY idx_user_id (user_id),
    KEY idx_created_at (created_at),
    FOREIGN KEY (board_id) REFERENCES boards(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### comments (댓글)

```sql
CREATE TABLE comments (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    post_id     BIGINT UNSIGNED NOT NULL COMMENT '게시글 ID',
    user_id     BIGINT UNSIGNED NOT NULL COMMENT '작성자 ID',
    parent_id   BIGINT UNSIGNED DEFAULT NULL COMMENT '부모 댓글 ID (대댓글)',
    content     TEXT NOT NULL,
    depth       TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '댓글 깊이',
    status      ENUM('active', 'deleted') NOT NULL DEFAULT 'active',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    KEY idx_post_id (post_id, status, created_at),
    KEY idx_user_id (user_id),
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (parent_id) REFERENCES comments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### categories (카테고리)

```sql
CREATE TABLE categories (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    parent_id   BIGINT UNSIGNED DEFAULT NULL,
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(50) UNIQUE NOT NULL,
    description TEXT DEFAULT NULL,
    order_num   INT UNSIGNED NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_parent_id (parent_id),
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### tags (태그)

```sql
CREATE TABLE tags (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    name        VARCHAR(50) UNIQUE NOT NULL,
    slug        VARCHAR(50) UNIQUE NOT NULL,
    post_count  INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '사용 횟수 (캐시)',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE post_tags (
    post_id     BIGINT UNSIGNED NOT NULL,
    tag_id      BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (post_id, tag_id),
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### files (첨부파일)

```sql
CREATE TABLE files (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    post_id     BIGINT UNSIGNED DEFAULT NULL COMMENT '연결된 게시글',
    comment_id  BIGINT UNSIGNED DEFAULT NULL COMMENT '연결된 댓글',
    user_id     BIGINT UNSIGNED NOT NULL COMMENT '업로더',
    original_name VARCHAR(255) NOT NULL COMMENT '원본 파일명',
    stored_name VARCHAR(255) NOT NULL COMMENT '저장된 파일명',
    mime_type   VARCHAR(100) NOT NULL,
    file_size   BIGINT UNSIGNED NOT NULL COMMENT '바이트 단위',
    storage_path VARCHAR(500) NOT NULL COMMENT '저장 경로',
    download_count INT UNSIGNED NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_post_id (post_id),
    KEY idx_user_id (user_id),
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE SET NULL,
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE SET NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### notifications (알림)

```sql
CREATE TABLE notifications (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT UNSIGNED NOT NULL COMMENT '수신자',
    type        VARCHAR(50) NOT NULL COMMENT '알림 타입 (comment, mention, system 등)',
    title       VARCHAR(255) NOT NULL,
    content     TEXT DEFAULT NULL,
    link        VARCHAR(500) DEFAULT NULL COMMENT '연결 URL',
    is_read     BOOLEAN NOT NULL DEFAULT FALSE,
    sender_id   BIGINT UNSIGNED DEFAULT NULL COMMENT '발신자 (시스템 알림은 NULL)',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_user_unread (user_id, is_read, created_at),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### sessions (세션)

```sql
CREATE TABLE sessions (
    id          VARCHAR(128) PRIMARY KEY COMMENT '세션 ID (Refresh Token Hash)',
    user_id     BIGINT UNSIGNED NOT NULL,
    user_agent  VARCHAR(500) DEFAULT NULL,
    ip_address  VARCHAR(45) DEFAULT NULL,
    expires_at  TIMESTAMP NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_user_id (user_id),
    KEY idx_expires_at (expires_at),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 3.3 Meta 테이블 (플러그인 확장용)

Meta 테이블은 플러그인이 Core 테이블을 수정하지 않고도 추가 데이터를 저장할 수 있는 확장점입니다.
[플러그인 스펙 §4.2](plugin-spec-v1.0.md#4-데이터베이스-규칙)와 동일한 구조입니다.

```sql
CREATE TABLE user_meta (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT UNSIGNED NOT NULL,
    namespace   VARCHAR(64) NOT NULL COMMENT '플러그인 이름',
    meta_key    VARCHAR(128) NOT NULL,
    meta_value  JSON,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_user_ns_key (user_id, namespace, meta_key),
    KEY idx_namespace (namespace),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE post_meta (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    post_id     BIGINT UNSIGNED NOT NULL,
    namespace   VARCHAR(64) NOT NULL,
    meta_key    VARCHAR(128) NOT NULL,
    meta_value  JSON,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_post_ns_key (post_id, namespace, meta_key),
    KEY idx_namespace (namespace),
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE comment_meta (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    comment_id  BIGINT UNSIGNED NOT NULL,
    namespace   VARCHAR(64) NOT NULL,
    meta_key    VARCHAR(128) NOT NULL,
    meta_value  JSON,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_comment_ns_key (comment_id, namespace, meta_key),
    KEY idx_namespace (namespace),
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE option_meta (
    id          BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    namespace   VARCHAR(64) NOT NULL COMMENT '플러그인 이름 또는 "core"',
    meta_key    VARCHAR(128) NOT NULL,
    meta_value  JSON,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_ns_key (namespace, meta_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 3.4 플러그인 관리 테이블

```sql
CREATE TABLE plugin_installations (
    id            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    plugin_name   VARCHAR(100) UNIQUE NOT NULL,
    version       VARCHAR(50) NOT NULL,
    status        ENUM('enabled', 'disabled', 'error') NOT NULL DEFAULT 'disabled',
    installed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enabled_at    TIMESTAMP DEFAULT NULL,
    config        JSON DEFAULT NULL,
    error_message TEXT DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE plugin_settings (
    id            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    plugin_name   VARCHAR(100) NOT NULL,
    setting_key   VARCHAR(200) NOT NULL,
    setting_value TEXT DEFAULT NULL,

    UNIQUE KEY uk_plugin_setting (plugin_name, setting_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE plugin_events (
    id            BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    plugin_name   VARCHAR(100) NOT NULL,
    event_type    ENUM('installed', 'enabled', 'disabled', 'uninstalled', 'error') NOT NULL,
    details       JSON DEFAULT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_plugin_name (plugin_name),
    KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 3.5 v1(그누보드) → v2 마이그레이션 참고

> 상세 마이그레이션 SQL은 [API Versioning 문서](api-versioning.md#마이그레이션-가이드)를 참조하세요.

**핵심 변경점:**

| v1 (그누보드) | v2 (신규) | 비고 |
|--------------|----------|------|
| `g5_member` | `users` | `mb_id` → `username`, `mb_nick` → `nickname` |
| `g5_write_{board}` | `posts` + `comments` | 게시글/댓글 분리, 단일 테이블 |
| `g5_board` | `boards` | `settings` JSON 컬럼 추가 |
| `g5_board_file` | `files` | 범용 파일 테이블로 통합 |
| (없음) | `*_meta` | 플러그인 확장용 Meta 테이블 신규 |

---

## 4. API 규약

### 4.1 Base URL

```
/api/v2/                # v2 신규 설계 API (목표)
/api/v1/                # v1 레거시 호환 API (현재 /api/v2로 운영 중)
/api/plugins/{name}/    # 플러그인 전용 API
```

### 4.2 인증 헤더

```
Authorization: Bearer {accessToken}
```

또는 httpOnly 쿠키 기반 인증 (권장):
```
Cookie: accessToken={jwt}; refreshToken={jwt}
```

### 4.3 응답 형식 표준

**성공 응답:**

```json
{
    "success": true,
    "data": { ... }
}
```

**목록 응답 (페이지네이션 포함):**

```json
{
    "success": true,
    "data": [ ... ],
    "meta": {
        "page": 1,
        "per_page": 20,
        "total": 150,
        "total_pages": 8
    }
}
```

**에러 응답:**

```json
{
    "success": false,
    "error": {
        "code": "NOT_FOUND",
        "message": "요청한 리소스를 찾을 수 없습니다",
        "details": {}
    }
}
```

### 4.4 페이지네이션 표준

| 파라미터 | 기본값 | 최대값 | 설명 |
|---------|--------|--------|------|
| `page` | 1 | - | 페이지 번호 |
| `per_page` | 20 | 100 | 페이지당 항목 수 |
| `sort` | `created_at` | - | 정렬 기준 |
| `order` | `desc` | - | `asc` 또는 `desc` |

### 4.5 에러 코드 체계

| 코드 | HTTP Status | 설명 |
|------|-------------|------|
| `BAD_REQUEST` | 400 | 잘못된 요청 파라미터 |
| `UNAUTHORIZED` | 401 | 인증 필요 |
| `FORBIDDEN` | 403 | 권한 부족 |
| `NOT_FOUND` | 404 | 리소스 없음 |
| `CONFLICT` | 409 | 리소스 충돌 (중복 등) |
| `VALIDATION_ERROR` | 422 | 입력 검증 실패 |
| `RATE_LIMITED` | 429 | 요청 제한 초과 |
| `INTERNAL_ERROR` | 500 | 서버 내부 오류 |

### 4.6 Core API 엔드포인트 요약

**인증:**
```
POST   /api/v2/auth/login           # 로그인
POST   /api/v2/auth/register        # 회원가입
POST   /api/v2/auth/refresh         # 토큰 재발급
POST   /api/v2/auth/logout          # 로그아웃
```

**게시글:**
```
GET    /api/v2/boards/:board_id/posts              # 목록
POST   /api/v2/boards/:board_id/posts              # 작성
GET    /api/v2/boards/:board_id/posts/:post_id     # 상세
PUT    /api/v2/boards/:board_id/posts/:post_id     # 수정
DELETE /api/v2/boards/:board_id/posts/:post_id     # 삭제
```

**댓글:**
```
GET    /api/v2/boards/:board_id/posts/:post_id/comments          # 목록
POST   /api/v2/boards/:board_id/posts/:post_id/comments          # 작성
PUT    /api/v2/boards/:board_id/posts/:post_id/comments/:id      # 수정
DELETE /api/v2/boards/:board_id/posts/:post_id/comments/:id      # 삭제
```

**회원:**
```
GET    /api/v2/users/me             # 내 정보
PUT    /api/v2/users/me             # 내 정보 수정
GET    /api/v2/users/:username      # 프로필 조회
```

**알림:**
```
GET    /api/v2/notifications        # 알림 목록
PUT    /api/v2/notifications/:id/read   # 읽음 처리
PUT    /api/v2/notifications/read-all   # 전체 읽음
```

**관리자:**
```
GET    /api/v2/admin/boards         # 게시판 목록
POST   /api/v2/admin/boards         # 게시판 생성
PUT    /api/v2/admin/boards/:id     # 게시판 수정
DELETE /api/v2/admin/boards/:id     # 게시판 삭제
GET    /api/v2/admin/users          # 회원 관리
GET    /api/v2/admin/settings       # 설정 조회
PUT    /api/v2/admin/settings       # 설정 변경
```

---

## 5. 인증/인가 시스템

### 5.1 JWT 토큰 구조

**Access Token (수명: 15분):**

```json
{
    "user_id": "username",
    "member_id": "12345",
    "level": 5,
    "exp": 1700000000,
    "iat": 1699999100,
    "iss": "angple"
}
```

**Refresh Token (수명: 7일):**

```json
{
    "user_id": "username",
    "token_id": "uuid-v4",
    "exp": 1700604800,
    "iat": 1699999100,
    "iss": "angple"
}
```

### 5.2 토큰 저장 및 전달

| 토큰 | 저장 위치 | 전달 방식 |
|------|----------|----------|
| Access Token | httpOnly Cookie | `Cookie: accessToken={jwt}` |
| Refresh Token | httpOnly Cookie | `Cookie: refreshToken={jwt}` |

> **보안 주의**: Access Token을 localStorage에 저장하지 마세요. XSS 공격에 취약합니다.

**쿠키 설정:**

```
Set-Cookie: accessToken={jwt}; HttpOnly; Secure; SameSite=Strict; Path=/api; Max-Age=900
Set-Cookie: refreshToken={jwt}; HttpOnly; Secure; SameSite=Strict; Path=/api/v2/auth; Max-Age=604800
```

### 5.3 레거시 SSO (damoang_jwt 쿠키)

그누보드(PHP)와의 동시 운영 기간 동안, `damoang_jwt` 쿠키를 통한 SSO를 지원합니다:

```
1. 사용자가 PHP 사이트에서 로그인
2. PHP가 damoang_jwt 쿠키 발급
3. Go 백엔드가 damoang_jwt 검증 → Access Token 발급
4. 프론트엔드는 Access Token으로 API 호출
```

### 5.4 권한 레벨 시스템

| 레벨 | 이름 | 설명 |
|------|------|------|
| 1 | 신규 회원 | 기본 읽기만 가능 |
| 2 | 일반 회원 | 글쓰기, 댓글 가능 |
| 3 | 인증 회원 | 파일 업로드, 추가 게시판 접근 |
| 5 | 우수 회원 | 특별 게시판 접근 |
| 8 | 부관리자 | 게시글/댓글 관리 |
| 9 | 관리자 | 게시판/회원 관리 |
| 10 | 최고관리자 | 모든 권한 (사이트 설정 포함) |

**권한 검증 패턴:**

```go
// Service에서 레벨 체크
func (s *PostService) CreatePost(userLevel int, ...) error {
    if userLevel < 2 {
        return common.ErrForbidden
    }
    // ...
}

// 미들웨어에서 레벨 체크
func RequireLevel(minLevel int) fiber.Handler {
    return func(c *fiber.Ctx) error {
        user := c.Locals("user").(*Claims)
        if user.Level < minLevel {
            return common.ErrorResponse(c, 403, "FORBIDDEN", "권한이 부족합니다")
        }
        return c.Next()
    }
}
```

### 5.5 OAuth 프로바이더 통합

OAuth 소셜 로그인은 **플러그인**으로 구현합니다. Core는 다음 확장점만 제공:

- `user.before_register` Hook: OAuth로 받은 프로필 데이터 주입
- `user.after_login` Hook: 소셜 로그인 후 처리
- `user_meta` 테이블: 소셜 계정 연결 정보 저장

---

## 6. Hook 시스템

Core가 제공하는 확장점입니다. [플러그인 스펙 §5](plugin-spec-v1.0.md#5-hook-시스템)와 동일한 시스템입니다.

### 6.1 Hook 타입

| 타입 | 설명 | 반환값 |
|------|------|--------|
| **Action Hook** | 이벤트 발생 시 실행. 부가 작업(알림, 로그) 수행 | 없음 |
| **Filter Hook** | 데이터를 변환하거나 처리를 가로챌 수 있음 | 변환된 데이터 |

### 6.2 백엔드 Hook 목록

**Content Hooks:**

| Hook | 타입 | 설명 | 파라미터 |
|------|------|------|---------|
| `post.before_create` | Filter | 글 생성 전 | `(ctx, *CreatePostRequest)` |
| `post.after_create` | Action | 글 생성 후 | `(ctx, *Post)` |
| `post.before_update` | Filter | 글 수정 전 | `(ctx, *UpdatePostRequest)` |
| `post.after_update` | Action | 글 수정 후 | `(ctx, *Post)` |
| `post.before_delete` | Filter | 글 삭제 전 (삭제 방지 가능) | `(ctx, postID)` |
| `post.after_delete` | Action | 글 삭제 후 | `(ctx, postID)` |
| `post.content` | Filter | 글 내용 렌더링 시 | `(ctx, content string)` |
| `comment.before_create` | Filter | 댓글 생성 전 | `(ctx, *CreateCommentRequest)` |
| `comment.after_create` | Action | 댓글 생성 후 | `(ctx, *Comment)` |
| `comment.before_delete` | Filter | 댓글 삭제 전 | `(ctx, commentID)` |
| `comment.after_delete` | Action | 댓글 삭제 후 | `(ctx, commentID)` |

**User Hooks:**

| Hook | 타입 | 설명 | 파라미터 |
|------|------|------|---------|
| `user.before_register` | Filter | 회원가입 전 (가입 거부 가능) | `(ctx, *RegisterRequest)` |
| `user.after_register` | Action | 회원가입 후 | `(ctx, *User)` |
| `user.after_login` | Action | 로그인 후 | `(ctx, *User)` |
| `user.after_logout` | Action | 로그아웃 후 | `(ctx, userID)` |
| `user.permission_check` | Filter | 권한 확인 시 (권한 확장) | `(ctx, userID, permission)` |

**Admin Hooks:**

| Hook | 타입 | 설명 | 파라미터 |
|------|------|------|---------|
| `admin.menu` | Filter | 관리자 메뉴 확장 | `(ctx, []MenuItem)` |
| `admin.dashboard` | Filter | 대시보드 위젯 추가 | `(ctx, []DashboardWidget)` |
| `admin.settings` | Filter | 설정 페이지 확장 | `(ctx, []SettingSection)` |

**Template Hooks (SSR 렌더링 시):**

| Hook | 타입 | 설명 | 파라미터 |
|------|------|------|---------|
| `template.head` | Action | `<head>`에 삽입 | `(ctx, *HeadData)` |
| `template.header` | Action | 헤더 영역에 삽입 | `(ctx)` |
| `template.sidebar` | Action | 사이드바에 삽입 | `(ctx)` |
| `template.footer` | Action | 푸터에 삽입 | `(ctx)` |
| `template.post_after` | Action | 글 본문 아래에 삽입 | `(ctx, *Post)` |

### 6.3 프론트엔드 Hook

프론트엔드 Hook은 `@angple/hook-system` 패키지로 구현됩니다:

```typescript
// Hook 등록 (플러그인/테마)
import { hookSystem } from '@angple/hook-system';

hookSystem.addAction('post.after_render', (post) => {
    console.log('게시글 렌더링 완료:', post.id);
});

hookSystem.addFilter('post.content', (content) => {
    return content.replace(/특정패턴/g, '변환');
});
```

### 6.4 우선순위 규칙

[플러그인 스펙 §5.3](plugin-spec-v1.0.md#5-hook-시스템)과 동일:

| Priority | 용도 | 예시 |
|----------|------|------|
| 1-9 | 보안, 검증 (반드시 먼저 실행) | 스팸 필터, XSS 방지 |
| 10-49 | 일반 기능 | 대부분의 플러그인 |
| 50-99 | 후처리, 로깅 | 분석, 통계 수집 |

---

## 7. 설정 시스템

### 7.1 Provider Pattern

설정 시스템은 Provider 패턴을 사용하여 저장소를 교체할 수 있습니다:

```
┌──────────────────┐
│  Settings API    │
├──────────────────┤
│  Provider        │ ← 인터페이스
├──────────────────┤
│  JSONProvider    │ ← 파일 기반 (개발/소규모)
│  MySQLProvider   │ ← DB 기반 (운영)
│  RedisProvider   │ ← 캐시 레이어
└──────────────────┘
```

**현재 구현:**
- 개발 환경: `settings.json` 파일 (JSONProvider)
- 운영 환경: `option_meta` 테이블 (MySQLProvider) + Redis 캐시

### 7.2 설정 카테고리

| 카테고리 | namespace | 설명 |
|---------|-----------|------|
| **Site** | `core.site` | 사이트 이름, URL, 로고, 언어 등 |
| **Theme** | `core.theme` | 활성 테마 ID, 테마별 설정 |
| **Plugin** | `{plugin-name}` | 플러그인별 개별 설정 |
| **User** | `core.user` | 회원 관련 정책 (가입 허용, 레벨 정책) |
| **Board** | `core.board` | 게시판 기본 설정 |

### 7.3 SSR 데이터 전달 흐름

```
┌─────────────────────────────────────────────────────────────┐
│ Server (+layout.server.ts)                                  │
│                                                              │
│  1. settings.json / DB에서 활성 테마 ID 조회                  │
│  2. 테마 메타데이터 로드 (theme.json)                        │
│  3. 사이트 설정 조회 (사이트명, 로고 등)                      │
│  4. PageData로 반환                                          │
│                                                              │
│  return {                                                    │
│      activeTheme: { id, name, colors, ... },                │
│      siteSettings: { name, logo, ... },                     │
│      user: { ... } // 인증된 경우                            │
│  }                                                           │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────┐
│ Client (+layout.svelte)                                      │
│                                                              │
│  // SSR 데이터로 스토어 초기화 (깜박임 방지)                   │
│  themeStore.initFromSSR(data.activeTheme);                   │
│  siteStore.init(data.siteSettings);                          │
└─────────────────────────────────────────────────────────────┘
```

---

## 8. 보안 가이드라인

### 8.1 토큰 보안

| 항목 | 정책 |
|------|------|
| Access Token 저장 | httpOnly Cookie **필수** (localStorage 금지) |
| Refresh Token 저장 | httpOnly Cookie, `Path=/api/v2/auth` 제한 |
| 토큰 전송 | HTTPS만 허용 (`Secure` 플래그) |
| SameSite | `Strict` (CSRF 방지) |

### 8.2 CORS 정책

```go
cors.Config{
    AllowOrigins:     "https://angple.com, https://admin.angple.com",
    AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
    AllowHeaders:     "Content-Type, Authorization",
    AllowCredentials: true,
    MaxAge:           86400, // 24시간 preflight 캐시
}
```

개발 환경:
```go
AllowOrigins: "http://localhost:5173, http://localhost:5174"
```

### 8.3 Rate Limiting

| 엔드포인트 | 제한 | 비고 |
|-----------|------|------|
| `/api/v2/auth/login` | 5회/분 | 브루트포스 방지 |
| `/api/v2/auth/register` | 3회/시간 | 스팸 계정 방지 |
| 일반 API (인증) | 60회/분 | 일반적 사용 |
| 일반 API (비인증) | 30회/분 | 봇 제한 |
| 파일 업로드 | 10회/분 | 리소스 보호 |

### 8.4 입력 검증 표준

**모든 입력은 서버에서 검증합니다:**

| 항목 | 검증 방법 |
|------|----------|
| SQL Injection | GORM Prepared Statement (직접 SQL 금지) |
| XSS | HTML 이스케이프 (출력 시) |
| Path Traversal | `sanitizePath()` 유틸리티 사용 |
| File Upload | 확장자 화이트리스트 + MIME 타입 + 파일 크기 제한 |
| JSON 입력 | 구조체 바인딩 + 필드별 검증 |

---

## 9. 프론트엔드 Core (angple)

### 9.1 기술 요구사항

| 항목 | 요구사항 |
|------|---------|
| Svelte 버전 | **5.0+** (Rune 모드 강제) |
| SvelteKit 버전 | **2.22+** |
| TypeScript | strict mode |
| CSS | Tailwind CSS 4.0 |
| UI 컴포넌트 | shadcn-svelte |

### 9.2 레이아웃 시스템

```
routes/
├── +layout.server.ts      # SSR: 테마/설정 로드
├── +layout.svelte          # 루트 레이아웃
│
├── (default)/              # 기본 레이아웃 그룹
│   ├── +layout.svelte      # Default Layout (헤더 + 사이드바 + 푸터)
│   └── boards/
│       └── [board_id]/
│           └── +page.svelte
│
└── (theme)/                # 테마 레이아웃 그룹
    ├── +layout.svelte      # Theme Layout (테마가 완전 제어)
    └── ...
```

### 9.3 Slot 시스템

프론트엔드 Slot은 테마와 플러그인이 UI를 삽입할 수 있는 확장점입니다.

**사용 가능한 슬롯:**

| 슬롯 이름 | 위치 | 용도 |
|----------|------|------|
| `header-before` | 헤더 상단 | 공지 배너, 프로모션 |
| `header-after` | 헤더 하단 | 네비게이션 확장 |
| `sidebar-left-top` | 왼쪽 사이드바 상단 | 위젯 삽입 |
| `sidebar-left-bottom` | 왼쪽 사이드바 하단 | 광고, 링크 |
| `sidebar-right-top` | 오른쪽 사이드바 상단 | 위젯 삽입 |
| `sidebar-right-bottom` | 오른쪽 사이드바 하단 | 광고, 링크 |
| `content-before` | 메인 콘텐츠 상단 | 공지, 배너 |
| `content-after` | 메인 콘텐츠 하단 | 관련 글, 광고 |
| `footer-before` | 푸터 상단 | 추가 정보 |
| `footer-after` | 푸터 하단 | 저작권, 링크 |
| `background` | 배경 | 테마 배경 효과 |
| `landing-hero` | 랜딩 히어로 | 테마 히어로 섹션 |
| `landing-content` | 랜딩 콘텐츠 | 테마 콘텐츠 섹션 |

**플러그인 슬롯** ([플러그인 스펙 §8](plugin-spec-v1.0.md#8-프론트엔드-확장)과 일치):

| 슬롯 | 위치 | Props |
|------|------|-------|
| `post.before_title` | 글 제목 위 | `post` |
| `post.after_content` | 글 본문 아래 | `post` |
| `post.actions` | 글 액션 버튼 영역 | `post` |
| `comment.actions` | 댓글 액션 영역 | `comment` |
| `user.profile` | 프로필 페이지 | `user`, `isOwn` |

### 9.4 SlotRegistry API

```typescript
import { registerComponent, getComponentsForSlot } from '$lib/components/slot-manager';

// 컴포넌트 등록
registerComponent('sidebar-right-top', MyWidget, 10, { title: '제목' }, 'my-plugin');

// 슬롯의 컴포넌트 조회
const components = getComponentsForSlot('sidebar-right-top');

// 소스별 제거 (플러그인 비활성화 시)
removeComponentsBySource('my-plugin');
```

### 9.5 API 클라이언트 패턴

```typescript
import { apiClient } from '$lib/api';

// 게시글 조회
const posts = await apiClient.getPosts('free', { page: 1, per_page: 20 });

// 게시글 작성
const post = await apiClient.createPost('free', { title, content });
```

### 9.6 스토어 패턴 (Svelte 5 Rune)

```typescript
// stores/theme.svelte.ts
class ThemeStore {
    activeTheme = $state<ThemeInfo | null>(null);
    isDarkMode = $state(false);

    initFromSSR(theme: ThemeInfo) {
        this.activeTheme = theme;
    }

    get themeId() {
        return $derived(this.activeTheme?.id ?? 'default');
    }
}

export const themeStore = new ThemeStore();
```

---

## 10. 확장 및 연동

### 10.1 관련 스펙 문서

| 문서 | 위치 | 설명 |
|------|------|------|
| **플러그인 스펙 v1.0** | [`plugin-spec-v1.0.md`](plugin-spec-v1.0.md) | 플러그인 개발 규약 |
| **위젯 스펙 v1.0** | [`angple/docs/specs/widget-spec-v1.0.md`](../../../angple/docs/specs/widget-spec-v1.0.md) | 위젯 시스템 규약 |
| **API 버전 전략** | [`api-versioning.md`](api-versioning.md) | v1/v2 전략 |
| **내부 연동 스펙** | [`internal-integration-spec.md`](internal-integration-spec.md) | damoang-ops, angple-ads 연동 |

### 10.2 플러그인 스펙 정합성

Core 스펙과 플러그인 스펙은 다음 영역에서 일관성을 유지해야 합니다:

| Core 스펙 섹션 | 플러그인 스펙 섹션 | 일관성 포인트 |
|---------------|-------------------|--------------|
| §3 DB 스키마 | §4 DB 규칙 | Core 테이블 목록, Meta 테이블 구조 |
| §6 Hook 시스템 | §5 Hook 시스템 | Hook 이름, 타입, 우선순위 |
| §4 API 규약 | §6 API 규칙 | 응답 형식, 에러 코드, 인증 방식 |
| §9 Slot 시스템 | §8 프론트엔드 확장 | 슬롯 목록, Props |

### 10.3 내부 운영 프로젝트

다모앙 운영에 필요한 내부 프로젝트는 별도 문서로 관리합니다:

- **damoang-ops**: 신고/제재 관리 시스템
- **angple-ads**: 광고 관리 시스템 (ads.damoang.net)

> 상세 내용은 [내부 연동 스펙](internal-integration-spec.md)을 참조하세요.

---

**— 문서 끝 —**
