# ang-gnu → angple-backend API 마이그레이션 계획

## 개요
- **소스**: `/Users/sdk/IdeaProjects/ang-gnu` (PHP/Gnuboard)
- **타겟**: `/Users/sdk/IdeaProjects/angple-workspace/angple-backend` (Go/Gin)
- **시작일**: 2026-01-19

---

## 우선순위 1: 회원 검증 API ✅ 완료

### 1.1 ID 중복 확인
- [x] Repository: `ExistsByUserID` 메서드 확인 (이미 존재)
- [x] Service: `member_service.go` 생성
- [x] Handler: `member_handler.go` 생성 - `CheckUserID`
- [x] Route: `POST /api/v2/members/check-id`
- [x] 검증 로직: 빈값, 형식(영문/숫자/_), 최소 3글자, 중복, 예약어

### 1.2 닉네임 중복 확인
- [x] Repository: `ExistsByNickname` 메서드 추가
- [x] Handler: `CheckNickname`
- [x] Route: `POST /api/v2/members/check-nickname`
- [x] 검증 로직: 빈값, 형식(한글/영문/숫자/._), 최소 4바이트, 중복, 예약어

### 1.3 이메일 중복 확인
- [x] Repository: `ExistsByEmailExcluding` 메서드 추가
- [x] Handler: `CheckEmail`
- [x] Route: `POST /api/v2/members/check-email`
- [x] 검증 로직: 빈값, 형식, 금지 도메인, 중복

### 1.4 휴대폰 중복 확인
- [x] Repository: `ExistsByPhone` 메서드 추가
- [x] Handler: `CheckPhone`
- [x] Route: `POST /api/v2/members/check-phone`
- [x] 검증 로직: 형식(01X로 시작, 10-11자리), 중복

### 1.5 통합
- [x] `routes.go` 업데이트
- [x] `main.go` DI 연결
- [x] 빌드 테스트 통과

---

## 우선순위 2: 게시글 관련 API ✅ 완료

### 2.1 자동 저장
- [x] Domain: `autosave.go` 생성
- [x] Repository: `autosave_repo.go` 생성
- [x] Service: `autosave_service.go` 생성
- [x] Handler: `autosave_handler.go` 생성
- [x] Routes:
  - [x] `POST /api/v2/autosave` - 저장
  - [x] `GET /api/v2/autosave` - 목록
  - [x] `GET /api/v2/autosave/:id` - 불러오기
  - [x] `DELETE /api/v2/autosave/:id` - 삭제

### 2.2 게시글 미리보기
- [x] Handler: `GetPostPreview`
- [x] Route: `GET /api/v2/boards/:board_id/posts/:id/preview`

### 2.3 금지어 필터
- [x] Handler: `filter_handler.go` 생성
- [x] Route: `POST /api/v2/filter/check`

### 2.4 토큰 생성
- [x] Handler: `token_handler.go` 생성
- [x] Routes:
  - [x] `POST /api/v2/tokens/write` - 게시글 작성 토큰
  - [x] `POST /api/v2/tokens/comment` - 댓글 토큰

---

## 우선순위 3: 플러그인 API ✅ 완료

### 3.1 회원 메모
- [x] Domain: `memo.go` 생성
- [x] Handler: `memo_handler.go` 생성 (스켈레톤 - TODO: DB 연동)
- [x] Routes:
  - [x] `GET /api/v2/members/:id/memo`
  - [x] `POST /api/v2/members/:id/memo`
  - [x] `PUT /api/v2/members/:id/memo`
  - [x] `DELETE /api/v2/members/:id/memo`

### 3.2 리액션
- [x] Domain: `reaction.go` 생성
- [x] Handler: `reaction_handler.go` 생성 (스켈레톤 - TODO: DB 연동)
- [x] Routes:
  - [x] `GET /api/v2/boards/:board_id/posts/:id/reactions`
  - [x] `POST /api/v2/boards/:board_id/posts/:id/reactions`

### 3.3 신고 (Nariya)
- [x] Domain: `report.go` 생성
- [x] Handler: `report_handler.go` 생성 (스켈레톤 - TODO: DB 연동)
- [x] Routes:
  - [x] `GET /api/v2/reports`
  - [x] `GET /api/v2/reports/data`
  - [x] `GET /api/v2/reports/recent`
  - [x] `POST /api/v2/reports/process`

### 3.4 Dajoongi
- [ ] Handler: `dajoongi_handler.go` 생성 (향후 구현)
- [ ] Route: `GET /api/v2/dajoongi`

---

## 완료 조건
- [ ] 모든 API 엔드포인트 구현 완료
- [ ] 빌드 성공 (`go build ./...`)
- [ ] 린트 통과 (`go vet ./...`)
