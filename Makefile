.PHONY: help dev build build-api build-gateway test clean docker-up docker-down

# 기본 타겟
help:
	@echo "Angple Backend - Available Commands:"
	@echo "  make dev              - 로컬 개발 환경 실행 (go run)"
	@echo "  make build            - 전체 빌드 (api + gateway)"
	@echo "  make build-api        - API 서버 빌드"
	@echo "  make build-gateway    - Gateway 빌드"
	@echo "  make test             - 테스트 실행"
	@echo "  make docker-up        - Docker Compose 실행"
	@echo "  make docker-down      - Docker Compose 중지"
	@echo "  make clean            - 빌드 결과물 삭제"

# 로컬 개발 환경
dev:
	@echo "Starting API server in development mode..."
	go run cmd/api/main.go

dev-gateway:
	@echo "Starting Gateway in development mode..."
	go run cmd/gateway/main.go

# 빌드
build: build-api build-gateway

build-api:
	@echo "Building API server..."
	go build -o bin/api cmd/api/main.go

build-gateway:
	@echo "Building Gateway..."
	go build -o bin/gateway cmd/gateway/main.go

# 테스트
test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Docker
docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-rebuild:
	@echo "Rebuilding Docker containers..."
	docker-compose up -d --build

# 정리
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Go 모듈
deps:
	@echo "Downloading dependencies..."
	go mod download

tidy:
	@echo "Tidying go.mod..."
	go mod tidy

# 린트
lint:
	@echo "Running linter..."
	golangci-lint run

# 포맷
fmt:
	@echo "Formatting code..."
	go fmt ./...
