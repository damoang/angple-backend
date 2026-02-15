# Findings: Backend Phase 1

## 코드베이스 분석 결과

### 프로젝트 구조
```
internal/
├── domain/        (15개 모델: post, comment, member, reaction, memo, report 등)
├── handler/       (17개, 총 3,678줄, 평균 216줄/파일)
├── service/       (14개, 총 2,675줄, 평균 191줄/파일)
├── repository/    (16개, 총 2,922줄, 평균 182줄/파일)
├── middleware/    (JWT, CORS, 쿠키 인증)
├── common/       (에러 정의, 응답 포맷)
├── routes/       (라우트 설정)
├── config/       (YAML + 환경변수 오버라이드)
├── plugin/       (플러그인 시스템 코어)
└── plugins/      (내장 플러그인: commerce, marketplace)
```

### 이미 구현된 Handler/Service/Repository

| 기능 | Handler | Service | Repository | 비고 |
|------|---------|---------|------------|------|
| Post | ✅ | ✅ | ✅ | 동적 테이블 g5_write_{board_id} |
| Comment | ✅ | ✅ | ✅ | 같은 테이블, wr_is_comment=1 |
| Auth | ✅ | ✅ | - | JWT + 그누보드 쿠키 |
| Member | ✅ (155줄) | ✅ (335줄) | ✅ | 회원 검증 서비스 |
| Reaction | ✅ (136줄) | ✅ | ✅ (245줄) | 추천/비추천 |
| Memo | ✅ (234줄) | ✅ | ✅ | 회원 메모 |
| Report | ✅ (253줄) | ✅ | ✅ | 신고 |
| Board | ✅ | ✅ | ✅ | 게시판 설정 |
| Menu | ✅ | ✅ | ✅ | 메뉴 |
| Site | ✅ | ✅ | ✅ | 사이트 설정 |
| Autosave | ✅ | ✅ | ✅ | 임시저장 |
| Promotion | ✅ | ✅ | ✅ | 프로모션 |
| Banner | ✅ | ✅ | ✅ | 배너 |
| Filter | ✅ | - | - | 필터 |
| Token | ✅ | - | - | 토큰 |
| Dajoongi | ✅ | ✅ | ✅ | 다준기 |
| Recommended | ✅ | - | - | 추천글 |

### DI 패턴 (main.go)
```
Repository(db) → Service(repo) → Handler(service) → routes.Setup(router, ...handlers)
```
플러그인: `plugin.NewManager()` → `RegisterBuiltIn()` → `storeSvc.BootEnabledPlugins()`

---

## DB 스키마 발견사항

### 현재 사용 중인 테이블
- `g5_member` — 회원 (mb_id, mb_password, mb_nick, mb_level, mb_point)
- `g5_board` — 게시판 설정 (bo_table, bo_use_good, bo_count_write)
- `g5_write_{board_id}` — 게시글+댓글 동적 테이블
- `g5_member_memo` — 회원 메모 (커스텀 테이블, 이미 구현)

### 미구현 테이블 (Phase 1 대상)
| 테이블 | 용도 | 상태 |
|--------|------|------|
| `g5_board_good` | 추천/비추천 기록 | ❌ 미구현 |
| `g5_board_file` | 첨부파일 | ❌ 미구현 |
| `g5_scrap` | 스크랩 | ❌ 미구현 (Phase 2) |
| `g5_memo` | 쪽지 | ❌ 미구현 (Phase 2) |
| `g5_point` | 포인트 내역 | ❌ 미구현 |

### 핵심 주의사항
- `sql_mode=''` 필수 (NOT NULL 기본값 허용)
- 동적 테이블: `fmt.Sprintf("g5_write_%s", boardID)`
- 비밀번호: 3가지 해싱 (`pkg/auth/legacy.go`)
- `wr_num` 정렬: 가장 작은 음수 - 1
- GORM zero value 생략 방지: `.Select("*").Create()`

---

## 기존 패턴 참고사항

### 새 기능 추가 순서
1. `internal/domain/` — 모델 + Request/Response DTO
2. `internal/repository/` — 인터페이스 + 구현 (동적 테이블 처리)
3. `internal/service/` — 비즈니스 로직 (권한 검증 포함)
4. `internal/handler/` — HTTP 핸들러 (Gin Context)
5. `internal/routes/routes.go` — 라우트 등록
6. `cmd/api/main.go` — DI 설정

### 응답 패턴
```go
// 성공
common.SuccessResponse(c, data, meta)
// 에러
common.ErrorResponse(c, 500, "Failed to ...", err)
```

### Phase 1 실제 작업 범위 재평가

**Reaction(추천/비추천)**: Handler/Service/Repo 이미 존재 → g5_board_good 테이블 연동 + 중복 체크 보강 필요
**Member**: Handler/Service 이미 존재 → 프로필 조회/수정 API 보강 필요
**파일 업로드**: 완전 신규 → g5_board_file 테이블 + 업로드 로직 전부 구현 필요
