# Promotion Plugin (직접홍보 게시판)

광고주가 직접 홍보 게시글을 작성하고, 다른 게시판 목록에 사잇광고로 삽입되는 시스템입니다.

## 기능

- **광고주 관리**: 관리자가 광고주를 등록/수정/삭제
- **직홍게 글 작성**: 광고주가 직접 홍보 글 작성
- **사잇광고**: 다른 게시판 목록에 직홍게 글 자동 삽입
- **상단 고정**: 특정 광고주의 글을 상단에 고정
- **사이드바 위젯**: 직홍게 최신글 위젯

## 설치

1. `plugins/promotion` 디렉토리에 플러그인 복사
2. 관리자 페이지에서 플러그인 활성화
3. DB 마이그레이션 자동 실행

## 설정

관리자 페이지 > 플러그인 > 직접홍보 > 설정에서 변경 가능:

| 설정 | 기본값 | 설명 |
|------|--------|------|
| insert_position | 3 | 사잇광고 삽입 위치 |
| insert_count | 1 | 사잇광고 개수 |
| exclude_boards | promotion,promotion_my,notice | 제외 게시판 |
| sidebar_widget_count | 5 | 사이드바 위젯 글 수 |

## API 엔드포인트

### 공개 API

```
GET  /api/plugins/promotion/posts              # 직홍게 목록
GET  /api/plugins/promotion/posts/insert       # 사잇광고용 글
GET  /api/plugins/promotion/posts/:id          # 직홍게 상세
```

### 광고주 API (로그인 필요)

```
POST   /api/plugins/promotion/posts            # 글 작성
PUT    /api/plugins/promotion/posts/:id        # 글 수정
DELETE /api/plugins/promotion/posts/:id        # 글 삭제
```

### 관리자 API

```
GET    /api/plugins/promotion/admin/advertisers      # 광고주 목록
POST   /api/plugins/promotion/admin/advertisers      # 광고주 추가
PUT    /api/plugins/promotion/admin/advertisers/:id  # 광고주 수정
DELETE /api/plugins/promotion/admin/advertisers/:id  # 광고주 삭제
```

## 데이터베이스

### promotion_advertisers (광고주)

| 컬럼 | 타입 | 설명 |
|------|------|------|
| id | BIGINT | PK |
| member_id | VARCHAR(50) | 회원 ID |
| name | VARCHAR(100) | 광고주명 |
| post_count | INT | 표시할 글 개수 |
| start_date | DATE | 계약 시작일 |
| end_date | DATE | 계약 종료일 |
| is_pinned | BOOLEAN | 상단 고정 |
| is_active | BOOLEAN | 활성화 |

### promotion_posts (직홍게 글)

| 컬럼 | 타입 | 설명 |
|------|------|------|
| id | BIGINT | PK |
| advertiser_id | BIGINT | 광고주 ID (FK) |
| title | VARCHAR(255) | 제목 |
| content | TEXT | 내용 |
| link_url | VARCHAR(500) | 외부 링크 |
| image_url | VARCHAR(500) | 대표 이미지 |
| views | INT | 조회수 |

## 라이선스

MIT License
