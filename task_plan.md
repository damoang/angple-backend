# Task Plan: Backend Phase 1 — 추천/비추천, 회원, 파일 업로드

## Goal
백엔드 Phase 1으로 추천/비추천(6개), 회원(6개), 파일 업로드(3개) 총 16개 API를 구현하여 프론트엔드 연동 준비를 완료한다.

## Current Phase
Phase 2: 추천/비추천 시스템

## Phases

### Phase 1: 코드베이스 분석
- [ ] 현재 프로젝트 구조 파악 (internal/ 디렉토리)
- [ ] 기존 구현 패턴 확인 (Post/Comment CRUD 참고)
- [ ] DB 스키마 확인 (그누보드 테이블: g5_board_good, g5_member 등)
- [ ] 라우트 등록 방식 확인 (routes.go, main.go DI)
- [ ] findings.md에 분석 결과 기록
- **Status:** ✅ complete

### Phase 2: 추천/비추천 시스템 (6개 API)
- [ ] Domain 모델 정의 (recommend request/response)
- [ ] Repository 구현 (g5_board_good 테이블 쿼리)
- [ ] Service 구현 (토글 로직, 중복 체크)
- [ ] Handler 구현 (6개 엔드포인트)
- [ ] Route 등록 + DI 설정
- [ ] 테스트
- **Status:** pending

### Phase 3: 회원 시스템 (6개 API)
- [ ] 회원 프로필 조회 (GET /members/{user_id})
- [ ] 회원 작성글/댓글 목록
- [ ] 포인트 내역 조회
- [ ] 회원가입 (POST /auth/register)
- [ ] 소셜 로그인 (POST /auth/social/{provider})
- [ ] 회원 탈퇴 (DELETE /members/me)
- **Status:** pending

### Phase 4: 파일 업로드 시스템 (3개 API)
- [ ] 에디터 이미지 업로드 (webp 변환)
- [ ] 첨부파일 업로드 (포인트 체크)
- [ ] 파일 다운로드 (포인트 차감, 스트림)
- **Status:** pending

### Phase 5: 통합 테스트 및 정리
- [ ] 16개 API 전체 동작 확인
- [ ] api-roadmap.csv 상태 업데이트
- [ ] plan.md Phase 1 완료 표시
- **Status:** pending

## Key Questions
1. g5_board_good 테이블 구조는? (추천/비추천 저장 방식)
2. 소셜 로그인 provider별 OAuth 설정은 어디에?
3. 파일 업로드 경로 및 webp 변환 라이브러리는?
4. 회원 탈퇴 시 데이터 보존 정책 세부사항?

## Decisions Made
| Decision | Rationale |
|----------|-----------|
|          |           |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
|       | 1       |            |

## Notes
- 기존 Post/Comment CRUD 패턴을 최대한 재사용
- Clean Architecture 레이어 분리 필수
- 파일 크기 1000줄 제한 준수
- 그누보드 동적 테이블 패턴 (g5_write_{board_id}) 주의
