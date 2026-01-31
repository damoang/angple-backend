# Angple Backend - 명령어 가이드

## 개발 환경 시작
```bash
# Docker 환경 시작 (MySQL + Redis)
make docker-up
# 또는
docker-compose up -d

# API 서버 실행
make dev
# 또는
go run cmd/api/main.go
```

## 빌드
```bash
# 전체 빌드 (api + gateway)
make build

# API만 빌드
make build-api

# 빌드 결과물
./bin/api
```

## 테스트
```bash
# 전체 테스트
make test
# 또는
go test -v ./...

# 커버리지 포함
make test-coverage

# 특정 패키지 테스트
go test ./internal/service/...

# 특정 함수 테스트
go test -run TestAuthService ./internal/service/
```

## 린트 & 포맷
```bash
# 린트 실행 (golangci-lint 필요)
make lint

# 코드 포맷
make fmt
# 또는
go fmt ./...
```

## Docker 관리
```bash
# 컨테이너 상태 확인
docker-compose ps

# 로그 확인
make docker-logs

# 컨테이너 중지
make docker-down

# 컨테이너 재빌드
make docker-rebuild
```

## 의존성 관리
```bash
# 의존성 다운로드
make deps
# 또는
go mod download

# go.mod 정리
make tidy
# 또는
go mod tidy
```

## 정리
```bash
# 빌드 결과물 삭제
make clean
```

## API 테스트
```bash
# Health Check
curl http://localhost:8081/health

# 메뉴 API
curl http://localhost:8081/api/v2/menus/sidebar

# 게시글 목록
curl http://localhost:8081/api/v2/boards/free/posts
```

## Git 워크플로우
```bash
# 새 브랜치 생성
git checkout -b feature/작업명

# 커밋/푸시 전 확인
make test && make lint
```
