# Task Plan: Backend Phase 1 & 2 완료

## Goal
백엔드 Phase 1 (추천/비추천, 회원, 파일 업로드)과 Phase 2 (스크랩, 메모, 차단, 쪽지) API를 구현하여 프론트엔드 연동 준비를 완료한다.

## Current Phase
✅ Phase 1 & Phase 2 완료

## Phases

### Phase 1: 추천/비추천, 회원, 파일 업로드 (16개 API)
- [x] 게시글 추천/비추천 (4개 API)
- [x] 댓글 추천 (2개 API)
- [x] 회원 프로필/작성글/댓글/포인트 (4개 API)
- [x] 회원가입/소셜로그인/탈퇴 (3개 API)
- [x] 파일 업로드/다운로드 (3개 API)
- **Status:** ✅ complete

### Phase 2: 스크랩, 메모, 차단, 쪽지 (15개 API)
- [x] 스크랩 (3개 API) - POST/DELETE /posts/:id/scrap, GET /me/scraps
- [x] 메모 (4개 API) - GET/POST/PUT/DELETE /members/:id/memo
- [x] 회원 차단 (3개 API) - POST/DELETE /members/:id/block, GET /members/me/blocks
- [x] 쪽지 (5개 API) - POST /messages, GET inbox/sent/:id, DELETE /:id
- **Status:** ✅ complete

### Phase 3: 알림 시스템 (6개 API) - 미구현
- [ ] 알림 목록 (GET /notifications)
- [ ] 읽지 않은 알림 (GET /notifications/unread)
- [ ] 알림 읽음 처리 (PUT /notifications/:id/read)
- [ ] 모두 읽음 처리 (PUT /notifications/read-all)
- [ ] 알림 삭제 (DELETE /notifications/:id)
- [ ] 실시간 알림 WebSocket (WS /ws/notifications)
- **Status:** pending

### Phase 4: 신고 시스템 (6개 API) - 미구현
- **Status:** pending

### Phase 5: 추천글, 갤러리, 통합 검색 (5개 API) - 미구현
- **Status:** pending

### Phase 6: 관리자 기능 (14개 API) - 미구현
- **Status:** pending

## Summary

| Phase | 상태 | API 수 |
|-------|------|--------|
| Phase 1 | ✅ 완료 | 16개 |
| Phase 2 | ✅ 완료 | 15개 |
| Phase 3 | ❌ 미구현 | 6개 |
| Phase 4 | ❌ 미구현 | 6개 |
| Phase 5 | ❌ 미구현 | 5개 |
| Phase 6 | ❌ 미구현 | 14개 |

**총 구현 완료: 31개 API**

## Notes
- Phase 1 & 2 모두 v2 API로 구현됨
- api-roadmap.csv 업데이트 완료
- v2 Block API 신규 구현 (2026-02-06)
