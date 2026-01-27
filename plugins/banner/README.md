# Banner Plugin (배너 광고)

위치 기반 배너 광고 시스템입니다. 헤더, 사이드바, 콘텐츠, 푸터 등 다양한 위치에 배너를 표시하고 클릭/노출 통계를 트래킹합니다.

## 기능

- **위치별 배너 관리**: 헤더, 사이드바, 콘텐츠, 푸터 4가지 위치 지원
- **기간 설정**: 시작일/종료일 기반 자동 노출 제어
- **우선순위**: 같은 위치 내에서 우선순위 높은 배너 먼저 표시
- **클릭/노출 트래킹**: 배너별 노출수, 클릭수, CTR 통계
- **클릭 로그**: IP, User Agent, Referer 등 상세 클릭 기록

## 설치

1. `plugins/banner` 디렉토리에 플러그인 복사
2. 관리자 페이지에서 플러그인 활성화
3. DB 마이그레이션 자동 실행

## 설정

관리자 페이지 > 플러그인 > 배너 광고 > 설정에서 변경 가능:

| 설정 | 기본값 | 설명 |
|------|--------|------|
| header_enabled | true | 헤더 배너 활성화 |
| sidebar_enabled | true | 사이드바 배너 활성화 |
| content_enabled | false | 콘텐츠 중간 배너 활성화 |
| footer_enabled | false | 푸터 배너 활성화 |
| content_insert_after_paragraph | 3 | 콘텐츠 배너 삽입 위치 (N번째 문단 뒤) |
| track_clicks | true | 클릭 추적 활성화 |
| track_views | true | 노출 추적 활성화 |

## API 엔드포인트

### 공개 API

```
GET  /api/plugins/banner/list              # 활성 배너 목록 (위치별)
GET  /api/plugins/banner/:id/click         # 클릭 트래킹 및 리다이렉트
POST /api/plugins/banner/:id/view          # 노출 트래킹
```

### 관리자 API

```
GET    /api/plugins/banner/admin/list      # 모든 배너 목록
POST   /api/plugins/banner/admin           # 배너 추가
PUT    /api/plugins/banner/admin/:id       # 배너 수정
DELETE /api/plugins/banner/admin/:id       # 배너 삭제
GET    /api/plugins/banner/admin/:id/stats # 배너 통계
```

## 쿼리 파라미터

### GET /api/plugins/banner/list

| 파라미터 | 타입 | 설명 |
|----------|------|------|
| position | string | 위치 필터 (header, sidebar, content, footer) |

### GET /api/plugins/banner/admin/list

| 파라미터 | 타입 | 설명 |
|----------|------|------|
| position | string | 위치 필터 |
| is_active | boolean | 활성화 상태 필터 |

## 데이터베이스

### banner_items (배너)

| 컬럼 | 타입 | 설명 |
|------|------|------|
| id | BIGINT | PK |
| title | VARCHAR(100) | 배너 제목 (관리용) |
| image_url | VARCHAR(500) | 배너 이미지 URL |
| link_url | VARCHAR(500) | 클릭 시 이동 URL |
| position | ENUM | 위치 (header, sidebar, content, footer) |
| start_date | DATE | 노출 시작일 |
| end_date | DATE | 노출 종료일 |
| priority | INT | 우선순위 (높을수록 먼저) |
| is_active | BOOLEAN | 활성화 여부 |
| click_count | INT | 클릭 수 |
| view_count | INT | 노출 수 |
| alt_text | VARCHAR(255) | 이미지 대체 텍스트 |
| target | ENUM | 링크 타겟 (_self, _blank) |
| memo | TEXT | 관리자 메모 |

### banner_click_logs (클릭 로그)

| 컬럼 | 타입 | 설명 |
|------|------|------|
| id | BIGINT | PK |
| banner_id | BIGINT | 배너 ID (FK) |
| member_id | VARCHAR(50) | 회원 ID |
| ip_address | VARCHAR(45) | IP 주소 |
| user_agent | VARCHAR(500) | User Agent |
| referer | VARCHAR(500) | Referer URL |
| created_at | TIMESTAMP | 클릭 시각 |

## 프론트엔드 슬롯

| 슬롯 | 컴포넌트 | 설명 |
|------|----------|------|
| global.header | HeaderBanner.svelte | 헤더 배너 (슬라이더) |
| global.sidebar | SidebarBanner.svelte | 사이드바 배너 |
| post.after_content | ContentBanner.svelte | 콘텐츠 중간 배너 |

## 사용 예시

### 배너 추가

```bash
curl -X POST /api/plugins/banner/admin \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "신년 이벤트",
    "image_url": "https://example.com/banner.jpg",
    "link_url": "https://example.com/event",
    "position": "header",
    "start_date": "2024-01-01T00:00:00Z",
    "end_date": "2024-01-31T23:59:59Z",
    "priority": 10,
    "is_active": true
  }'
```

### 배너 통계 조회

```bash
curl /api/plugins/banner/admin/1/stats \
  -H "Authorization: Bearer <token>"

# Response
{
  "banner_id": 1,
  "title": "신년 이벤트",
  "view_count": 12500,
  "click_count": 250,
  "ctr": 2.0,
  "daily_clicks": [
    {"date": "2024-01-20", "count": 35},
    {"date": "2024-01-21", "count": 42}
  ]
}
```

## 라이선스

MIT License
