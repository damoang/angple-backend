# ANGPLE API 버전 전략

## 버전 개요

| 버전 | 데이터베이스 | 상태 | 목적 |
|------|-------------|------|------|
| **v1** | 그누보드 DB (g5_*) | 현재 개발 중 | 현세대 마이그레이션 |
| **v2** | 신규 설계 DB | 계획 중 | Core 중심 재설계 |

---

## API v1 (현세대 마이그레이션)

### 목표
- 기존 그누보드(ang-gnu) 시스템에서 Go 백엔드로 점진적 마이그레이션
- 100% DB 호환성 유지
- PHP → Go 전환으로 성능 개선 (800ms → 50ms)

### 데이터베이스
```
기존 그누보드 테이블 그대로 사용
├── g5_member          # 회원
├── g5_board           # 게시판 설정
├── g5_write_{board}   # 동적 게시판 테이블
├── g5_board_file      # 첨부파일
├── g5_board_good      # 추천
└── ... (기타 g5_* 테이블)
```

### 특징
- 그누보드 컬럼 네이밍 유지 (wr_*, mb_*, ca_*)
- 동적 테이블 지원 (g5_write_{board_id})
- 댓글: 같은 테이블에 wr_is_comment=1로 저장
- 레거시 비밀번호 호환 (SHA1, MySQL PASSWORD())

### Base URL
```
/api/v1/*   → 그누보드 DB 직접 사용
/api/v2/*   → 현재 사용 중 (실제로는 v1 역할)
```

> **참고**: 현재 `/api/v2`로 되어 있으나, 실제로는 그누보드 DB를 사용하는 v1 역할입니다.
> 추후 정리 시 `/api/v1`으로 변경 예정.

---

## API v2 (신규 설계) - 계획

### 목표
- Core 테이블 최소화
- 플러그인 확장 구조
- 현대적인 DB 설계
- 멀티테넌트 지원

### Core 테이블 (최소 필수)

```sql
-- 사용자
CREATE TABLE users (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    username    VARCHAR(50) UNIQUE NOT NULL,
    email       VARCHAR(255) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    nickname    VARCHAR(100) NOT NULL,
    level       INT DEFAULT 1,
    status      ENUM('active', 'inactive', 'banned') DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW() ON UPDATE NOW()
);

-- 게시판
CREATE TABLE boards (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    slug        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    settings    JSON,
    created_at  TIMESTAMP DEFAULT NOW()
);

-- 게시글
CREATE TABLE posts (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    board_id    BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    title       VARCHAR(255) NOT NULL,
    content     MEDIUMTEXT NOT NULL,
    status      ENUM('draft', 'published', 'deleted') DEFAULT 'published',
    view_count  INT DEFAULT 0,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW() ON UPDATE NOW(),
    FOREIGN KEY (board_id) REFERENCES boards(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 댓글 (별도 테이블)
CREATE TABLE comments (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    post_id     BIGINT NOT NULL,
    user_id     BIGINT NOT NULL,
    parent_id   BIGINT NULL,
    content     TEXT NOT NULL,
    depth       INT DEFAULT 0,
    created_at  TIMESTAMP DEFAULT NOW(),
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (parent_id) REFERENCES comments(id) ON DELETE CASCADE
);

-- Meta 테이블 (플러그인 확장용)
CREATE TABLE user_meta (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT NOT NULL,
    namespace   VARCHAR(64) NOT NULL,
    key         VARCHAR(128) NOT NULL,
    value       JSON,
    UNIQUE KEY (user_id, namespace, key),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE post_meta LIKE user_meta; -- post_id 참조
CREATE TABLE comment_meta LIKE user_meta; -- comment_id 참조
CREATE TABLE option_meta (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    namespace   VARCHAR(64) NOT NULL,
    key         VARCHAR(128) NOT NULL,
    value       JSON,
    UNIQUE KEY (namespace, key)
);
```

### v1 → v2 마이그레이션 전략

1. **Phase 1**: v1 API 완성 (현재)
   - 그누보드 DB로 모든 기능 구현
   - 프론트엔드 완전 전환

2. **Phase 2**: v2 DB 설계 및 구현
   - Core 테이블 설계
   - 데이터 마이그레이션 스크립트
   - v2 API 개발

3. **Phase 3**: 병행 운영
   - v1, v2 API 동시 운영
   - 점진적 v2 전환

4. **Phase 4**: v1 종료
   - v1 API deprecated
   - 그누보드 DB 아카이브

---

## 플러그인과 API 버전

### v1 플러그인 (현재)
- 그누보드 DB 테이블 직접 사용 가능
- `g5_*` 테이블 JOIN 허용
- 기존 ang-gnu 플러그인 호환성 고려

### v2 플러그인 (계획)
- Core 테이블 수정 금지
- Meta 테이블만 사용
- 플러그인 전용 테이블: `{plugin}_{table}` 형식
- 플러그인 스펙 v1.0 준수 필수

---

## 마이그레이션 가이드

### g5_member → users

```sql
INSERT INTO users (username, email, password, nickname, level, created_at)
SELECT mb_id, mb_email, mb_password, mb_nick, mb_level, mb_datetime
FROM g5_member;
```

### g5_write_{board} → posts + comments

```sql
-- 게시글
INSERT INTO posts (board_id, user_id, title, content, created_at)
SELECT b.id, u.id, w.wr_subject, w.wr_content, w.wr_datetime
FROM g5_write_free w
JOIN boards b ON b.slug = 'free'
JOIN users u ON u.username = w.mb_id
WHERE w.wr_is_comment = 0;

-- 댓글
INSERT INTO comments (post_id, user_id, content, created_at)
SELECT p.id, u.id, w.wr_content, w.wr_datetime
FROM g5_write_free w
JOIN posts p ON p.id = w.wr_parent
JOIN users u ON u.username = w.mb_id
WHERE w.wr_is_comment = 1;
```

---

## 결론

| 단계 | 작업 | 시기 |
|------|------|------|
| 현재 | v1 API 완성 (그누보드 DB) | 진행 중 |
| 다음 | 프론트엔드 완전 전환 | v1 완성 후 |
| 이후 | v2 DB 설계 | 안정화 후 |
| 최종 | v1 → v2 마이그레이션 | TBD |
