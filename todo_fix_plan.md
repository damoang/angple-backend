# TODO/FIXME 해결 계획

## 목표
v1 백엔드의 미완성 코드(TODO/FIXME) 9개를 해결하여 코드 완성도를 높인다.

## Phase 1: 핵심 기능 (1-4번)

### 1. post_handler.go:316 - Nogood 필드 추가
- **현재**: `Nogood: 0`
- **해결**: `Nogood: post.Dislikes` (domain/post.go에 `Dislikes` 필드 이미 존재)
- **난이도**: 낮음 (1줄 수정)

### 2. post_handler.go:321 - Board Subject 조회
- **현재**: `BoardSubject: boardID`
- **해결**: `boardRepo.FindByID(boardID)` → `board.Subject` 사용
- **필요**: PostHandler에 BoardRepository 주입 확인
- **난이도**: 낮음 (DI 확인 후 조회 로직 추가)

### 3. member_handler.go:152 - Nickname 조회 구현
- **현재**: `"nickname": ""`
- **해결**: `memberRepo.FindByUserID(memberID)` → `member.Nickname` 사용
- **필요**: MemberHandler에 MemberRepository 주입
- **난이도**: 낮음 (DI 확인 후 조회 로직 추가)

### 4. ws_handler.go:16 - Origin 검증
- **현재**: `return true` (모든 origin 허용)
- **해결**: 환경변수/설정에서 허용 origin 목록 확인
- **필요**: config에 `allowed_origins` 추가, 검증 로직 구현
- **난이도**: 중간

---

## Phase 2: 사이트 관리 (5-6번)

### 5. site_handler.go:77 - Validator 적용
- **해결**: gin binding validator 사용

### 6. site_handler.go:146 - 인증 추가
- **해결**: middleware.JWTAuth + 권한 체크

---

## Phase 3: 플러그인 (7-9번) - 선택적

### 7. settlement_handler.go:328 - 관리자 권한 확인
### 8. settlement_service.go:200 - 은행 API 연동 (스킵)
### 9. order_service.go:190 - 로깅 추가

---

## 진행 상황
- 시작일: 2026-02-06
- 현재 Phase: 완료
- 완료: 8/9

| # | 파일 | 상태 |
|---|------|------|
| 1 | post_handler.go:316 | ✅ 완료 - `post.Dislikes` 사용, PostResponse에 Dislikes 필드 추가 |
| 2 | post_handler.go:321 | ✅ 완료 - BoardRepo 주입, `board.Subject` 조회 |
| 3 | member_handler.go:152 | ✅ 완료 - MemberRepo 주입, `member.Nickname` 조회 |
| 4 | ws_handler.go:16 | ✅ 완료 - config CORS 설정 기반 origin 검증 |
| 5 | site_handler.go:77 | ✅ 완료 - go-playground/validator 적용 |
| 6 | site_handler.go:146 | ✅ 완료 - CheckUserPermission으로 admin 권한 확인 |
| 7 | settlement_handler.go:328 | ✅ 완료 - GetUserLevel로 관리자 권한 확인 (level >= 10) |
| 8 | settlement_service.go:200 | ❌ 스킵 (은행 API 연동 - 외부 의존성) |
| 9 | order_service.go:190 | ✅ 완료 - log.Printf로 장바구니 삭제 실패 로깅 |
