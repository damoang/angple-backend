# ANGPLE 내부 운영 프로젝트 연동 스펙 v1.0

> damoang-ops, angple-ads 등 내부 운영 프로젝트와 Core의 연동 규약
> SDK Corporation, 2026년 1월

---

## 목차

1. [개요](#1-개요)
2. [생태계 구성](#2-생태계-구성)
3. [damoang-ops (신고/제재 관리)](#3-damoang-ops-신고제재-관리)
4. [angple-ads (광고 관리)](#4-angple-ads-광고-관리)
5. [인증 연동](#5-인증-연동)
6. [데이터 흐름](#6-데이터-흐름)
7. [보안 고려사항](#7-보안-고려사항)

---

## 1. 개요

### 1.1 문서 목적

이 문서는 **다모앙(damoang.net) 운영에 필요한 내부 프로젝트**와 ANGPLE Core의 연동 방법을 정의합니다. 오픈소스 Core에는 포함되지 않는 비공개 운영 도구입니다.

### 1.2 원칙

- 내부 운영 프로젝트는 Core를 **수정하지 않고** Core API와 Hook만 사용합니다
- 내부 프로젝트는 별도 리포지토리로 관리합니다
- Core의 플러그인 스펙을 준수하되, 비공개 플러그인으로 배포합니다

---

## 2. 생태계 구성

```
┌──────────────────────────────────────────────────────────────┐
│                      오픈소스 (MIT)                           │
│                                                               │
│  angple              angple-backend                           │
│  (SvelteKit 5)       (Go/Fiber API)                          │
│  프론트엔드           백엔드                                   │
│                                                               │
├──────────────────────────────────────────────────────────────┤
│                      내부 운영용 (비공개)                       │
│                                                               │
│  damoang-ops                    angple-ads                    │
│  (신고/제재 관리)                (광고 관리)                    │
│  ops.damoang.net               ads.damoang.net               │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

---

## 3. damoang-ops (신고/제재 관리)

### 3.1 기능 범위

| 기능 | 설명 |
|------|------|
| 신고 접수 | 게시글/댓글/회원 신고 접수 및 목록 관리 |
| 제재 관리 | 경고, 게시 제한, 정지, 영구 정지 |
| 신고 통계 | 신고 추이, 유형별 통계 |
| 관리자 로그 | 제재 이력 감사 로그 |

### 3.2 Core 연동 방식

**사용하는 Hook:**

| Hook | 용도 |
|------|------|
| `post.before_create` | 제재 상태 확인 (게시 제한 사용자 차단) |
| `comment.before_create` | 제재 상태 확인 |
| `user.after_login` | 정지 상태 사용자 로그인 차단 |
| `admin.menu` | 관리자 메뉴에 "신고 관리" 추가 |

**사용하는 Meta 테이블:**

```sql
-- user_meta에 제재 정보 저장
INSERT INTO user_meta (user_id, namespace, meta_key, meta_value)
VALUES (123, 'damoang-ops', 'sanction_status', '{"type": "warning", "until": "2025-02-01"}');
```

**전용 API:**

```
POST   /api/plugins/damoang-ops/reports          # 신고 접수
GET    /api/plugins/damoang-ops/reports           # 신고 목록
PUT    /api/plugins/damoang-ops/reports/:id       # 신고 처리
POST   /api/plugins/damoang-ops/sanctions         # 제재 적용
GET    /api/plugins/damoang-ops/sanctions/:userId # 제재 이력
```

### 3.3 배포

- 비공개 리포지토리 (`github.com/damoang/damoang-ops`)
- Go 플러그인으로 `angple-backend`에 내장 빌드
- 프론트엔드는 Admin 앱 내 별도 라우트 (`/admin/ops/`)

---

## 4. angple-ads (광고 관리)

### 4.1 기능 범위

| 기능 | 설명 |
|------|------|
| 광고 등록 | 이미지/HTML 배너, 텍스트 광고 등록 |
| 위치 관리 | 광고 위치(position)별 배너 할당 |
| 스케줄링 | 시작/종료일, 요일별 노출 설정 |
| 통계 | 노출 수, 클릭 수, CTR |
| 광고주 관리 | 광고주 계정, 결제 이력 |

### 4.2 아키텍처

```
┌──────────────────────┐         ┌──────────────────────┐
│  angple (Web)        │         │  angple-ads          │
│                      │         │  (독립 서비스)        │
│  <AdSlot             │ ──────> │  ads.damoang.net     │
│    position="..."    │  iframe │                      │
│  />                  │         │  /serve?position=... │
└──────────────────────┘         └──────────┬───────────┘
                                            │
                                            ▼
                                 ┌──────────────────────┐
                                 │  MySQL (ads DB)      │
                                 │  별도 데이터베이스     │
                                 └──────────────────────┘
```

**핵심:** angple-ads는 **완전히 독립된 서비스**로, Core와는 iframe/API로만 통신합니다.

### 4.3 프론트엔드 연동

**환경변수:**

```bash
VITE_ADS_URL=https://ads.damoang.net
```

**AdSlot 컴포넌트:**

```svelte
<iframe
    src="{VITE_ADS_URL}/serve?position={position}"
    width="100%"
    height="{height}"
    frameborder="0"
    loading="lazy"
/>
```

### 4.4 광고 위치 규약

위젯 스펙 §10.3의 position 규약을 따릅니다:

| Position | 위치 | 권장 크기 |
|----------|------|----------|
| `header-top` | 헤더 최상단 | 728x90 |
| `sidebar-top` | 사이드바 상단 | 300x250 |
| `sidebar-bottom` | 사이드바 하단 | 300x600 |
| `content-top` | 콘텐츠 상단 | 728x90 |
| `content-bottom` | 콘텐츠 하단 | 728x90 |
| `post-before` | 게시글 상단 | 728x90 |
| `post-after` | 게시글 하단 | 728x90 |
| `index-custom` | 메인 페이지 | 가변 |

### 4.5 광고 서빙 API

```
GET /serve?position={position}&site={site_id}

# 응답: 해당 position에 활성화된 광고 HTML 렌더링
# 통계: 노출 자동 카운트

POST /click?ad_id={ad_id}
# 클릭 추적 → 광고주 URL로 리다이렉트
```

### 4.6 배포

- 비공개 리포지토리 (`github.com/damoang/angple-ads`)
- 독립 서비스로 배포 (`ads.damoang.net`)
- 별도 DB (Core DB와 분리)

---

## 5. 인증 연동

### 5.1 내부 프로젝트 인증 흐름

```
1. 관리자가 Core(angple)에 로그인
2. Core JWT 토큰 발급
3. 내부 프로젝트 접근 시:
   a. damoang-ops: Core JWT로 인증 (같은 JWT 시크릿 공유)
   b. angple-ads 관리자: 별도 인증 또는 Core JWT 검증
```

### 5.2 JWT 시크릿 공유

내부 프로젝트가 Core JWT를 검증하려면 동일한 JWT 시크릿을 공유해야 합니다:

```yaml
# angple-backend config
jwt:
  secret: ${JWT_SECRET}

# damoang-ops config (동일)
jwt:
  secret: ${JWT_SECRET}
  issuer: "angple"
```

### 5.3 권한 요구사항

| 프로젝트 | 최소 권한 레벨 |
|---------|--------------|
| damoang-ops | Level 8 (부관리자) |
| angple-ads 관리자 | Level 9 (관리자) |

---

## 6. 데이터 흐름

### 6.1 damoang-ops → Core

```
[사용자 신고]
    → damoang-ops API에 신고 접수
    → 관리자 확인 후 제재 결정
    → user_meta에 제재 정보 저장
    → Core Hook이 제재 상태 확인하여 행동 차단
```

### 6.2 angple-ads → angple (프론트엔드)

```
[광고 표시]
    → angple의 AdSlot 컴포넌트가 iframe으로 광고 요청
    → angple-ads가 position에 맞는 광고 반환
    → iframe 내에서 렌더링 (보안 격리)
    → 클릭 시 angple-ads가 추적 후 리다이렉트
```

---

## 7. 보안 고려사항

### 7.1 격리 원칙

| 항목 | 정책 |
|------|------|
| 데이터베이스 | angple-ads는 별도 DB 사용. Core DB 직접 접근 금지 |
| 네트워크 | 내부 프로젝트 간 통신은 내부 네트워크에서만 |
| 인증 | JWT 시크릿은 환경변수로 관리, 코드에 하드코딩 금지 |
| CORS | angple-ads iframe은 sandbox 속성 적용 |

### 7.2 iframe 보안 (angple-ads)

```html
<iframe
    src="https://ads.damoang.net/serve?position=..."
    sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
    referrerpolicy="no-referrer"
    loading="lazy"
></iframe>
```

### 7.3 감사 로그

모든 내부 프로젝트의 관리자 행위는 감사 로그에 기록합니다:

```sql
-- plugin_events 테이블 활용
INSERT INTO plugin_events (plugin_name, event_type, details, created_at)
VALUES ('damoang-ops', 'enabled', '{"action": "sanction", "target_user": 123, "admin": 456}', NOW());
```

---

**— 문서 끝 —**
