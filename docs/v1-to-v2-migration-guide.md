# v1 → v2 API 전환 가이드

> 대상: angple 프론트엔드 개발자
> 작성일: 2026-02-02

---

## 1. 개요

| 항목 | v1 (레거시) | v2 (신규) |
|------|------------|----------|
| Base URL | `/api/v2` | `/api/v2-next` (테스트) → 최종 `/api/v2` |
| DB | 그누보드 `g5_*` 테이블 | 신규 `v2_*` 테이블 |
| 게시판 식별 | `board_id` (문자열, e.g. `"free"`) | `slug` (문자열, e.g. `"free"`) |
| 응답 형식 | `{"data": ..., "meta": {...}}` | `{"success": true, "data": ..., "meta": {...}}` |
| 페이지네이션 | `limit` | `per_page` + `total_pages` |
| 인증 | `damoang_jwt` 쿠키 | `damoang_jwt` 쿠키 (동일) |

---

## 2. 응답 형식 변경

### v1 성공 응답
```json
{
  "data": { "id": 1, "title": "..." },
  "meta": { "page": 1, "limit": 20, "total": 150 }
}
```

### v2 성공 응답
```json
{
  "success": true,
  "data": { "id": 1, "title": "..." },
  "meta": { "page": 1, "per_page": 20, "total": 150, "total_pages": 8 }
}
```

### v1 에러 응답
```json
{
  "error": { "code": "NOT_FOUND", "message": "...", "details": "..." }
}
```

### v2 에러 응답
```json
{
  "success": false,
  "error": { "code": "NOT_FOUND", "message": "...", "details": "..." }
}
```

**프론트엔드 변경점:**
1. 응답에서 `success` 필드로 성공/실패 판별 가능
2. 페이지네이션: `meta.limit` → `meta.per_page`, `meta.total_pages` 추가
3. 에러 응답에도 `success: false` 포함

---

## 3. 엔드포인트 매핑

### 3.1 게시판

| v1 | v2 | 비고 |
|----|-----|------|
| `GET /api/v2/boards` | `GET /api/v2-next/boards` | 응답 필드 변경 |
| `GET /api/v2/boards/:board_id` | `GET /api/v2-next/boards/:slug` | 파라미터명 변경 |

### 3.2 게시글

| v1 | v2 | 비고 |
|----|-----|------|
| `GET /api/v2/boards/:board_id/posts` | `GET /api/v2-next/boards/:slug/posts` | 파라미터명 변경 |
| `GET /api/v2/boards/:board_id/posts/:id` | `GET /api/v2-next/boards/:slug/posts/:id` | |
| `POST /api/v2/boards/:board_id/posts` | `POST /api/v2-next/boards/:slug/posts` | |
| `PUT /api/v2/boards/:board_id/posts/:id` | `PUT /api/v2-next/boards/:slug/posts/:id` | |
| `DELETE /api/v2/boards/:board_id/posts/:id` | `DELETE /api/v2-next/boards/:slug/posts/:id` | |

### 3.3 댓글

| v1 | v2 | 비고 |
|----|-----|------|
| `GET .../posts/:id/comments` | `GET .../posts/:id/comments` | 동일 구조 |
| `POST .../posts/:id/comments` | `POST .../posts/:id/comments` | |
| `DELETE .../comments/:comment_id` | `DELETE .../comments/:comment_id` | |

### 3.4 사용자

| v1 | v2 | 비고 |
|----|-----|------|
| (없음) | `GET /api/v2-next/users` | v2 전용 |
| (없음) | `GET /api/v2-next/users/:id` | v2 전용 |
| (없음) | `GET /api/v2-next/users/username/:username` | v2 전용 |

### 3.5 v2 미구현 (v1 전용, 레거시 유지)

다음 API는 아직 v2로 전환되지 않았습니다. 전환 완료 전까지 v1 엔드포인트를 계속 사용하세요:

- 인증: `/api/v2/auth/*`
- 메뉴: `/api/v2/menus/*`
- 사이트: `/api/v2/sites/*`
- 회원 검증: `/api/v2/members/check-*`
- 스크랩, 차단, 쪽지, 메모
- 알림, WebSocket
- 신고, 이용제한
- 추천글, 갤러리, 통합검색
- 관리자 API
- 플러그인 API

---

## 4. 데이터 모델 변경

### 게시글 (Post)

| v1 필드 | v2 필드 | 비고 |
|---------|---------|------|
| `wr_id` (int) | `id` (uint64) | |
| `wr_subject` | `title` | |
| `wr_content` | `content` | |
| `mb_id` (문자열) | `user_id` (uint64) | FK → v2_users |
| `wr_hit` | `view_count` | |
| `wr_good` | `like_count` | |
| `wr_nogood` | `dislike_count` | |
| `wr_datetime` | `created_at` | ISO 8601 |
| (없음) | `status` | `published`, `draft`, `deleted` |
| (게시판별 동적 테이블) | `board_id` (FK) | 단일 `v2_posts` 테이블 |

### 댓글 (Comment)

| v1 필드 | v2 필드 | 비고 |
|---------|---------|------|
| `wr_id` | `id` | |
| `wr_content` | `content` | |
| `mb_id` | `user_id` (FK) | |
| `wr_parent` | `post_id` (FK) | |
| `wr_comment_reply` | `parent_id` (FK) | 대댓글 |
| (없음) | `depth` | 댓글 깊이 |
| (없음) | `status` | `active`, `deleted` |

### 사용자 (User)

| v1 필드 (g5_member) | v2 필드 (v2_users) | 비고 |
|--------------------|-------------------|------|
| `mb_id` (문자열 PK) | `id` (uint64 PK) | 숫자 PK로 변경 |
| `mb_id` | `username` | |
| `mb_nick` | `nickname` | |
| `mb_email` | `email` | |
| `mb_level` | `level` | |
| `mb_point` | `points` | |
| (없음) | `status` | `active`, `inactive`, `banned` |
| (없음) | `avatar_url` | |
| (없음) | `bio` | |

---

## 5. 전환 절차 (프론트엔드)

### Step 1: API 클라이언트 추상화

```typescript
// api-client.ts
const API_VERSION = process.env.NEXT_PUBLIC_API_VERSION || 'v2'; // 'v2' = legacy, 'v2-next' = new

export const apiBase = `/api/${API_VERSION}`;

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${apiBase}${path}`, {
    credentials: 'include',
    ...options,
  });
  const json = await res.json();

  // v2-next는 success 필드 있음
  if ('success' in json && !json.success) {
    throw new ApiError(json.error);
  }

  return json.data;
}
```

### Step 2: 점진적 전환

1. 환경변수로 API 버전 전환 가능하게 구성
2. 페이지/컴포넌트 단위로 v2-next 테스트
3. 전체 전환 후 `API_VERSION`을 `v2-next`로 고정

### Step 3: 최종 정리

프론트엔드가 100% v2-next 사용 확인 후:
- 백엔드: 레거시 `/api/v2` → `/api/v1`으로 이동
- 백엔드: `/api/v2-next` → `/api/v2`로 승격
- 프론트엔드: `API_VERSION`을 `v2`로 변경

---

## 6. 쿼리 파라미터 변경

| 용도 | v1 | v2 |
|------|-----|-----|
| 페이지 번호 | `?page=1` | `?page=1` (동일) |
| 페이지 크기 | `?per_page=20` | `?per_page=20` (동일) |
| 검색 키워드 | `?keyword=` 또는 `?q=` | `?keyword=` (통일) |
| 정렬 | (API마다 다름) | `?sort=created_at&order=desc` |

---

## 7. 에러 처리

v2에서는 HTTP 상태 코드와 함께 구조화된 에러 코드를 반환합니다:

| 코드 | HTTP | 의미 |
|------|------|------|
| `BAD_REQUEST` | 400 | 잘못된 파라미터 |
| `UNAUTHORIZED` | 401 | 인증 필요 |
| `FORBIDDEN` | 403 | 권한 없음 |
| `NOT_FOUND` | 404 | 리소스 없음 |
| `CONFLICT` | 409 | 중복 |
| `INTERNAL_SERVER_ERROR` | 500 | 서버 오류 |

프론트엔드에서 `response.success === false`일 때 `response.error.code`로 분기 처리하면 됩니다.
