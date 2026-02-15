# ANGPLE Plugin Specification v1.0

> 플러그인 개발자를 위한 공식 규약 문서
> SDK Corporation, 2026년 1월
>
> **관련 문서:** [Core 스펙 v1.0](core-spec-v1.0.md) | [위젯 스펙 v1.0](../../../angple/docs/specs/widget-spec-v1.0.md) | [내부 연동 스펙](internal-integration-spec.md)

---

## 목차

1. [철학과 원칙](#1-철학과-원칙)
2. [플러그인 구조](#2-플러그인-구조)
3. [plugin.yaml 스펙](#3-pluginyaml-스펙)
4. [데이터베이스 규칙](#4-데이터베이스-규칙)
5. [Hook 시스템](#5-hook-시스템)
6. [API 규칙](#6-api-규칙)
7. [관리자 UI 확장](#7-관리자-ui-확장)
8. [프론트엔드 확장](#8-프론트엔드-확장)
9. [보안 가이드라인](#9-보안-가이드라인)
10. [배포 및 마켓플레이스](#10-배포-및-마켓플레이스)
11. [버전 관리 정책](#11-버전-관리-정책)

---

## 1. 철학과 원칙

### 1.1 핵심 철학

ANGPLE 플러그인 시스템은 다음 세 가지 원칙을 기반으로 설계되었습니다.

| 원칙 | 설명 |
|------|------|
| **Core 최소주의** | Core는 커뮤니티 운영에 필수적인 기능만 포함. 모든 확장 기능은 플러그인으로 구현 |
| **비침투적 확장** | 플러그인은 Core의 코드, DB 스키마, API를 직접 수정하지 않음. Hook과 확장점만 사용 |
| **독립적 생명주기** | 플러그인의 설치/업데이트/제거는 Core와 다른 플러그인에 영향 없음 |

### 1.2 이 규약을 따라야 하는 이유

- Core 업데이트 시 플러그인이 깨지지 않습니다
- 다른 개발자의 플러그인과 충돌 없이 공존할 수 있습니다
- 마켓플레이스 등록 시 자동 검증을 통과할 수 있습니다
- 사용자가 안심하고 플러그인을 설치할 수 있습니다

### 1.3 규약 위반 시

- 마켓플레이스 등록이 거부됩니다
- Core 업데이트 후 플러그인이 동작하지 않을 수 있습니다
- 다른 플러그인과 충돌이 발생할 수 있습니다
- 사용자 데이터 손실의 위험이 있습니다

---

## 2. 플러그인 구조

### 2.1 디렉토리 구조

```
plugins/{plugin-name}/
├── plugin.yaml          # [필수] 플러그인 매니페스트
├── main.go              # [필수] 플러그인 진입점
├── migrations/          # [선택] DB 마이그레이션
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
├── handlers/            # [선택] HTTP 핸들러
│   └── handler.go
├── hooks/               # [선택] Hook 핸들러
│   └── hooks.go
├── admin/               # [선택] 관리자 페이지
│   ├── pages/
│   └── components/
├── web/                 # [선택] 프론트엔드 컴포넌트
│   ├── components/
│   └── assets/
├── locales/             # [선택] 다국어 지원
│   ├── ko.yaml
│   └── en.yaml
├── README.md            # [권장] 문서
└── LICENSE              # [권장] 라이선스
```

### 2.2 네이밍 규칙

| 항목 | 규칙 | 예시 |
|------|------|------|
| 플러그인 이름 | 소문자, 하이픈 사용 | `bookmark` |
| 테이블 이름 | `{plugin}_{table}` | `bookmark_items` |
| API 경로 | `/api/plugins/{plugin}/` | `/api/plugins/bookmark/` |
| Hook 이름 | `{plugin}.{action}` | `bookmark.added` |
| 설정 키 | `{plugin}.{key}` | `bookmark.max_items` |

---

## 3. plugin.yaml 스펙

### 3.1 전체 스키마

```yaml
# 기본 정보 (필수)
name: my-plugin                        # 플러그인 고유 식별자
version: 1.0.0                         # 시맨틱 버전
title: 내 플러그인                       # 표시 이름
description: 플러그인 설명               # 설명
author: 개발자명                         # 개발자/조직
license: MIT                           # 라이선스
homepage: https://github.com/...       # 홈페이지 URL

# 호환성 (필수)
requires:
  angple: ">=1.0.0 <2.0.0"            # Core 버전 요구사항
  go: ">=1.21"                        # Go 버전 (선택)
  plugins:                             # 의존 플러그인 (선택)
    - name: other-plugin
      version: ">=1.0.0"

# 충돌 플러그인 (선택)
conflicts:
  - old-plugin                         # 함께 설치 불가

# DB 마이그레이션 (선택)
migrations:
  - file: 001_init.up.sql
    version: 1

# Hook 등록 (선택)
hooks:
  - event: post.after_create           # Core Hook
    handler: OnPostCreated             # Go 함수명
    priority: 10                       # 실행 순서 (낮을수록 먼저)

# API 라우트 (선택)
routes:
  - path: /list
    method: GET
    handler: ListItems
    auth: required                     # required | optional | none

# 설정 스키마 (선택) - 관리자 UI 자동 생성
settings:
  - key: max_items
    type: number
    default: 100
    label: 최대 항목 수

# 권한 정의 (선택)
permissions:
  - id: myplugin.use
    label: 플러그인 사용
```

### 3.2 필수 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `name` | string | 플러그인 고유 식별자. 소문자, 하이픈만 허용 |
| `version` | string | 시맨틱 버전 (예: 1.0.0, 2.1.0-beta) |
| `title` | string | 사용자에게 표시되는 이름 |
| `requires.angple` | string | 호환되는 Core 버전 범위 |

### 3.3 버전 범위 문법

| 표현식 | 의미 |
|--------|------|
| `>=1.0.0` | 1.0.0 이상 모든 버전 |
| `>=1.0.0 <2.0.0` | 1.x.x 버전만 (2.0.0 미만) |
| `~1.2.0` | 1.2.x 버전 (패치 업데이트만 허용) |
| `^1.2.0` | 1.x.x 버전 (마이너 업데이트 허용) |

---

## 4. 데이터베이스 규칙

### 4.1 Core 테이블 (수정 금지)

다음 테이블은 Core가 관리하며, 플러그인에서 **절대 수정할 수 없습니다**:

```
users, posts, comments, boards, categories, tags, files, notifications, sessions
```

**금지 사항:**
- `ALTER TABLE users ADD COLUMN ...`
- `DROP TABLE posts`
- `CREATE INDEX ON comments ...`
- `TRUNCATE TABLE ...`

**허용 사항:**
- `SELECT * FROM users WHERE ...`
- Hook을 통한 데이터 조회
- Meta 테이블을 통한 확장 데이터 저장

### 4.2 Meta 테이블 (확장 저장소)

Core는 플러그인이 확장 데이터를 저장할 수 있도록 Meta 테이블을 제공합니다:

```sql
CREATE TABLE user_meta (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id    BIGINT NOT NULL,
    namespace  VARCHAR(64) NOT NULL,   -- 플러그인 이름
    key        VARCHAR(128) NOT NULL,
    value      JSON,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW() ON UPDATE NOW(),
    UNIQUE KEY (user_id, namespace, key),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- post_meta, comment_meta, option_meta 동일 구조
```

### 4.3 플러그인 전용 테이블

**네이밍 규칙:**
- 형식: `{plugin_name}_{table_name}`
- 플러그인 이름의 하이픈(-)은 언더스코어(_)로 변환
- 예: `hello-world` 플러그인 → `hello_world_logs`

**마이그레이션 파일:**
```
migrations/
├── 001_init.up.sql       # 버전 1 적용
├── 001_init.down.sql     # 버전 1 롤백
├── 002_add_index.up.sql  # 버전 2 적용
└── 002_add_index.down.sql
```

---

## 5. Hook 시스템

### 5.1 Hook 타입

**Action Hook (동작 확장)** - 반환값 없음
```go
// Core가 호출
hook.Do("post.after_create", ctx, post)

// 플러그인에서 등록
hook.Register("post.after_create", func(ctx context.Context, post *Post) {
    // 알림 발송, 로그 기록 등
})
```

**Filter Hook (데이터 변환)** - 데이터 반환
```go
// Core가 호출
content = hook.Apply("post.content", ctx, content)

// 플러그인에서 등록
hook.RegisterFilter("post.content", func(ctx context.Context, content string) string {
    return processedContent
})
```

### 5.2 Core Hook 목록

**Content Hooks:**

| Hook | 타입 | 설명 |
|------|------|------|
| `post.before_create` | Filter | 글 생성 전 (데이터 검증/수정) |
| `post.after_create` | Action | 글 생성 후 (알림, 로그) |
| `post.before_update` | Filter | 글 수정 전 |
| `post.after_update` | Action | 글 수정 후 |
| `post.before_delete` | Filter | 글 삭제 전 (삭제 방지 가능) |
| `post.after_delete` | Action | 글 삭제 후 |
| `post.content` | Filter | 글 내용 렌더링 시 |
| `comment.*` | - | 댓글 관련 (동일 패턴) |

**User Hooks:**

| Hook | 타입 | 설명 |
|------|------|------|
| `user.before_register` | Filter | 회원가입 전 (가입 거부 가능) |
| `user.after_register` | Action | 회원가입 후 |
| `user.after_login` | Action | 로그인 후 |
| `user.permission_check` | Filter | 권한 확인 시 (권한 확장) |

**Admin Hooks:**

| Hook | 타입 | 설명 |
|------|------|------|
| `admin.menu` | Filter | 관리자 메뉴 확장 |
| `admin.dashboard` | Filter | 대시보드 위젯 추가 |
| `admin.settings` | Filter | 설정 페이지 확장 |

**Template Hooks:**

| Hook | 타입 | 설명 |
|------|------|------|
| `template.head` | Action | `<head>`에 삽입 |
| `template.header` | Action | 헤더 영역에 삽입 |
| `template.sidebar` | Action | 사이드바에 삽입 |
| `template.footer` | Action | 푸터에 삽입 |
| `template.post_after` | Action | 글 본문 아래에 삽입 |

### 5.3 Hook 우선순위

| Priority | 용도 | 예시 |
|----------|------|------|
| 1-9 | 보안, 검증 (반드시 먼저 실행) | 스팸 필터, XSS 방지 |
| 10-49 | 일반 기능 | 대부분의 플러그인 |
| 50-99 | 후처리, 로깅 | 분석, 통계 수집 |

---

## 6. API 규칙

### 6.1 네임스페이스

모든 플러그인 API는 자신의 네임스페이스 내에서만 라우트를 등록해야 합니다:

```
# Core API (플러그인 접근 금지)
/api/v1/posts
/api/v1/users

# 플러그인 API (허용)
/api/plugins/{plugin-name}/*
```

**금지 사항:**
- `/api/v1/posts/bookmark` (Core 경로 침범)
- `/api/bookmark` (plugins 네임스페이스 미사용)
- `/api/plugins/bookmark/../users` (경로 탈출)

### 6.2 응답 형식

```json
// 성공 응답
{
    "success": true,
    "data": { ... }
}

// 에러 응답
{
    "success": false,
    "error": {
        "code": "NOT_FOUND",
        "message": "항목을 찾을 수 없습니다"
    }
}
```

### 6.3 인증 및 권한

| auth 값 | 동작 |
|---------|------|
| `required` | 로그인 필수. 미로그인 시 401 반환 |
| `optional` | 로그인 선택. ctx.User()가 nil일 수 있음 |
| `none` | 인증 불필요. 공개 API |

---

## 7. 관리자 UI 확장

### 7.1 자동 설정 UI

`plugin.yaml`의 settings 섹션을 정의하면 Core가 자동으로 설정 UI를 생성합니다:

```yaml
settings:
  - key: max_items
    type: number
    default: 100
    min: 1
    max: 1000
    label: 최대 항목 수

  - key: enabled
    type: boolean
    default: true
    label: 기능 활성화

  - key: display_mode
    type: select
    options:
      - value: grid
        label: 그리드
      - value: list
        label: 목록
    default: list
    label: 표시 방식
```

### 7.2 지원되는 설정 타입

| 타입 | 옵션 | UI 렌더링 |
|------|------|----------|
| `string` | maxLength, pattern | 텍스트 입력 |
| `number` | min, max, step | 숫자 입력 / 슬라이더 |
| `boolean` | - | 토글 스위치 |
| `select` | options[] | 드롭다운 |
| `multiselect` | options[] | 다중 선택 |
| `textarea` | rows, maxLength | 여러 줄 입력 |
| `code` | language | 코드 에디터 |
| `json` | schema | JSON 에디터 |

---

## 8. 프론트엔드 확장

### 8.1 컴포넌트 슬롯

Core 테마는 플러그인이 UI를 삽입할 수 있는 슬롯을 제공합니다:

```svelte
<!-- Core 테마의 post.svelte -->
<article>
    <PluginSlot name="post.before_title" {post} />
    <h1>{post.title}</h1>
    <PluginSlot name="post.after_title" {post} />
    <div class="content">{post.content}</div>
    <PluginSlot name="post.after_content" {post} />
    <div class="actions">
        <PluginSlot name="post.actions" {post} />
    </div>
</article>
```

### 8.2 사용 가능한 슬롯 목록

| 슬롯 | 위치 | 전달 Props |
|------|------|-----------|
| `global.head` | `<head>` 내부 | - |
| `global.header` | 헤더 영역 | user |
| `global.sidebar` | 사이드바 | user |
| `post.before_title` | 글 제목 위 | post |
| `post.after_content` | 글 본문 아래 | post |
| `post.actions` | 글 액션 버튼 영역 | post |
| `comment.actions` | 댓글 액션 영역 | comment |
| `user.profile` | 프로필 페이지 | user, isOwn |

---

## 9. 보안 가이드라인

### 9.1 필수 보안 요구사항

**입력 검증:**
- 모든 사용자 입력은 서버에서 검증
- SQL Injection 방지: Prepared Statement 사용 필수
- XSS 방지: 출력 시 이스케이프 처리
- 파일 업로드: 확장자, MIME 타입, 크기 검증

**인증/권한:**
- 민감한 API는 `auth: required` 설정
- 권한 확인은 Core의 permission 시스템 사용
- 자체 권한 시스템 구현 금지

**데이터 보호:**
- 비밀번호, 토큰 등 민감 정보 로깅 금지
- 개인정보는 암호화 저장
- HTTPS 외 통신 금지

### 9.2 금지 패턴

- `eval()`, `exec()` 등 동적 코드 실행
- 하드코딩된 API 키, 비밀번호
- Core 내부 함수 직접 호출 (미공개 API)
- 다른 플러그인 디렉토리 접근
- 시스템 명령어 실행

### 9.3 보안 검토 체크리스트

| 검사 항목 | 실패 시 |
|----------|--------|
| SQL Injection 취약점 | 등록 거부 |
| XSS 취약점 | 등록 거부 |
| 하드코딩된 시크릿 | 등록 거부 |
| 위험한 함수 사용 | 수동 검토 |
| 과도한 권한 요청 | 수동 검토 |

---

## 10. 배포 및 마켓플레이스

### 10.1 배포 방식

**공개 플러그인 (마켓플레이스):**
- ANGPLE 공식 마켓플레이스에 등록
- 자동 보안 검사 통과 필요
- 무료 또는 유료 판매 가능
- 버전 관리 및 자동 업데이트 지원

**비공개 플러그인:**
- Private Git Repository
- 직접 배포 (ZIP, 바이너리)
- 자체 업데이트 서버 운영 가능

### 10.2 필수 제출 파일

| 파일 | 필수 | 설명 |
|------|------|------|
| `plugin.yaml` | 필수 | 플러그인 매니페스트 |
| `README.md` | 필수 | 설치 방법, 사용법, 설정 설명 |
| `LICENSE` | 필수 | 라이선스 파일 |
| `CHANGELOG.md` | 권장 | 버전별 변경 사항 |
| `screenshot.png` | 권장 | 마켓플레이스 표시용 스크린샷 |

---

## 11. 버전 관리 정책

### 11.1 시맨틱 버전

```
MAJOR.MINOR.PATCH[-PRERELEASE]

예시:
1.0.0        # 첫 정식 릴리즈
1.1.0        # 새 기능 추가 (하위 호환)
1.1.1        # 버그 수정
2.0.0        # 호환성 깨지는 변경
2.0.0-beta.1 # 프리릴리즈
```

### 11.2 버전 변경 기준

| 버전 | 변경 사항 |
|------|----------|
| MAJOR | API 변경, DB 스키마 변경 (마이그레이션 필요), 설정 형식 변경 |
| MINOR | 새 기능 추가, 새 설정 항목 (기본값 제공), 새 Hook 등록 |
| PATCH | 버그 수정, 성능 개선, 문서 수정 |

---

## 부록 A: 플러그인 배포 전 체크리스트

### A.1 필수 항목
- [ ] plugin.yaml의 모든 필수 필드 작성
- [ ] requires.angple 버전 범위 지정
- [ ] README.md 작성 (설치, 설정, 사용법)
- [ ] LICENSE 파일 포함
- [ ] 모든 마이그레이션에 up/down 파일 존재

### A.2 DB 규칙
- [ ] Core 테이블 ALTER 없음
- [ ] 테이블 이름이 `{plugin}_{table}` 형식
- [ ] Meta 테이블 사용 시 namespace 지정
- [ ] Foreign Key는 Core 테이블의 id만 참조

### A.3 API 규칙
- [ ] 모든 라우트가 `/api/plugins/{plugin}/` 하위
- [ ] 인증 필요 API에 `auth: required` 설정
- [ ] 응답 형식이 표준 준수

### A.4 보안
- [ ] SQL Prepared Statement 사용
- [ ] 사용자 입력 이스케이프 처리
- [ ] 하드코딩된 시크릿 없음
- [ ] eval, exec 등 위험 함수 미사용

### A.5 테스트
- [ ] 설치/제거 테스트 완료
- [ ] Core 최신 버전에서 동작 확인
- [ ] 다른 주요 플러그인과 충돌 없음

---

## 부록 B: 예제 - bookmark 플러그인

### B.1 기능
- 글 북마크 추가/제거 (토글)
- 내 북마크 목록 조회
- 글 삭제 시 북마크 자동 정리

### B.2 디렉토리 구조
```
plugins/bookmark/
├── plugin.yaml
├── main.go
├── migrations/
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
├── handlers/
│   ├── toggle.go
│   └── list.go
├── hooks/
│   └── hooks.go
└── web/
    └── components/
        └── BookmarkButton.svelte
```

### B.3 마이그레이션

```sql
-- migrations/001_init.up.sql
CREATE TABLE bookmark_items (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id    BIGINT NOT NULL,
    post_id    BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE KEY (user_id, post_id),
    INDEX idx_user (user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
);
```

---

**— 문서 끝 —**
