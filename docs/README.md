# Angple Backend API 문서

## 📋 API 로드맵 (CSV)

전체 API 목록은 `api-roadmap.csv` 파일에서 확인할 수 있습니다.

### 엑셀에서 열기
1. Excel 또는 Google Sheets에서 `api-roadmap.csv` 파일 열기
2. 데이터 > 텍스트 나누기 > 쉼표로 구분

### 필터링하여 보기
- **Phase**: 구현 단계별로 필터링
- **Status**: ✅ (완료) / ❌ (미구현)으로 필터링
- **Priority**: 우선순위별 정렬

## 📖 Swagger API 문서

### 로컬에서 Swagger UI 실행

#### 방법 1: Docker (권장)
```bash
docker run -p 8082:8080 \
  -e SWAGGER_JSON=/docs/swagger.yaml \
  -v $(pwd)/docs:/docs \
  swaggerapi/swagger-ui
```

브라우저에서 http://localhost:8082 접속

#### 방법 2: Swagger Editor (온라인)
1. https://editor.swagger.io 접속
2. `docs/swagger.yaml` 파일 내용 복사
3. 왼쪽 에디터에 붙여넣기

#### 방법 3: Go 서버에 통합 (예정)
```bash
# go install
go install github.com/swaggo/swag/cmd/swag@latest

# Swagger 문서 생성
swag init -g cmd/api/main.go -o docs/swag

# 서버 실행 후 접속
# http://localhost:8081/swagger/index.html
```

## 🚀 구현 완료 API (v2.0.0)

### 인증 (Auth)
- ✅ `POST /api/v2/auth/login` - 로그인
- ✅ `POST /api/v2/auth/refresh` - 토큰 재발급
- ✅ `GET /api/v2/auth/profile` - 프로필 조회

### 게시글 (Posts)
- ✅ `GET /api/v2/boards/{board_id}/posts` - 목록 조회
- ✅ `GET /api/v2/boards/{board_id}/posts/search` - 검색
- ✅ `GET /api/v2/boards/{board_id}/posts/{id}` - 상세 조회
- ✅ `POST /api/v2/boards/{board_id}/posts` - 작성 (JWT 필요)
- ✅ `PUT /api/v2/boards/{board_id}/posts/{id}` - 수정 (JWT 필요)
- ✅ `DELETE /api/v2/boards/{board_id}/posts/{id}` - 삭제 (JWT 필요)

### 댓글 (Comments)
- ✅ `GET /api/v2/boards/{board_id}/posts/{post_id}/comments` - 목록 조회
- ✅ `GET /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}` - 상세 조회
- ✅ `POST /api/v2/boards/{board_id}/posts/{post_id}/comments` - 작성 (JWT 필요)
- ✅ `PUT /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}` - 수정 (JWT 필요)
- ✅ `DELETE /api/v2/boards/{board_id}/posts/{post_id}/comments/{id}` - 삭제 (JWT 필요)

## 📅 다음 구현 예정

### Phase 1: 핵심 사용자 기능
1. **추천/비추천 시스템**
   - 게시글 추천/비추천
   - 댓글 추천
   - 두 번 누르면 취소

2. **파일 업로드**
   - 에디터 이미지 업로드 (복붙)
   - 첨부파일 업로드
   - gif → mp4, webp 변환
   - 중복 파일 체크

3. **회원 프로필**
   - 회원 정보 조회 (사이드바)
   - 작성글/댓글 목록
   - 포인트 내역

### Phase 2: 커뮤니티 기능
- 스크랩
- 메모
- 차단
- 쪽지

### Phase 3: 알림 시스템
- 댓글/대댓글 알림
- 추천 받음 알림
- WebSocket 실시간 알림

## 🧪 API 테스트

### 로그인 후 게시글 작성 예제

```bash
# 1. 로그인
TOKEN=$(curl -s -X POST http://localhost:8081/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","password":"test1234"}' \
  | jq -r '.data.access_token')

# 2. 게시글 작성
curl -X POST http://localhost:8081/api/v2/boards/free/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "테스트 게시글",
    "content": "내용입니다",
    "author": "user1"
  }'

# 3. 댓글 작성
curl -X POST http://localhost:8081/api/v2/boards/free/posts/1/comments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "댓글입니다",
    "author": "user1"
  }'
```

## 📊 응답 형식

### 성공 응답
```json
{
  "data": { ... },
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

### 에러 응답
```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Invalid request",
    "details": "Invalid post ID"
  }
}
```

## 🔐 인증

JWT Bearer Token 사용:
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### 토큰 유효기간
- Access Token: 15분
- Refresh Token: 7일

## 📝 업데이트 로그

### v2.0.0 (2025-11-28)
- ✅ 인증 API (로그인, 토큰 재발급, 프로필)
- ✅ 게시글 CRUD API (목록, 검색, 상세, 작성, 수정, 삭제)
- ✅ 댓글 CRUD API (목록, 상세, 작성, 수정, 삭제)
- ✅ JWT 인증 미들웨어
- ✅ 그누보드 레거시 비밀번호 호환
- ✅ Clean Architecture 적용 (Handler → Service → Repository)
