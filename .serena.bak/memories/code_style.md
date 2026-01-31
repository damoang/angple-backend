# Angple Backend - 코드 스타일 & 컨벤션

## Go 표준 스타일
- `gofmt`, `goimports` 사용
- 함수/메서드에 주석 작성
- 테스트 코드 포함

## 파일 크기 제한
- **1000줄 이하** 유지
- 초과 시 모듈 분리 필요

## 아키텍처 패턴
- **인터페이스 기반 설계**: Repository, Service는 인터페이스로 추상화
- **의존성 주입**: `cmd/api/main.go`에서 수동 와이어링

```go
// DI 예시
memberRepo := repository.NewMemberRepository(db)
authService := service.NewAuthService(memberRepo, jwtManager)
authHandler := handler.NewAuthHandler(authService)
```

## API 응답 포맷
- `internal/common/response.go` 사용
- 표준 `APIResponse` 구조체로 일관성 유지

## 에러 처리
- 공유 에러는 `internal/common/errors.go`에 정의

## 새 엔드포인트 추가 순서
1. `internal/domain/` - 모델/DTO 정의
2. `internal/repository/` - 리포지토리 인터페이스 & 구현
3. `internal/service/` - 서비스 인터페이스 & 구현
4. `internal/handler/` - 핸들러 구현
5. `internal/routes/routes.go` - 라우트 등록
6. `cmd/api/main.go` - DI 와이어링

## 네이밍 컨벤션
- 파일명: `snake_case` (예: `auth_handler.go`)
- 패키지명: 소문자 (예: `handler`, `service`)
- 구조체/인터페이스: `PascalCase`
- 함수/메서드: `PascalCase` (public), `camelCase` (private)

## 테스트 파일
- `_test.go` 접미사 사용
- 같은 패키지에 위치

## 환경별 동작
- `local`, `dev`: Mock 인증 활성화
- `staging`, `prod`: 실제 인증만 허용
